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
	// --- raw UPnP passthrough ---
	case "adv-command":
		return m.advCommand(d, input)
	case "command":
		// Wrapper around another command: {"cmd": "volume", "val": 30}.
		var p struct {
			Cmd string          `json:"cmd"`
			Val json.RawMessage `json:"val"`
		}
		if err := json.Unmarshal(input, &p); err != nil || p.Cmd == "" {
			return fmt.Errorf("command requires an input with a cmd")
		}
		if strings.EqualFold(p.Cmd, "command") {
			return fmt.Errorf("command cannot wrap itself")
		}
		return m.Command(d, p.Cmd, p.Val)

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
	case "shuffle":
		shuffle := boolOrDefault(input, true)
		return updatePlayMode(coord, &shuffle, nil)
	case "repeat":
		repeat, err := asRepeat(input)
		if err != nil {
			return err
		}
		return updatePlayMode(coord, nil, &repeat)
	case "crossfade":
		return coord.SetCrossfadeMode(boolOrDefault(input, true))
	case "sleep":
		dur, err := asDuration(input)
		if err != nil {
			return fmt.Errorf("sleep: %w", err)
		}
		return coord.ConfigureSleepTimer(dur)
	case "snooze":
		dur, err := asDuration(input)
		if err != nil {
			return fmt.Errorf("snooze: %w", err)
		}
		return coord.SnoozeAlarm(dur)
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
		return d.SetRelativeVolume(stepOrDefault(input, 2))
	case "volumedown":
		return d.SetRelativeVolume(-stepOrDefault(input, 2))
	case "groupvolume":
		n, ok := asInt(input)
		if !ok {
			return fmt.Errorf("groupvolume requires a number")
		}
		return coord.SetGroupVolume(n)
	case "groupvolumeup":
		return d.SetRelativeGroupVolume(stepOrDefault(input, 2))
	case "groupvolumedown":
		return d.SetRelativeGroupVolume(-stepOrDefault(input, 2))
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
	case "setbuttonlockstate":
		return d.SetButtonLockState(strings.EqualFold(asString(input), "on"))
	case "setnightmode":
		return d.SetNightMode(boolOrDefault(input, true))

	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

// advCommand runs a raw UPnP action on the addressed device. When the input
// carries a `reply` name, the action's response is published to
// `<prefix>/<uuid>/<reply>`.
func (m *Manager) advCommand(d *Device, input json.RawMessage) error {
	var p struct {
		Cmd   string          `json:"cmd"`
		Val   json.RawMessage `json:"val"`
		Reply string          `json:"reply"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return fmt.Errorf("adv-command requires an input object with cmd/val: %w", err)
	}
	if p.Cmd == "" {
		return fmt.Errorf("adv-command requires a cmd")
	}

	result, err := d.AdvancedCommand(p.Cmd, p.Val)
	if err != nil {
		return err
	}
	if p.Reply != "" && m.onReply != nil {
		m.onReply(d.UUID, p.Reply, result)
	}
	return nil
}

// SonosCommand runs a raw `<Service>.<Action>` call with the payload as its
// arguments — sonos2mqtt's `sonosCommand` control field.
func (m *Manager) SonosCommand(d *Device, cmd string, input json.RawMessage) error {
	_, err := d.AdvancedCommand(cmd, input)
	return err
}

// updatePlayMode changes only the shuffle and/or repeat part of the current
// play mode, leaving the other part as it is.
func updatePlayMode(coord *Device, shuffle *bool, repeat *string) error {
	current, err := coord.GetMediaInfo()
	if err != nil {
		return err
	}
	curShuffle, curRepeat := splitPlayMode(current)
	if shuffle != nil {
		curShuffle = *shuffle
	}
	if repeat != nil {
		curRepeat = *repeat
	}
	return coord.SetPlayMode(joinPlayMode(curShuffle, curRepeat))
}

// splitPlayMode decomposes a Sonos play mode into shuffle and repeat
// ("off", "all" or "one").
func splitPlayMode(mode string) (shuffle bool, repeat string) {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "REPEAT_ALL":
		return false, "all"
	case "REPEAT_ONE":
		return false, "one"
	case "SHUFFLE_NOREPEAT":
		return true, "off"
	case "SHUFFLE":
		return true, "all"
	case "SHUFFLE_REPEAT_ONE":
		return true, "one"
	default: // NORMAL
		return false, "off"
	}
}

func joinPlayMode(shuffle bool, repeat string) string {
	if shuffle {
		switch repeat {
		case "all":
			return "SHUFFLE"
		case "one":
			return "SHUFFLE_REPEAT_ONE"
		default:
			return "SHUFFLE_NOREPEAT"
		}
	}
	switch repeat {
	case "all":
		return "REPEAT_ALL"
	case "one":
		return "REPEAT_ONE"
	default:
		return "NORMAL"
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

// boolOrDefault mirrors sonos2mqtt's payload-to-bool coercion: true, 1 and the
// strings "true"/"on" count as true, an absent payload yields def.
func boolOrDefault(raw json.RawMessage, def bool) bool {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return def
	}
	return strings.EqualFold(s, "true") || strings.EqualFold(s, "on") || s == "1"
}

// asRepeat reads a repeat setting as "off", "all" or "one". A bool selects
// between repeat-all and off, matching sonos2mqtt.
func asRepeat(raw json.RawMessage) (string, error) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	switch strings.ToLower(s) {
	case "true":
		return "all", nil
	case "", "null", "false", "off", "none":
		return "off", nil
	case "all", "repeatall":
		return "all", nil
	case "one", "repeatone":
		return "one", nil
	}
	return "", fmt.Errorf("repeat must be one of Off, RepeatAll, RepeatOne")
}

// asDuration reads a sleep/snooze duration. A number is minutes (1-60), a
// string is passed through as hh:mm:ss, and an empty payload turns it off.
func asDuration(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "false" || trimmed == `""` {
		return "", nil
	}
	if n, ok := asInt(raw); ok {
		if n < 1 || n > 60 {
			return "", fmt.Errorf("minutes must be between 1 and 60")
		}
		return fmt.Sprintf("00:%02d:00", n), nil
	}
	if s := asString(raw); s != "" {
		return s, nil
	}
	return "", fmt.Errorf("expected minutes or a hh:mm:ss duration")
}
