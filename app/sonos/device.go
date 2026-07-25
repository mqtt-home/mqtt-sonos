package sonos

import (
	"fmt"
	"strconv"
)

// Device is a single Sonos speaker reachable on the LAN. Control actions target
// InstanceID 0 (the only instance Sonos exposes).
type Device struct {
	UUID  string // RINCON_xxxxxxxxxxxxx01400
	Name  string // room / zone name
	Model string
	Host  string
	Port  int
}

func (d *Device) baseURL() string {
	return fmt.Sprintf("http://%s:%d", d.Host, d.Port)
}

// --- AVTransport ---

func (d *Device) Play() error {
	_, err := soapCall(d.baseURL(), avTransport, "Play",
		soapArg{"InstanceID", "0"}, soapArg{"Speed", "1"})
	return err
}

func (d *Device) Pause() error {
	_, err := soapCall(d.baseURL(), avTransport, "Pause", soapArg{"InstanceID", "0"})
	return err
}

func (d *Device) Stop() error {
	_, err := soapCall(d.baseURL(), avTransport, "Stop", soapArg{"InstanceID", "0"})
	return err
}

func (d *Device) Next() error {
	_, err := soapCall(d.baseURL(), avTransport, "Next", soapArg{"InstanceID", "0"})
	return err
}

func (d *Device) Previous() error {
	_, err := soapCall(d.baseURL(), avTransport, "Previous", soapArg{"InstanceID", "0"})
	return err
}

// Seek seeks within the current track. unit is REL_TIME (hh:mm:ss),
// TRACK_NR (track number) or TIME_DELTA.
func (d *Device) Seek(unit, target string) error {
	_, err := soapCall(d.baseURL(), avTransport, "Seek",
		soapArg{"InstanceID", "0"}, soapArg{"Unit", unit}, soapArg{"Target", target})
	return err
}

func (d *Device) SetPlayMode(mode string) error {
	_, err := soapCall(d.baseURL(), avTransport, "SetPlayMode",
		soapArg{"InstanceID", "0"}, soapArg{"NewPlayMode", mode})
	return err
}

func (d *Device) SetAVTransportURI(uri, metadata string) error {
	_, err := soapCall(d.baseURL(), avTransport, "SetAVTransportURI",
		soapArg{"InstanceID", "0"}, soapArg{"CurrentURI", uri}, soapArg{"CurrentURIMetaData", metadata})
	return err
}

// BecomeCoordinatorOfStandaloneGroup removes the device from its current group,
// making it a standalone coordinator (i.e. "leave group").
func (d *Device) BecomeCoordinatorOfStandaloneGroup() error {
	_, err := soapCall(d.baseURL(), avTransport, "BecomeCoordinatorOfStandaloneGroup",
		soapArg{"InstanceID", "0"})
	return err
}

// JoinGroup makes this device a member of the group coordinated by the given
// coordinator UUID.
func (d *Device) JoinGroup(coordinatorUUID string) error {
	return d.SetAVTransportURI("x-rincon:"+coordinatorUUID, "")
}

func (d *Device) AddURIToQueue(uri, metadata string, asNext bool, position int) error {
	next := "0"
	if asNext {
		next = "1"
	}
	_, err := soapCall(d.baseURL(), avTransport, "AddURIToQueue",
		soapArg{"InstanceID", "0"},
		soapArg{"EnqueuedURI", uri},
		soapArg{"EnqueuedURIMetaData", metadata},
		soapArg{"DesiredFirstTrackNumberEnqueued", strconv.Itoa(position)},
		soapArg{"EnqueueAsNext", next})
	return err
}

func (d *Device) ClearQueue() error {
	_, err := soapCall(d.baseURL(), avTransport, "RemoveAllTracksFromQueue",
		soapArg{"InstanceID", "0"})
	return err
}

// SwitchToQueue points playback at this device's own queue.
func (d *Device) SwitchToQueue() error {
	return d.SetAVTransportURI("x-rincon-queue:"+d.UUID+"#0", "")
}

func (d *Device) SwitchToLineIn() error {
	return d.SetAVTransportURI("x-rincon-stream:"+d.UUID, "")
}

func (d *Device) SwitchToTV() error {
	return d.SetAVTransportURI("x-sonos-htastream:"+d.UUID+":spdif", "")
}

func (d *Device) SetCrossfadeMode(on bool) error {
	_, err := soapCall(d.baseURL(), avTransport, "SetCrossfadeMode",
		soapArg{"InstanceID", "0"}, soapArg{"CrossfadeMode", boolArg(on)})
	return err
}

// ConfigureSleepTimer sets the sleep timer to a hh:mm:ss duration. An empty
// duration turns the timer off.
func (d *Device) ConfigureSleepTimer(duration string) error {
	_, err := soapCall(d.baseURL(), avTransport, "ConfigureSleepTimer",
		soapArg{"InstanceID", "0"}, soapArg{"NewSleepTimerDuration", duration})
	return err
}

// SnoozeAlarm snoozes a currently ringing alarm for a hh:mm:ss duration. An
// empty duration cancels the snooze.
func (d *Device) SnoozeAlarm(duration string) error {
	_, err := soapCall(d.baseURL(), avTransport, "SnoozeAlarm",
		soapArg{"InstanceID", "0"}, soapArg{"Duration", duration})
	return err
}

