package sonos

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Sonos UPnP services. Each speaker exposes these on TCP port 1400.
type service struct {
	serviceType string
	controlURL  string
	eventURL    string
}

var (
	avTransport = service{
		serviceType: "urn:schemas-upnp-org:service:AVTransport:1",
		controlURL:  "/MediaRenderer/AVTransport/Control",
		eventURL:    "/MediaRenderer/AVTransport/Event",
	}
	renderingControl = service{
		serviceType: "urn:schemas-upnp-org:service:RenderingControl:1",
		controlURL:  "/MediaRenderer/RenderingControl/Control",
		eventURL:    "/MediaRenderer/RenderingControl/Event",
	}
	groupRenderingControl = service{
		serviceType: "urn:schemas-upnp-org:service:GroupRenderingControl:1",
		controlURL:  "/MediaRenderer/GroupRenderingControl/Control",
		eventURL:    "/MediaRenderer/GroupRenderingControl/Event",
	}
	zoneGroupTopology = service{
		serviceType: "urn:schemas-upnp-org:service:ZoneGroupTopology:1",
		controlURL:  "/ZoneGroupTopology/Control",
		eventURL:    "/ZoneGroupTopology/Event",
	}
	deviceProperties = service{
		serviceType: "urn:schemas-upnp-org:service:DeviceProperties:1",
		controlURL:  "/DeviceProperties/Control",
		eventURL:    "/DeviceProperties/Event",
	}
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// soapArg is a single ordered SOAP argument. Order matters in UPnP requests.
type soapArg struct {
	name  string
	value string
}

// soapEnvelope/soapBody decode the response wrapper to reach the action result.
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Inner []byte `xml:",innerxml"`
	} `xml:"Body"`
}

// soapCall performs a SOAP action against a speaker service and returns the raw
// XML inside the SOAP <Body> (the action response element).
func soapCall(baseURL string, svc service, action string, args ...soapArg) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&b, `<u:%s xmlns:u="%s">`, action, svc.serviceType)
	for _, a := range args {
		fmt.Fprintf(&b, "<%s>%s</%s>", a.name, escapeXML(a.value), a.name)
	}
	fmt.Fprintf(&b, `</u:%s></s:Body></s:Envelope>`, action)

	req, err := http.NewRequest(http.MethodPost, baseURL+svc.controlURL, strings.NewReader(b.String()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", fmt.Sprintf(`"%s#%s"`, svc.serviceType, action))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("soap %s/%s failed: %s: %s", svc.serviceType, action, resp.Status, string(body))
	}

	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode soap response: %w", err)
	}
	return env.Body.Inner, nil
}

// soapValue extracts the text content of a single named element from a SOAP
// response body fragment.
func soapValue(body []byte, element string) string {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == element {
			var v string
			if err := dec.DecodeElement(&v, &se); err != nil {
				return ""
			}
			return v
		}
	}
}

func escapeXML(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
