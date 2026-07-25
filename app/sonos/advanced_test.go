package sonos

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// newFakeSpeaker starts a stub speaker that records the last SOAP request and
// answers with the given response body (the inner XML of the SOAP <Body>).
func newFakeSpeaker(t *testing.T, responseBody string) (*Device, *soapRequest) {
	t.Helper()
	last := &soapRequest{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		last.path = r.URL.Path
		last.action = r.Header.Get("SOAPACTION")
		last.body = string(body)

		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
			responseBody+`</s:Body></s:Envelope>`)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return &Device{UUID: "RINCON_TEST01400", Name: "Test", Host: u.Hostname(), Port: port}, last
}

type soapRequest struct {
	path   string
	action string
	body   string
}

func TestAdvancedCommandSetVolume(t *testing.T) {
	d, req := newFakeSpeaker(t, `<u:SetVolumeResponse xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1"></u:SetVolumeResponse>`)

	val := json.RawMessage(`{"InstanceID":0,"Channel":"Master","DesiredVolume":40}`)
	if _, err := d.AdvancedCommand("RenderingControlService.SetVolume", val); err != nil {
		t.Fatalf("AdvancedCommand: %v", err)
	}

	if req.path != "/MediaRenderer/RenderingControl/Control" {
		t.Errorf("path = %q", req.path)
	}
	if want := `"urn:schemas-upnp-org:service:RenderingControl:1#SetVolume"`; req.action != want {
		t.Errorf("SOAPACTION = %q, want %q", req.action, want)
	}
	// Argument order is significant in UPnP and must follow the JSON key order.
	want := `<InstanceID>0</InstanceID><Channel>Master</Channel><DesiredVolume>40</DesiredVolume>`
	if !strings.Contains(req.body, want) {
		t.Errorf("body = %q, want it to contain %q", req.body, want)
	}
}

func TestAdvancedCommandReturnsResponseValues(t *testing.T) {
	d, _ := newFakeSpeaker(t, `<u:GetVolumeResponse xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1">`+
		`<CurrentVolume>40</CurrentVolume></u:GetVolumeResponse>`)

	result, err := d.AdvancedCommand("RenderingControlService.GetVolume", json.RawMessage(`{"InstanceID":0,"Channel":"Master"}`))
	if err != nil {
		t.Fatalf("AdvancedCommand: %v", err)
	}
	if got := result["CurrentVolume"]; got != 40 {
		t.Errorf("CurrentVolume = %v (%T), want 40 (int)", got, got)
	}
}

func TestAdvancedCommandAcceptsServiceNameVariants(t *testing.T) {
	for _, name := range []string{"AVTransportService", "AVTransport", "avtransport"} {
		d, req := newFakeSpeaker(t, `<u:PlayResponse/>`)
		if _, err := d.AdvancedCommand(name+".Play", json.RawMessage(`{"InstanceID":0,"Speed":"1"}`)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if req.path != "/MediaRenderer/AVTransport/Control" {
			t.Errorf("%s: path = %q", name, req.path)
		}
	}
}

func TestAdvancedCommandErrors(t *testing.T) {
	d, _ := newFakeSpeaker(t, `<u:Response/>`)

	tests := []struct {
		name string
		cmd  string
		val  string
	}{
		{"missing action", "RenderingControlService", `{}`},
		{"unknown service", "NoSuchService.Play", `{}`},
		{"val not an object", "AVTransportService.Play", `"nope"`},
	}
	for _, tc := range tests {
		if _, err := d.AdvancedCommand(tc.cmd, json.RawMessage(tc.val)); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

func TestAdvancedCommandWithoutVal(t *testing.T) {
	d, req := newFakeSpeaker(t, `<u:GetZoneGroupStateResponse/>`)
	if _, err := d.AdvancedCommand("ZoneGroupTopologyService.GetZoneGroupState", nil); err != nil {
		t.Fatalf("AdvancedCommand: %v", err)
	}
	if !strings.Contains(req.body, "<u:GetZoneGroupState ") {
		t.Errorf("body = %q", req.body)
	}
}

func TestSoapArgValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`0`, "0"},
		{`40`, "40"},
		{`"Master"`, "Master"},
		{`true`, "1"},
		{`false`, "0"},
		{`null`, ""},
		{`"x-rincon:RINCON_1"`, "x-rincon:RINCON_1"},
	}
	for _, tc := range tests {
		if got := soapArgValue(json.RawMessage(tc.in)); got != tc.want {
			t.Errorf("soapArgValue(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOrderedSoapArgsKeepsOrder(t *testing.T) {
	args, err := orderedSoapArgs(json.RawMessage(`{"c":1,"a":2,"b":3}`))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range args {
		names = append(names, a.name)
	}
	if strings.Join(names, ",") != "c,a,b" {
		t.Errorf("order = %v, want [c a b]", names)
	}
}

func TestSoapValuesEscapesArguments(t *testing.T) {
	d, req := newFakeSpeaker(t, `<u:SetAVTransportURIResponse/>`)
	val := json.RawMessage(`{"InstanceID":0,"CurrentURI":"x-sonosapi:a&b","CurrentURIMetaData":"<tag>"}`)
	if _, err := d.AdvancedCommand("AVTransportService.SetAVTransportURI", val); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.body, "a&amp;b") || !strings.Contains(req.body, "&lt;tag&gt;") {
		t.Errorf("arguments not escaped: %q", req.body)
	}
}
