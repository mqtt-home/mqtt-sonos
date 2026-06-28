package sonos

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/philipparndt/go-logger"
)

const ssdpZonePlayer = "urn:schemas-upnp-org:device:ZonePlayer:1"

// deviceDescription matches the relevant fields of /xml/device_description.xml.
type deviceDescription struct {
	Device struct {
		ModelName    string `xml:"modelName"`
		RoomName     string `xml:"roomName"`
		FriendlyName string `xml:"friendlyName"`
		UDN          string `xml:"UDN"`
	} `xml:"device"`
}

// Discover finds all Sonos speakers and the current grouping. When seed is a
// non-empty IP it reads the household topology from that one speaker (no
// multicast). Otherwise it performs SSDP discovery and reads topology from the
// first speaker that answers.
func Discover(seed string, timeout time.Duration) ([]*Device, []Group, error) {
	var seedURL string
	if seed != "" {
		seedURL = fmt.Sprintf("http://%s:1400", seed)
		logger.Info("Discovering Sonos topology from seed device", "device", seed)
	} else {
		logger.Info("Discovering Sonos devices via SSDP")
		locations, err := ssdpSearch(timeout)
		if err != nil {
			return nil, nil, err
		}
		if len(locations) == 0 {
			return nil, nil, fmt.Errorf("no Sonos devices found via SSDP")
		}
		seedURL = baseFromLocation(locations[0])
	}

	devices, groups, err := fetchZoneGroupState(seedURL)
	if err != nil {
		return nil, nil, err
	}

	// Enrich each device with its model name from its description document.
	for _, d := range devices {
		if desc, err := fetchDeviceDescription(d.baseURL()); err == nil {
			d.Model = desc.Device.ModelName
			if d.Name == "" {
				d.Name = desc.Device.RoomName
			}
		} else {
			logger.Debug("Failed to fetch device description", "device", d.Name, "error", err)
		}
	}

	return devices, groups, nil
}

func fetchDeviceDescription(baseURL string) (*deviceDescription, error) {
	resp, err := httpClient.Get(baseURL + "/xml/device_description.xml")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var desc deviceDescription
	if err := xml.Unmarshal(body, &desc); err != nil {
		return nil, err
	}
	return &desc, nil
}

// ssdpSearch sends an SSDP M-SEARCH and collects the LOCATION URLs of Sonos
// ZonePlayers that respond. Implemented with the stdlib to avoid an extra
// dependency.
func ssdpSearch(timeout time.Duration) ([]string, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	multicast := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	msearch := strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"HOST: 239.255.255.250:1900",
		"MAN: \"ssdp:discover\"",
		"MX: 1",
		"ST: " + ssdpZonePlayer,
		"", "",
	}, "\r\n")

	// Send a few times: UDP discovery packets are best-effort.
	for i := 0; i < 3; i++ {
		if _, err := conn.WriteTo([]byte(msearch), multicast); err != nil {
			return nil, err
		}
	}

	_ = conn.SetReadDeadline(time.Now().Add(timeout))

	seen := map[string]bool{}
	var locations []string
	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break // deadline reached
		}
		loc := parseSSDPLocation(buf[:n])
		if loc != "" && !seen[loc] {
			seen[loc] = true
			locations = append(locations, loc)
		}
	}
	return locations, nil
}

func parseSSDPLocation(packet []byte) string {
	r := bufio.NewReader(strings.NewReader(string(packet)))
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return ""
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "LOCATION:") {
			return strings.TrimSpace(line[len("LOCATION:"):])
		}
	}
}

func baseFromLocation(location string) string {
	host, port := hostPortFromLocation(location)
	return fmt.Sprintf("http://%s:%d", host, port)
}
