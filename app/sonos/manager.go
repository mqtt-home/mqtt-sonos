package sonos

import (
	"strings"
	"sync"
	"time"

	"github.com/mqtt-home/mqtt-sonos/config"
	"github.com/philipparndt/go-logger"
)

// Manager owns the set of Sonos devices, their live state and the group
// topology, and turns pushed GENA events into state-change callbacks.
type Manager struct {
	cfg      config.SonosConfig
	seed     string
	listener *EventListener

	mu            sync.RWMutex
	devices       map[string]*Device // uuid -> device
	states        map[string]*State  // uuid -> state
	groups        []Group
	coordinatorOf map[string]string // member uuid -> coordinator uuid

	onStateChange func(uuid string, state *State)

	debounceMu sync.Mutex
	debounce   map[string]*time.Timer

	stopPoll chan struct{}
}

func NewManager(cfg config.SonosConfig, seed string) *Manager {
	return &Manager{
		cfg:           cfg,
		seed:          seed,
		devices:       map[string]*Device{},
		states:        map[string]*State{},
		coordinatorOf: map[string]string{},
		debounce:      map[string]*time.Timer{},
	}
}

// SetStateChangeCallback registers a callback invoked (debounced) whenever a
// device's published state changes.
func (m *Manager) SetStateChangeCallback(cb func(uuid string, state *State)) {
	m.onStateChange = cb
}

// Start discovers the household, begins listening for events, subscribes every
// device and performs an initial state poll.
func (m *Manager) Start() error {
	devices, groups, err := Discover(m.seed, 3*time.Second)
	if err != nil {
		return err
	}

	m.mu.Lock()
	for _, d := range devices {
		m.devices[d.UUID] = d
		m.states[d.UUID] = &State{UUID: d.UUID, Name: d.Name, Model: d.Model}
	}
	m.applyGroups(groups)
	m.mu.Unlock()

	logger.Info("Discovered Sonos devices", "count", len(devices), "groups", len(groups))

	m.listener = NewEventListener(m.cfg.ListenerHost, m.cfg.ListenerPort, m.handleEvent)
	if err := m.listener.Start(); err != nil {
		return err
	}

	// Subscribe to per-device events and to topology on one device.
	first := true
	for _, d := range devices {
		m.listener.SubscribeDevice(d)
		if first {
			m.listener.SubscribeTopology(d)
			first = false
		}
	}

	// Initial poll fills state that LastChange events don't immediately resend.
	for _, d := range devices {
		m.pollDevice(d)
	}

	// Periodic poll as a fallback to GENA events (missed callbacks, unreachable
	// listener host). Events remain the primary, near-instant path.
	if interval := m.cfg.PollingInterval; interval > 0 {
		m.stopPoll = make(chan struct{})
		go m.pollLoop(time.Duration(interval) * time.Second)
		logger.Info("State polling enabled", "interval", interval)
	}

	return nil
}

func (m *Manager) Stop() {
	if m.stopPoll != nil {
		close(m.stopPoll)
		m.stopPoll = nil
	}
	if m.listener != nil {
		m.listener.Stop()
	}
}

// pollLoop periodically re-polls every device.
func (m *Manager) pollLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopPoll:
			return
		case <-ticker.C:
			for _, d := range m.Devices() {
				m.pollDevice(d)
			}
		}
	}
}

// applyGroups updates the grouping. Caller must hold m.mu.
func (m *Manager) applyGroups(groups []Group) {
	m.groups = groups
	m.coordinatorOf = map[string]string{}
	for _, g := range groups {
		for _, member := range g.Members {
			m.coordinatorOf[member] = g.Coordinator
		}
		for _, member := range g.Members {
			if st := m.states[member]; st != nil {
				st.CoordinatorUUID = g.Coordinator
				st.GroupName = g.Name
			}
		}
	}
}

// pollDevice fetches current volume/mute/transport/track and stores it.
func (m *Manager) pollDevice(d *Device) {
	st := m.stateFor(d.UUID)
	if st == nil {
		return
	}

	if vol, err := d.GetVolume(); err == nil {
		m.mu.Lock()
		if st.Volume == nil {
			st.Volume = map[string]int{}
		}
		st.Volume["Master"] = vol
		m.mu.Unlock()
	}
	if mute, err := d.GetMute(); err == nil {
		m.mu.Lock()
		if st.Mute == nil {
			st.Mute = map[string]bool{}
		}
		st.Mute["Master"] = mute
		m.mu.Unlock()
	}
	if ts, err := d.GetTransportInfo(); err == nil && ts != "" {
		m.mu.Lock()
		st.TransportState = ts
		m.mu.Unlock()
	}
	if pm, err := d.GetMediaInfo(); err == nil && pm != "" {
		m.mu.Lock()
		st.PlayMode = pm
		m.mu.Unlock()
	}
	if track, err := d.GetPositionInfo(); err == nil && track != nil {
		m.mu.Lock()
		st.CurrentTrack = track
		m.mu.Unlock()
	}
	m.touch(d.UUID)
}

// handleEvent applies a pushed GENA notification.
func (m *Manager) handleEvent(uuid, svcShort, value string) {
	switch svcShort {
	case svcAVTransport:
		m.applyAVTransport(uuid, value)
	case svcRenderingControl:
		m.applyRenderingControl(uuid, value)
	case svcZoneGroupTopology:
		m.refreshTopology()
	}
}

