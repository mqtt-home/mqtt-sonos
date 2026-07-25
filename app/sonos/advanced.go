package sonos

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// Every UPnP service a Sonos speaker exposes, keyed by the node-sonos-ts
// service name used in sonos2mqtt's `adv-command` (`<Service>.<Action>`).
// Services the bridge does not model itself are still reachable this way.
var servicesByName = map[string]service{
	"alarmclock":            {"urn:schemas-upnp-org:service:AlarmClock:1", "/AlarmClock/Control", "/AlarmClock/Event"},
	"audioin":               {"urn:schemas-upnp-org:service:AudioIn:1", "/AudioIn/Control", "/AudioIn/Event"},
	"avtransport":           avTransport,
	"connectionmanager":     {"urn:schemas-upnp-org:service:ConnectionManager:1", "/MediaRenderer/ConnectionManager/Control", "/MediaRenderer/ConnectionManager/Event"},
	"contentdirectory":      {"urn:schemas-upnp-org:service:ContentDirectory:1", "/MediaServer/ContentDirectory/Control", "/MediaServer/ContentDirectory/Event"},
	"deviceproperties":      deviceProperties,
	"groupmanagement":       {"urn:schemas-upnp-org:service:GroupManagement:1", "/GroupManagement/Control", "/GroupManagement/Event"},
	"grouprenderingcontrol": groupRenderingControl,
	"htcontrol":             {"urn:schemas-upnp-org:service:HTControl:1", "/HTControl/Control", "/HTControl/Event"},
	"musicservices":         {"urn:schemas-upnp-org:service:MusicServices:1", "/MusicServices/Control", "/MusicServices/Event"},
	"qplay":                 {"urn:schemas-tencent-com:service:QPlay:1", "/QPlay/Control", "/QPlay/Event"},
	"queue":                 {"urn:schemas-sonos-com:service:Queue:1", "/MediaRenderer/Queue/Control", "/MediaRenderer/Queue/Event"},
	"renderingcontrol":      renderingControl,
	"systemproperties":      {"urn:schemas-upnp-org:service:SystemProperties:1", "/SystemProperties/Control", "/SystemProperties/Event"},
	"virtuallinein":         {"urn:schemas-upnp-org:service:VirtualLineIn:1", "/MediaRenderer/VirtualLineIn/Control", "/MediaRenderer/VirtualLineIn/Event"},
	"zonegrouptopology":     zoneGroupTopology,
}

// lookupService resolves a service by its node-sonos-ts name. Both
// `RenderingControlService` and `RenderingControl` are accepted, case-insensitive.
func lookupService(name string) (service, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.TrimSuffix(key, "service")
	svc, ok := servicesByName[key]
	return svc, ok
}

// AdvancedCommand executes a raw UPnP action against one of the speaker's
// services and returns the action response as name/value pairs. It is the
// escape hatch sonos2mqtt exposes as `adv-command`, e.g.
//
//	cmd = "RenderingControlService.SetVolume"
//	val = {"InstanceID": 0, "Channel": "Master", "DesiredVolume": 40}
//
// Argument order is significant in UPnP, so the JSON key order of val is
// preserved (matching how the JS implementation serialises its input object).
func (d *Device) AdvancedCommand(cmd string, val json.RawMessage) (map[string]any, error) {
	serviceName, action, ok := strings.Cut(strings.TrimSpace(cmd), ".")
	if !ok || serviceName == "" || action == "" {
		return nil, fmt.Errorf("cmd must be %q, got %q", "<Service>.<Action>", cmd)
	}
	svc, ok := lookupService(serviceName)
	if !ok {
		return nil, fmt.Errorf("unknown service %q", serviceName)
	}

	args, err := orderedSoapArgs(val)
	if err != nil {
		return nil, fmt.Errorf("%s.%s: %w", serviceName, action, err)
	}

	body, err := soapCall(d.baseURL(), svc, action, args...)
	if err != nil {
		return nil, err
	}
	return soapResponseValues(body), nil
}

// orderedSoapArgs turns the `val` object into SOAP arguments, keeping the
// order the keys appear in the JSON document.
func orderedSoapArgs(raw json.RawMessage) ([]soapArg, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("val is not valid JSON: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("val must be an object")
	}

	var args []soapArg
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("val must be an object")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		args = append(args, soapArg{key, soapArgValue(value)})
	}
	return args, nil
}

// soapArgValue renders a JSON value as a UPnP argument string: booleans become
// 1/0, null becomes empty, strings are unquoted and everything else is passed
// through verbatim.
func soapArgValue(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	switch s {
	case "", "null":
		return ""
	case "true":
		return "1"
	case "false":
		return "0"
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			return str
		}
	}
	return s
}

// soapResponseValues collects the direct children of a SOAP action response
// into a map. Values that look like integers are returned as numbers, so
// e.g. GetVolume replies with {"CurrentVolume": 40}.
func soapResponseValues(body []byte) map[string]any {
	out := map[string]any{}
	dec := xml.NewDecoder(bytes.NewReader(body))

	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			if _, ok := tok.(xml.EndElement); ok {
				depth--
			}
			continue
		}
		depth++
		// depth 1 is the action response element itself; its children carry the values.
		if depth != 2 {
			continue
		}
		var v string
		if err := dec.DecodeElement(&v, &se); err != nil {
			break
		}
		depth--
		if n, err := strconv.Atoi(v); err == nil {
			out[se.Name.Local] = n
		} else {
			out[se.Name.Local] = v
		}
	}
	return out
}
