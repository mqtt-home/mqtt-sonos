package sonos

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Command executes a sonos2mqtt-compatible command against a device. Transport
// commands are routed to the device's group coordinator; volume/mute/tone are
// applied to the device itself. input is the raw JSON value of the command's
// `input` field (may be nil).
func (m *Manager) Command(d *Device, command string, input json.RawMessage) error {
	coord := m.Coordinator(d)
	cmd := strings.ToLower(strings.TrimSpace(command))

	switch cmd {
	// --- transport (coordinator) ---
	case "play":
		return coord.Play()
	case "pause":
		return coord.Pause()
	case "stop":
		return coord.Stop()
	case "toggle":
		state, _ := coord.GetTransportInfo()
		if state == "PLAYING" {
			return coord.Pause()
		}
		return coord.Play()
	case "next":
		return coord.Next()
	case "previous":
		return coord.Previous()
	case "seek":
		return coord.Seek("REL_TIME", asString(input))
	case "selecttrack":
		n, ok := asInt(input)
		if !ok {
			return fmt.Errorf("selecttrack requires a number")
		}
		return coord.Seek("TRACK_NR", strconv.Itoa(n))
	case "playmode":
		return coord.SetPlayMode(asString(input))
	case "clearqueue":
		return coord.ClearQueue()
	case "switchtoqueue":
		return coord.SwitchToQueue()
	case "switchtoline":
		return coord.SwitchToLineIn()
	case "switchtotv":
		return coord.SwitchToTV()
	case "setavtransporturi":
		return coord.SetAVTransportURI(asString(input), "")
	case "queue":
		return m.queue(coord, input)

	// --- volume / mute / tone (per device) ---
	case "volume":
		n, ok := asInt(input)
		if !ok {
			return fmt.Errorf("volume requires a number")
		}
		return d.SetVolume(n)
	case "volumeup":
		return d.SetRelativeVolume(stepOrDefault(input, 4))
	case "volumedown":
		return d.SetRelativeVolume(-stepOrDefault(input, 4))
	case "mute":
		return d.SetMute(true)
	case "unmute":
		return d.SetMute(false)
	case "setbass":
		n, _ := asInt(input)
		return d.SetBass(n)
	case "settreble":
		n, _ := asInt(input)
		return d.SetTreble(n)

	// --- grouping ---
	case "joingroup":
		target := m.Resolve(asString(input))
		if target == nil {
			return fmt.Errorf("joingroup: unknown device %q", asString(input))
		}
		return d.JoinGroup(m.Coordinator(target).UUID)
	case "leavegroup":
		return d.BecomeCoordinatorOfStandaloneGroup()

	// --- device properties ---
	case "setledstate":
		return d.SetLEDState(strings.EqualFold(asString(input), "on"))

	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

func (m *Manager) queue(coord *Device, input json.RawMessage) error {
	// Accept either a bare URI string or an object {trackUri, positionInQueue, enqueueAsNext}.
	if uri := asString(input); uri != "" {
		return coord.AddURIToQueue(uri, "", false, 0)
	}
	var obj struct {
		TrackURI        string `json:"trackUri"`
		PositionInQueue int    `json:"positionInQueue"`
		EnqueueAsNext   bool   `json:"enqueueAsNext"`
	}
	if err := json.Unmarshal(input, &obj); err != nil || obj.TrackURI == "" {
		return fmt.Errorf("queue requires a trackUri")
	}
	return coord.AddURIToQueue(obj.TrackURI, "", obj.EnqueueAsNext, obj.PositionInQueue)
}

// PauseAll pauses every group coordinator.
func (m *Manager) PauseAll() {
	seen := map[string]bool{}
	for _, d := range m.Devices() {
		c := m.Coordinator(d)
		if seen[c.UUID] {
			continue
		}
		seen[c.UUID] = true
		_ = c.Pause()
	}
}

// --- input coercion helpers ---

func asString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

func asInt(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	// Allow a numeric string too.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			return v, true
		}
	}
	return 0, false
}

func stepOrDefault(raw json.RawMessage, def int) int {
	if n, ok := asInt(raw); ok {
		return n
	}
	return def
}
