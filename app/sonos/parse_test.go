package sonos

import "testing"

const avtLastChange = `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/">` +
	`<InstanceID val="0">` +
	`<TransportState val="PLAYING"/>` +
	`<CurrentPlayMode val="SHUFFLE"/>` +
	`<CurrentTrackURI val="x-sonos-spotify:track42"/>` +
	`<CurrentTrackDuration val="0:03:21"/>` +
	`<CurrentTrackMetaData val="&lt;DIDL-Lite xmlns=&quot;urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/&quot; xmlns:dc=&quot;http://purl.org/dc/elements/1.1/&quot; xmlns:upnp=&quot;urn:schemas-upnp-org:metadata-1-0/upnp/&quot;&gt;&lt;item id=&quot;-1&quot; parentID=&quot;-1&quot;&gt;&lt;dc:title&gt;Get Lucky&lt;/dc:title&gt;&lt;dc:creator&gt;Daft Punk&lt;/dc:creator&gt;&lt;upnp:album&gt;Random Access Memories&lt;/upnp:album&gt;&lt;upnp:albumArtURI&gt;/getaa?u=foo&lt;/upnp:albumArtURI&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;"/>` +
	`</InstanceID></Event>`

func TestParseAVTransportLastChange(t *testing.T) {
	u := parseAVTransportLastChange(avtLastChange, "http://10.0.0.1:1400")
	if u == nil {
		t.Fatal("nil update")
	}
	if u.TransportState == nil || *u.TransportState != "PLAYING" {
		t.Errorf("transportState = %v, want PLAYING", u.TransportState)
	}
	if u.PlayMode == nil || *u.PlayMode != "SHUFFLE" {
		t.Errorf("playMode = %v, want SHUFFLE", u.PlayMode)
	}
	if u.CurrentTrack == nil {
		t.Fatal("nil current track")
	}
	if u.CurrentTrack.Title != "Get Lucky" {
		t.Errorf("title = %q, want Get Lucky", u.CurrentTrack.Title)
	}
	if u.CurrentTrack.Artist != "Daft Punk" {
		t.Errorf("artist = %q, want Daft Punk", u.CurrentTrack.Artist)
	}
	if u.CurrentTrack.Album != "Random Access Memories" {
		t.Errorf("album = %q", u.CurrentTrack.Album)
	}
	if u.CurrentTrack.Duration != "0:03:21" {
		t.Errorf("duration = %q, want 0:03:21", u.CurrentTrack.Duration)
	}
	if u.CurrentTrack.TrackUri != "x-sonos-spotify:track42" {
		t.Errorf("trackUri = %q", u.CurrentTrack.TrackUri)
	}
	if want := "http://10.0.0.1:1400/getaa?u=foo"; u.CurrentTrack.AlbumArtUri != want {
		t.Errorf("albumArtUri = %q, want %q", u.CurrentTrack.AlbumArtUri, want)
	}
}

const rcsLastChange = `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/RCS/">` +
	`<InstanceID val="0">` +
	`<Volume channel="Master" val="25"/>` +
	`<Volume channel="LF" val="100"/>` +
	`<Mute channel="Master" val="0"/>` +
	`<Bass val="3"/>` +
	`<Treble val="-2"/>` +
	`</InstanceID></Event>`

func TestParseRenderingControlLastChange(t *testing.T) {
	u := parseRenderingControlLastChange(rcsLastChange)
	if u == nil {
		t.Fatal("nil update")
	}
	if u.Volume["Master"] != 25 {
		t.Errorf("Master volume = %d, want 25", u.Volume["Master"])
	}
	if u.Volume["LF"] != 100 {
		t.Errorf("LF volume = %d, want 100", u.Volume["LF"])
	}
	if muted, ok := u.Mute["Master"]; !ok || muted {
		t.Errorf("Master mute = %v, want false", u.Mute["Master"])
	}
	if u.Bass == nil || *u.Bass != 3 {
		t.Errorf("bass = %v, want 3", u.Bass)
	}
	if u.Treble == nil || *u.Treble != -2 {
		t.Errorf("treble = %v, want -2", u.Treble)
	}
}

const zoneGroupState = `<ZoneGroups>` +
	`<ZoneGroup Coordinator="RINCON_AAA01400" ID="RINCON_AAA01400:1">` +
	`<ZoneGroupMember UUID="RINCON_AAA01400" Location="http://10.0.0.1:1400/xml/device_description.xml" ZoneName="Living Room"/>` +
	`<ZoneGroupMember UUID="RINCON_BBB01400" Location="http://10.0.0.2:1400/xml/device_description.xml" ZoneName="Kitchen"/>` +
	`</ZoneGroup>` +
	`<ZoneGroup Coordinator="RINCON_CCC01400" ID="RINCON_CCC01400:2">` +
	`<ZoneGroupMember UUID="RINCON_CCC01400" Location="http://10.0.0.3:1400/xml/device_description.xml" ZoneName="Bedroom"/>` +
	`<ZoneGroupMember UUID="RINCON_DDD01400" Location="http://10.0.0.4:1400/xml/device_description.xml" ZoneName="Bedroom (R)" Invisible="1"/>` +
	`</ZoneGroup>` +
	`</ZoneGroups>`

func TestParseZoneGroupState(t *testing.T) {
	devices, groups, err := parseZoneGroupState(zoneGroupState)
	if err != nil {
		t.Fatal(err)
	}
	// 3 visible devices (the invisible bonded satellite is skipped).
	if len(devices) != 3 {
		t.Fatalf("devices = %d, want 3", len(devices))
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}

	byUUID := map[string]*Device{}
	for _, d := range devices {
		byUUID[d.UUID] = d
	}
	lr := byUUID["RINCON_AAA01400"]
	if lr == nil || lr.Host != "10.0.0.1" || lr.Port != 1400 || lr.Name != "Living Room" {
		t.Errorf("living room parsed wrong: %+v", lr)
	}

	var g0 Group
	for _, g := range groups {
		if g.Coordinator == "RINCON_AAA01400" {
			g0 = g
		}
	}
	if len(g0.Members) != 2 {
		t.Errorf("group members = %d, want 2", len(g0.Members))
	}
	if g0.Name != "Living Room + 1" {
		t.Errorf("group name = %q, want %q", g0.Name, "Living Room + 1")
	}
}

func TestCleanName(t *testing.T) {
	cases := map[string]string{
		"Living Room": "living-room",
		"Kitchen":     "kitchen",
		"Office  2":   "office-2",
	}
	for in, want := range cases {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDIDLEmpty(t *testing.T) {
	if parseDIDL("", "http://x") != nil {
		t.Error("empty metadata should parse to nil")
	}
	if parseDIDL("NOT_IMPLEMENTED", "http://x") != nil {
		t.Error("NOT_IMPLEMENTED should parse to nil")
	}
}

func TestHostPortFromLocation(t *testing.T) {
	host, port := hostPortFromLocation("http://10.0.0.5:1400/xml/device_description.xml")
	if host != "10.0.0.5" || port != 1400 {
		t.Errorf("got %s:%d, want 10.0.0.5:1400", host, port)
	}
}
