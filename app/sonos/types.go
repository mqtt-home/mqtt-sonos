package sonos

// Track mirrors the node-sonos-ts Track shape used by sonos2mqtt so existing
// consumers see identical JSON keys.
type Track struct {
	Artist       string `json:"Artist,omitempty"`
	Title        string `json:"Title,omitempty"`
	Album        string `json:"Album,omitempty"`
	AlbumArtUri  string `json:"AlbumArtUri,omitempty"`
	Duration     string `json:"Duration,omitempty"`
	ItemId       string `json:"ItemId,omitempty"`
	ParentId     string `json:"ParentId,omitempty"`
	TrackUri     string `json:"TrackUri,omitempty"`
	ProtocolInfo string `json:"ProtocolInfo,omitempty"`
}

// State is the per-device state object published to the retained `<prefix>/<uuid>`
// topic. Field names and JSON tags match sonos2mqtt's SonosState so the contract
// is preserved.
type State struct {
	UUID             string          `json:"uuid"`
	Model            string          `json:"model,omitempty"`
	Name             string          `json:"name"`
	GroupName        string          `json:"groupName,omitempty"`
	CoordinatorUUID  string          `json:"coordinatorUuid,omitempty"`
	Volume           map[string]int  `json:"volume,omitempty"`
	Mute             map[string]bool `json:"mute,omitempty"`
	CurrentTrack     *Track          `json:"currentTrack,omitempty"`
	NextTrack        *Track          `json:"nextTrack,omitempty"`
	EnqueuedMetadata *Track          `json:"enqueuedMetadata,omitempty"`
	TransportState   string          `json:"transportState,omitempty"`
	PlayMode         string          `json:"playmode,omitempty"`
	Bass             *int            `json:"bass,omitempty"`
	Treble           *int            `json:"treble,omitempty"`
	TS               int64           `json:"ts"`
}

// clone returns a deep copy so callers can publish a stable snapshot while the
// live state keeps mutating.
func (s *State) clone() *State {
	if s == nil {
		return nil
	}
	cp := *s
	if s.Volume != nil {
		cp.Volume = make(map[string]int, len(s.Volume))
		for k, v := range s.Volume {
			cp.Volume[k] = v
		}
	}
	if s.Mute != nil {
		cp.Mute = make(map[string]bool, len(s.Mute))
		for k, v := range s.Mute {
			cp.Mute[k] = v
		}
	}
	if s.CurrentTrack != nil {
		t := *s.CurrentTrack
		cp.CurrentTrack = &t
	}
	if s.NextTrack != nil {
		t := *s.NextTrack
		cp.NextTrack = &t
	}
	if s.EnqueuedMetadata != nil {
		t := *s.EnqueuedMetadata
		cp.EnqueuedMetadata = &t
	}
	if s.Bass != nil {
		v := *s.Bass
		cp.Bass = &v
	}
	if s.Treble != nil {
		v := *s.Treble
		cp.Treble = &v
	}
	return &cp
}
