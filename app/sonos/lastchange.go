package sonos

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// avtEvent is the inner <Event> document carried (XML-escaped) in an
// AVTransport LastChange notification.
type avtEvent struct {
	InstanceID struct {
		TransportState               valAttr `xml:"TransportState"`
		CurrentPlayMode              valAttr `xml:"CurrentPlayMode"`
		CurrentTrackURI              valAttr `xml:"CurrentTrackURI"`
		CurrentTrackDuration         valAttr `xml:"CurrentTrackDuration"`
		CurrentTrackMetaData         valAttr `xml:"CurrentTrackMetaData"`
		NextTrackMetaData            valAttr `xml:"NextTrackMetaData"`
		EnqueuedTransportURIMetaData valAttr `xml:"EnqueuedTransportURIMetaData"`
	} `xml:"InstanceID"`
}

// rcsEvent is the inner <Event> document of a RenderingControl LastChange.
type rcsEvent struct {
	InstanceID struct {
		Volume []channelVal `xml:"Volume"`
		Mute   []channelVal `xml:"Mute"`
		Bass   valAttr      `xml:"Bass"`
		Treble valAttr      `xml:"Treble"`
	} `xml:"InstanceID"`
}

type valAttr struct {
	Val string `xml:"val,attr"`
}

type channelVal struct {
	Channel string `xml:"channel,attr"`
	Val     string `xml:"val,attr"`
}

// avtUpdate carries the parsed fields of an AVTransport LastChange. Pointers
// distinguish "absent in this event" from "present and empty".
type avtUpdate struct {
	TransportState   *string
	PlayMode         *string
	CurrentTrack     *Track
	NextTrack        *Track
	EnqueuedMetadata *Track
}

// rcsUpdate carries the parsed fields of a RenderingControl LastChange.
type rcsUpdate struct {
	Volume map[string]int
	Mute   map[string]bool
	Bass   *int
	Treble *int
}

// parseAVTransportLastChange parses the LastChange value of an AVTransport
// event. baseURL is used to absolutise relative album-art URIs.
func parseAVTransportLastChange(lastChange, baseURL string) *avtUpdate {
	var ev avtEvent
	if err := xml.Unmarshal([]byte(lastChange), &ev); err != nil {
		return nil
	}
	u := &avtUpdate{}
	in := ev.InstanceID
	if in.TransportState.Val != "" {
		v := in.TransportState.Val
		u.TransportState = &v
	}
	if in.CurrentPlayMode.Val != "" {
		v := in.CurrentPlayMode.Val
		u.PlayMode = &v
	}
	if in.CurrentTrackMetaData.Val != "" {
		t := parseDIDL(in.CurrentTrackMetaData.Val, baseURL)
		if t == nil {
			t = &Track{}
		}
		if in.CurrentTrackURI.Val != "" {
			t.TrackUri = in.CurrentTrackURI.Val
		}
		if in.CurrentTrackDuration.Val != "" && in.CurrentTrackDuration.Val != "NOT_IMPLEMENTED" {
			t.Duration = in.CurrentTrackDuration.Val
		}
		u.CurrentTrack = t
	}
	if in.NextTrackMetaData.Val != "" {
		if t := parseDIDL(in.NextTrackMetaData.Val, baseURL); t != nil {
			u.NextTrack = t
		}
	}
	if in.EnqueuedTransportURIMetaData.Val != "" {
		if t := parseDIDL(in.EnqueuedTransportURIMetaData.Val, baseURL); t != nil {
			u.EnqueuedMetadata = t
		}
	}
	return u
}

// parseRenderingControlLastChange parses the LastChange value of a
// RenderingControl event.
func parseRenderingControlLastChange(lastChange string) *rcsUpdate {
	var ev rcsEvent
	if err := xml.Unmarshal([]byte(lastChange), &ev); err != nil {
		return nil
	}
	u := &rcsUpdate{}
	for _, v := range ev.InstanceID.Volume {
		if n, err := strconv.Atoi(v.Val); err == nil {
			if u.Volume == nil {
				u.Volume = map[string]int{}
			}
			ch := v.Channel
			if ch == "" {
				ch = "Master"
			}
			u.Volume[ch] = n
		}
	}
	for _, v := range ev.InstanceID.Mute {
		if u.Mute == nil {
			u.Mute = map[string]bool{}
		}
		ch := v.Channel
		if ch == "" {
			ch = "Master"
		}
		u.Mute[ch] = v.Val == "1"
	}
	if ev.InstanceID.Bass.Val != "" {
		if n, err := strconv.Atoi(ev.InstanceID.Bass.Val); err == nil {
			u.Bass = &n
		}
	}
	if ev.InstanceID.Treble.Val != "" {
		if n, err := strconv.Atoi(ev.InstanceID.Treble.Val); err == nil {
			u.Treble = &n
		}
	}
	return u
}

// didlLite matches the DIDL-Lite metadata envelope used in track metadata.
type didlLite struct {
	Items []struct {
		Title       string `xml:"title"`
		Creator     string `xml:"creator"`
		Album       string `xml:"album"`
		AlbumArtURI string `xml:"albumArtURI"`
		ID          string `xml:"id,attr"`
		ParentID    string `xml:"parentID,attr"`
		Res         struct {
			ProtocolInfo string `xml:"protocolInfo,attr"`
			Duration     string `xml:"duration,attr"`
			Value        string `xml:",chardata"`
		} `xml:"res"`
	} `xml:"item"`
}

// parseDIDL parses a DIDL-Lite metadata string into a Track. Returns nil for
// empty/placeholder metadata.
func parseDIDL(metadata, baseURL string) *Track {
	metadata = strings.TrimSpace(metadata)
	if metadata == "" || metadata == "NOT_IMPLEMENTED" {
		return nil
	}
	var didl didlLite
	if err := xml.Unmarshal([]byte(metadata), &didl); err != nil {
		return nil
	}
	if len(didl.Items) == 0 {
		return nil
	}
	it := didl.Items[0]
	t := &Track{
		Title:        it.Title,
		Artist:       it.Creator,
		Album:        it.Album,
		ItemId:       it.ID,
		ParentId:     it.ParentID,
		ProtocolInfo: it.Res.ProtocolInfo,
		Duration:     it.Res.Duration,
		TrackUri:     it.Res.Value,
	}
	if it.AlbumArtURI != "" {
		t.AlbumArtUri = absoluteArtURI(it.AlbumArtURI, baseURL)
	}
	if t.Title == "" && t.Artist == "" && t.Album == "" && t.TrackUri == "" {
		return nil
	}
	return t
}

func absoluteArtURI(uri, baseURL string) string {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	if strings.HasPrefix(uri, "/") {
		return baseURL + uri
	}
	return uri
}
