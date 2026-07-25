package sonos

import (
	"strings"
	"testing"
)

func TestGuessTrackURIRadio(t *testing.T) {
	uri, metadata := GuessTrackURI("radio:s24896")

	if want := "x-sonosapi-stream:s24896?sid=254&flags=8224&sn=0"; uri != want {
		t.Errorf("uri = %q, want %q", uri, want)
	}
	for _, want := range []string{
		`<item id="10092020s24896" restricted="true">`,
		`<upnp:class>object.item.audioItem.audioBroadcast</upnp:class>`,
		`RINCON_AssociatedZPUDN`,
	} {
		if !strings.Contains(metadata, want) {
			t.Errorf("metadata missing %q:\n%s", want, metadata)
		}
	}
}

func TestGuessTrackURIResolvedStreamKeepsURI(t *testing.T) {
	in := "x-sonosapi-stream:s1234?sid=254&flags=8224&sn=0"
	uri, metadata := GuessTrackURI(in)
	if uri != in {
		t.Errorf("uri = %q, want it unchanged", uri)
	}
	// The query string must not leak into the item id.
	if !strings.Contains(metadata, `<item id="10092020s1234"`) {
		t.Errorf("metadata = %s", metadata)
	}
}

func TestGuessTrackURISpotifyTrack(t *testing.T) {
	uri, metadata := GuessTrackURI("spotify:track:5AdoS3gS47X40nBNlNmPQ8")

	if want := "x-sonos-spotify:spotify%3atrack%3a5AdoS3gS47X40nBNlNmPQ8?sid=9&flags=8224&sn=7"; uri != want {
		t.Errorf("uri = %q, want %q", uri, want)
	}
	if !strings.Contains(metadata, `<item id="00032020spotify%3atrack%3a5AdoS3gS47X40nBNlNmPQ8"`) {
		t.Errorf("metadata = %s", metadata)
	}
	if !strings.Contains(metadata, "SA_RINCON2311_X_#Svc2311-0-Token") {
		t.Errorf("metadata missing the Spotify cdudn: %s", metadata)
	}
}

func TestGuessTrackURISpotifyPlaylistHasParent(t *testing.T) {
	_, metadata := GuessTrackURI("spotify:playlist:37i9dQZF1DXcBWIGoYBM5M")
	if !strings.Contains(metadata, `parentID="10fe2664playlists"`) {
		t.Errorf("metadata = %s", metadata)
	}
}

func TestGuessTrackURIPassesThroughUnknown(t *testing.T) {
	for _, in := range []string{
		"x-rincon-queue:RINCON_123#0",
		"x-rincon:RINCON_123",
		"x-sonos-htastream:RINCON_123:spdif",
		"radio:not-a-station",
		"",
	} {
		uri, metadata := GuessTrackURI(in)
		if uri != in || metadata != "" {
			t.Errorf("GuessTrackURI(%q) = (%q, %q), want it untouched", in, uri, metadata)
		}
	}
}

func TestGuessTrackURIMp3Radio(t *testing.T) {
	in := "x-rincon-mp3radio://https://stream.example/live.mp3"
	uri, metadata := GuessTrackURI(in)
	if uri != in {
		t.Errorf("uri = %q, want it unchanged", uri)
	}
	if !strings.Contains(metadata, `<item id="-1" restricted="true">`) {
		t.Errorf("metadata = %s", metadata)
	}
}