func (d *Device) GetTransportInfo() (string, error) {
	body, err := soapCall(d.baseURL(), avTransport, "GetTransportInfo", soapArg{"InstanceID", "0"})
	if err != nil {
		return "", err
	}
	return soapValue(body, "CurrentTransportState"), nil
}

func (d *Device) GetPositionInfo() (track *Track, err error) {
	body, err := soapCall(d.baseURL(), avTransport, "GetPositionInfo", soapArg{"InstanceID", "0"})
	if err != nil {
		return nil, err
	}
	meta := soapValue(body, "TrackMetaData")
	t := parseDIDL(meta, d.baseURL())
	if t == nil {
		t = &Track{}
	}
	t.TrackUri = soapValue(body, "TrackURI")
	if dur := soapValue(body, "TrackDuration"); dur != "" {
		t.Duration = dur
	}
	return t, nil
}

func (d *Device) GetMediaInfo() (playMode string, err error) {
	// CurrentPlayMode is reported via GetTransportSettings.
	body, err := soapCall(d.baseURL(), avTransport, "GetTransportSettings", soapArg{"InstanceID", "0"})
	if err != nil {
		return "", err
	}
	return soapValue(body, "PlayMode"), nil
}

// --- RenderingControl ---

func (d *Device) GetVolume() (int, error) {
	body, err := soapCall(d.baseURL(), renderingControl, "GetVolume",
		soapArg{"InstanceID", "0"}, soapArg{"Channel", "Master"})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(soapValue(body, "CurrentVolume"))
}

func (d *Device) SetVolume(volume int) error {
	_, err := soapCall(d.baseURL(), renderingControl, "SetVolume",
		soapArg{"InstanceID", "0"}, soapArg{"Channel", "Master"}, soapArg{"DesiredVolume", strconv.Itoa(clampVolume(volume))})
	return err
}

func (d *Device) SetRelativeVolume(delta int) error {
	_, err := soapCall(d.baseURL(), renderingControl, "SetRelativeVolume",
		soapArg{"InstanceID", "0"}, soapArg{"Channel", "Master"}, soapArg{"Adjustment", strconv.Itoa(delta)})
	return err
}

func (d *Device) GetMute() (bool, error) {
	body, err := soapCall(d.baseURL(), renderingControl, "GetMute",
		soapArg{"InstanceID", "0"}, soapArg{"Channel", "Master"})
	if err != nil {
		return false, err
	}
	return soapValue(body, "CurrentMute") == "1", nil
}

func (d *Device) SetMute(mute bool) error {
	v := "0"
	if mute {
		v = "1"
	}
	_, err := soapCall(d.baseURL(), renderingControl, "SetMute",
		soapArg{"InstanceID", "0"}, soapArg{"Channel", "Master"}, soapArg{"DesiredMute", v})
	return err
}

func (d *Device) SetBass(bass int) error {
	_, err := soapCall(d.baseURL(), renderingControl, "SetBass",
		soapArg{"InstanceID", "0"}, soapArg{"DesiredBass", strconv.Itoa(bass)})
	return err
}

func (d *Device) SetTreble(treble int) error {
	_, err := soapCall(d.baseURL(), renderingControl, "SetTreble",
		soapArg{"InstanceID", "0"}, soapArg{"DesiredTreble", strconv.Itoa(treble)})
	return err
}

// SetNightMode toggles the speaker's night sound EQ (home theatre models).
func (d *Device) SetNightMode(on bool) error {
	_, err := soapCall(d.baseURL(), renderingControl, "SetEQ",
		soapArg{"InstanceID", "0"}, soapArg{"EQType", "NightMode"}, soapArg{"DesiredValue", boolArg(on)})
	return err
}

// --- GroupRenderingControl ---

// SetGroupVolume sets the volume of the whole group. Must be called on the
// group coordinator.
func (d *Device) SetGroupVolume(volume int) error {
	_, err := soapCall(d.baseURL(), groupRenderingControl, "SetGroupVolume",
		soapArg{"InstanceID", "0"}, soapArg{"DesiredVolume", strconv.Itoa(clampVolume(volume))})
	return err
}

func (d *Device) SetRelativeGroupVolume(delta int) error {
	_, err := soapCall(d.baseURL(), groupRenderingControl, "SetRelativeGroupVolume",
		soapArg{"InstanceID", "0"}, soapArg{"Adjustment", strconv.Itoa(delta)})
	return err
}

// --- DeviceProperties ---

func (d *Device) SetLEDState(on bool) error {
	v := "Off"
	if on {
		v = "On"
	}
	_, err := soapCall(d.baseURL(), deviceProperties, "SetLEDState", soapArg{"DesiredLEDState", v})
	return err
}

// SetButtonLockState locks or unlocks the physical buttons on the speaker.
func (d *Device) SetButtonLockState(locked bool) error {
	v := "Off"
	if locked {
		v = "On"
	}
	_, err := soapCall(d.baseURL(), deviceProperties, "SetButtonLockState",
		soapArg{"DesiredButtonLockState", v})
	return err
}

func boolArg(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func clampVolume(volume int) int {
	if volume < 0 {
		return 0
	}
	if volume > 100 {
		return 100
	}
	return volume
}
