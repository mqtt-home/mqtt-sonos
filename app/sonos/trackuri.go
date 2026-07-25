package sonos

import (
	"fmt"
	"strings"
)

// defaultCdUdn is the content-directory UDN used for anything that is not a
// music-service item.
const defaultCdUdn = "RINCON_AssociatedZPUDN"

// spotifyRegion is the account region baked into Spotify URIs. 2311 is the
// value node-sonos-ts defaults to.
const spotifyRegion = "2311"

// didlItem is the subset of DIDL-Lite fields needed to describe a track that is
// being set or enqueued.
type didlItem struct {
	TrackURI  string
	ItemID    string
	ParentID  string
	Title     string
	UpnpClass string
	CdUdn     string
}

// GuessTrackURI translates the shorthand URIs sonos2mqtt accepts — `radio:s1234`
// for TuneIn, `spotify:track:xyz` and friends — into the real Sonos URI plus the
// DIDL-Lite metadata the speaker needs alongside it. URIs that are already
// playable, or that aren't recognised, are returned unchanged with no metadata.
func GuessTrackURI(uri string) (trackURI, metadata string) {
	item := guessDidlItem(strings.TrimSpace(uri))
	if item == nil {
		return uri, ""
	}
	if item.TrackURI == "" {
		item.TrackURI = uri
	}
	return item.TrackURI, item.metadataXML()
}

func guessDidlItem(uri string) *didlItem {
	switch {
	// TuneIn station, e.g. radio:s24896
	case strings.HasPrefix(uri, "radio:"):
		station := strings.TrimPrefix(uri, "radio:")
		if !strings.HasPrefix(station, "s") {
			return nil
		}
		return &didlItem{
			TrackURI:  fmt.Sprintf("x-sonosapi-stream:%s?sid=254&flags=8224&sn=0", station),
			ItemID:    "10092020" + station,
			Title:     "Some radio station",
			UpnpClass: "object.item.audioItem.audioBroadcast",
		}

	// An already-resolved TuneIn stream still needs metadata to play.
	case strings.HasPrefix(uri, "x-sonosapi-stream:"):
		station := strings.TrimPrefix(uri, "x-sonosapi-stream:")
		if i := strings.Index(station, "?"); i >= 0 {
			station = station[:i]
		}
		return &didlItem{
			ItemID:    "10092020" + station,
			Title:     "Some radio station",
			UpnpClass: "object.item.audioItem.audioBroadcast",
		}

	// Plain MP3 stream URL.
	case strings.HasPrefix(uri, "x-rincon-mp3radio://"):
		return &didlItem{ItemID: "-1"}
	}

	if item := guessSpotifyItem(uri); item != nil {
		return item
	}
	return nil
}

// guessSpotifyItem maps a spotify: URI onto the container/stream URI and item id
// the Sonos Spotify service expects.
func guessSpotifyItem(uri string) *didlItem {
	parts := strings.Split(uri, ":")
	if (len(parts) != 3 && len(parts) != 5) || parts[0] != "spotify" {
		return nil
	}

	encoded := strings.ReplaceAll(uri, ":", "%3a")
	item := &didlItem{CdUdn: fmt.Sprintf("SA_RINCON%s_X_#Svc%s-0-Token", spotifyRegion, spotifyRegion)}

	switch parts[1] {
	case "album":
		item.TrackURI = "x-rincon-cpcontainer:1004206c" + encoded + "?sid=9&flags=8300&sn=7"
		item.ItemID = "0004206c" + encoded
		item.UpnpClass = "object.container.album.musicAlbum"
	case "artistRadio":
		item.TrackURI = "x-sonosapi-radio:" + encoded + "?sid=9&flags=8300&sn=7"
		item.ItemID = "100c206c" + encoded
		item.ParentID = "10052064" + strings.Replace(encoded, "artistRadio", "artist", 1)
		item.Title = "Artist radio"
		item.UpnpClass = "object.item.audioItem.audioBroadcast.#artistRadio"
	case "artistTopTracks":
		item.TrackURI = "x-rincon-cpcontainer:100e206c" + encoded + "?sid=9&flags=8300&sn=7"
		item.ItemID = "100e206c" + encoded
		item.ParentID = "10052064" + strings.Replace(encoded, "artistTopTracks", "artist", 1)
		item.UpnpClass = "object.container.playlistContainer"
	case "playlist":
		item.TrackURI = "x-rincon-cpcontainer:1006206c" + encoded + "?sid=9&flags=8300&sn=7"
		item.ItemID = "1006206c" + encoded
		item.ParentID = "10fe2664playlists"
		item.Title = "Spotify playlist"
		item.UpnpClass = "object.container.playlistContainer"
	case "track":
		item.TrackURI = "x-sonos-spotify:" + encoded + "?sid=9&flags=8224&sn=7"
		item.ItemID = "00032020" + encoded
		item.UpnpClass = "object.item.audioItem.musicTrack"
	case "user":
		item.TrackURI = "x-rincon-cpcontainer:10062a6c" + encoded + "?sid=9&flags=10860&sn=7"
		item.ItemID = "10062a6c" + encoded
		item.ParentID = "10082664playlists"
		item.Title = "User's playlist"
		item.UpnpClass = "object.container.playlistContainer"
	default:
		return nil
	}
	return item
}

// metadataXML renders the item as the DIDL-Lite document Sonos expects in a
// *MetaData argument.
func (t *didlItem) metadataXML() string {
	itemID := t.ItemID
	if itemID == "" {
		itemID = "-1"
	}
	cdudn := t.CdUdn
	if cdudn == "" {
		cdudn = defaultCdUdn
	}

	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
		`xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" ` +
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`)
	fmt.Fprintf(&b, `<item id="%s" restricted="true"`, escapeXML(itemID))
	if t.ParentID != "" {
		fmt.Fprintf(&b, ` parentID="%s"`, escapeXML(t.ParentID))
	}
	b.WriteString(">")
	if t.Title != "" {
		fmt.Fprintf(&b, "<dc:title>%s</dc:title>", escapeXML(t.Title))
	}
	if t.UpnpClass != "" {
		fmt.Fprintf(&b, "<upnp:class>%s</upnp:class>", escapeXML(t.UpnpClass))
	}
	fmt.Fprintf(&b, `<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">%s</desc>`, escapeXML(cdudn))
	b.WriteString("</item></DIDL-Lite>")
	return b.String()
}
