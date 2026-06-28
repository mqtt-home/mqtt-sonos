package sonos

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// Group is one Sonos zone group: a coordinator plus its members.
type Group struct {
	Coordinator string   // UUID of the coordinator device
	Name        string   // group name (coordinator's zone name, "+ N" when grouped)
	Members     []string // member UUIDs (including the coordinator)
}

type zoneGroupXML struct {
	Coordinator string               `xml:"Coordinator,attr"`
	ID          string               `xml:"ID,attr"`
	Members     []zoneGroupMemberXML `xml:"ZoneGroupMember"`
}

type zoneGroupMemberXML struct {
	UUID      string `xml:"UUID,attr"`
	Location  string `xml:"Location,attr"`
	ZoneName  string `xml:"ZoneName,attr"`
	Invisible string `xml:"Invisible,attr"`
}

// fetchZoneGroupState queries one speaker for the whole household topology and
// returns the discovered devices and the current grouping. One player's
// topology describes every speaker, so this is the Docker/VLAN-safe path.
func fetchZoneGroupState(baseURL string) ([]*Device, []Group, error) {
	body, err := soapCall(baseURL, zoneGroupTopology, "GetZoneGroupState")
	if err != nil {
		return nil, nil, err
	}
	raw := soapValue(body, "ZoneGroupState")
	if raw == "" {
		return nil, nil, fmt.Errorf("empty ZoneGroupState")
	}
	return parseZoneGroupState(raw)
}

func parseZoneGroupState(raw string) ([]*Device, []Group, error) {
	// The payload may arrive wrapped in <ZoneGroupState> or as a bare
	// <ZoneGroups> document, and topology events sometimes nest it differently.
	// Stream the document and collect every <ZoneGroup> element regardless.
	dec := xml.NewDecoder(strings.NewReader(raw))
	var zoneGroups []zoneGroupXML
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("parse ZoneGroupState: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "ZoneGroup" {
			var zg zoneGroupXML
			if err := dec.DecodeElement(&zg, &se); err != nil {
				return nil, nil, fmt.Errorf("parse ZoneGroup: %w", err)
			}
			zoneGroups = append(zoneGroups, zg)
		}
	}

	var devices []*Device
	var groups []Group
	names := map[string]string{} // uuid -> zone name

	for _, zg := range zoneGroups {
		g := Group{Coordinator: zg.Coordinator}
		for _, m := range zg.Members {
			if m.Invisible == "1" {
				// Hidden bonded members (e.g. a stereo-pair satellite) are not
				// independently controllable; skip them as zones.
				continue
			}
			host, port := hostPortFromLocation(m.Location)
			if host == "" {
				continue
			}
			devices = append(devices, &Device{
				UUID: m.UUID,
				Name: m.ZoneName,
				Host: host,
				Port: port,
			})
			names[m.UUID] = m.ZoneName
			g.Members = append(g.Members, m.UUID)
		}
		if len(g.Members) == 0 {
			continue
		}
		g.Name = names[zg.Coordinator]
		if len(g.Members) > 1 {
			g.Name = fmt.Sprintf("%s + %d", names[zg.Coordinator], len(g.Members)-1)
		}
		groups = append(groups, g)
	}
	return devices, groups, nil
}

func hostPortFromLocation(location string) (string, int) {
	u, err := url.Parse(location)
	if err != nil {
		return "", 0
	}
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 1400
	}
	return host, port
}