// refreshTopology re-reads the household grouping from any known device. Called
// when a ZoneGroupTopology event signals a group change; re-fetching via SOAP
// avoids the escaping/nesting ambiguity of the evented payload.
func (m *Manager) refreshTopology() {
	devs := m.Devices()
	if len(devs) == 0 {
		return
	}
	_, groups, err := fetchZoneGroupState(devs[0].baseURL())
	if err != nil {
		logger.Debug("Failed to refresh topology", "error", err)
		return
	}
	m.applyTopology(groups)
}

// applyTopology updates grouping and republishes every device, whose published
// state carries the group name and coordinator.
func (m *Manager) applyTopology(groups []Group) {
	m.mu.Lock()
	m.applyGroups(groups)
	m.mu.Unlock()
	for _, uuid := range m.deviceUUIDs() {
		m.touch(uuid)
	}
}

func (m *Manager) applyAVTransport(uuid, lastChange string) {
	d := m.device(uuid)
	if d == nil {
		return
	}
	upd := parseAVTransportLastChange(lastChange, d.baseURL())
	if upd == nil {
		return
	}
	m.mu.Lock()
	st := m.states[uuid]
	if st != nil {
		if upd.TransportState != nil {
			st.TransportState = *upd.TransportState
		}
		if upd.PlayMode != nil {
			st.PlayMode = *upd.PlayMode
		}
		if upd.CurrentTrack != nil {
			st.CurrentTrack = upd.CurrentTrack
		}
		if upd.NextTrack != nil {
			st.NextTrack = upd.NextTrack
		}
		if upd.EnqueuedMetadata != nil {
			st.EnqueuedMetadata = upd.EnqueuedMetadata
		}
	}
	m.mu.Unlock()
	m.touch(uuid)
}

func (m *Manager) applyRenderingControl(uuid, lastChange string) {
	upd := parseRenderingControlLastChange(lastChange)
	if upd == nil {
		return
	}
	m.mu.Lock()
	st := m.states[uuid]
	if st != nil {
		if len(upd.Volume) > 0 {
			if st.Volume == nil {
				st.Volume = map[string]int{}
			}
			for k, v := range upd.Volume {
				st.Volume[k] = v
			}
		}
		if len(upd.Mute) > 0 {
			if st.Mute == nil {
				st.Mute = map[string]bool{}
			}
			for k, v := range upd.Mute {
				st.Mute[k] = v
			}
		}
		if upd.Bass != nil {
			st.Bass = upd.Bass
		}
		if upd.Treble != nil {
			st.Treble = upd.Treble
		}
	}
	m.mu.Unlock()
	m.touch(uuid)
}

// touch marks a device's state changed and schedules a debounced publish.
func (m *Manager) touch(uuid string) {
	if m.onStateChange == nil {
		return
	}
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()
	if t, ok := m.debounce[uuid]; ok {
		t.Stop()
	}
	m.debounce[uuid] = time.AfterFunc(400*time.Millisecond, func() {
		st := m.snapshot(uuid)
		if st != nil {
			m.onStateChange(uuid, st)
		}
	})
}

// snapshot returns a stable copy of a device's current state with a fresh ts.
func (m *Manager) snapshot(uuid string) *State {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.states[uuid]
	if st == nil {
		return nil
	}
	st.TS = time.Now().UnixMilli()
	return st.clone()
}

// --- lookups ---

func (m *Manager) device(uuid string) *Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.devices[uuid]
}

func (m *Manager) stateFor(uuid string) *State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[uuid]
}

func (m *Manager) deviceUUIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.devices))
	for id := range m.devices {
		ids = append(ids, id)
	}
	return ids
}

// Snapshot returns a stable copy of one device's current state.
func (m *Manager) Snapshot(uuid string) *State {
	return m.snapshot(uuid)
}

// DeviceCount returns the number of known devices.
func (m *Manager) DeviceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}

// Devices returns all known devices.
func (m *Manager) Devices() []*Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, d)
	}
	return out
}

// Groups returns the current grouping.
func (m *Manager) Groups() []Group {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Group, len(m.groups))
	copy(out, m.groups)
	return out
}

// Snapshots returns a stable copy of every device's state.
func (m *Manager) Snapshots() map[string]*State {
	out := map[string]*State{}
	for _, id := range m.deviceUUIDs() {
		if st := m.snapshot(id); st != nil {
			out[id] = st
		}
	}
	return out
}

// Resolve finds a device by UUID (case-insensitive) or by cleaned zone name,
// matching sonos2mqtt's control-topic identifier rules.
func (m *Manager) Resolve(id string) *Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lid := strings.ToLower(id)
	for uuid, d := range m.devices {
		if strings.ToLower(uuid) == lid {
			return d
		}
	}
	for _, d := range m.devices {
		if cleanName(d.Name) == lid || strings.EqualFold(d.Name, id) {
			return d
		}
	}
	return nil
}

// Coordinator returns the group coordinator device for a member device. Falls
// back to the device itself when its group is unknown.
func (m *Manager) Coordinator(d *Device) *Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if coord, ok := m.coordinatorOf[d.UUID]; ok {
		if cd := m.devices[coord]; cd != nil {
			return cd
		}
	}
	return d
}

// cleanName lowercases and replaces whitespace with dashes, matching
// sonos2mqtt's friendly-name topic cleaning.
func cleanName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}
