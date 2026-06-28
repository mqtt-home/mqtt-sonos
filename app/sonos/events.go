package sonos

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/philipparndt/go-logger"
)

// Short service names used in GENA callback URLs and dispatched to handlers.
const (
	svcAVTransport       = "AVTransport"
	svcRenderingControl  = "RenderingControl"
	svcZoneGroupTopology = "ZoneGroupTopology"
)

var subscribeTimeout = 3600 * time.Second

// EventCallback receives a parsed GENA notification: the device UUID, the short
// service name and the raw inner value (LastChange for AVT/RCS, ZoneGroupState
// for topology).
type EventCallback func(uuid, svcShort, value string)

// EventListener runs a small HTTP server that Sonos speakers POST GENA
// notifications to, and manages the SUBSCRIBE/renew lifecycle.
type EventListener struct {
	host     string
	port     int
	onEvent  EventCallback
	server   *http.Server
	mu       sync.Mutex
	subs     map[string]*genaSub // key: uuid + "|" + svcShort
	stopOnce sync.Once
	stop     chan struct{}
}

type genaSub struct {
	device   *Device
	svc      service
	svcShort string
	sid      string
}

func NewEventListener(host string, port int, onEvent EventCallback) *EventListener {
	return &EventListener{
		host:    host,
		port:    port,
		onEvent: onEvent,
		subs:    map[string]*genaSub{},
		stop:    make(chan struct{}),
	}
}

// callbackHost returns the host the speakers should call back to, auto-detecting
// a non-loopback LAN IP when not configured.
func (e *EventListener) callbackHost() string {
	if e.host != "" {
		return e.host
	}
	if ip := outboundIP(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

// Start launches the callback HTTP server and the renewal loop.
func (e *EventListener) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/sonos/", e.handleNotify)

	e.server = &http.Server{
		Addr:    ":" + strconv.Itoa(e.port),
		Handler: mux,
	}
	ln, err := net.Listen("tcp", e.server.Addr)
	if err != nil {
		return err
	}
	logger.Info("GENA event listener started", "host", e.callbackHost(), "port", e.port)
	go func() {
		if err := e.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Error("Event listener stopped", "error", err)
		}
	}()
	go e.renewLoop()
	return nil
}

func (e *EventListener) Stop() {
	e.stopOnce.Do(func() {
		close(e.stop)
		e.mu.Lock()
		for _, s := range e.subs {
			e.unsubscribe(s)
		}
		e.mu.Unlock()
		if e.server != nil {
			_ = e.server.Close()
		}
	})
}

// handleNotify processes a NOTIFY POST from a speaker. Path is
// /sonos/{uuid}/{svcShort}.
func (e *EventListener) handleNotify(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	w.WriteHeader(http.StatusOK)
	if len(parts) != 3 {
		return
	}
	uuid, svcShort := parts[1], parts[2]

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}

	var value string
	switch svcShort {
	case svcZoneGroupTopology:
		value = soapValue(body, "ZoneGroupState")
	default:
		value = soapValue(body, "LastChange")
	}
	if value == "" {
		return
	}
	if e.onEvent != nil {
		e.onEvent(uuid, svcShort, value)
	}
}

// Subscribe registers a GENA subscription for a device's service.
func (e *EventListener) Subscribe(d *Device, svc service, svcShort string) error {
	callback := fmt.Sprintf("<http://%s:%d/sonos/%s/%s>", e.callbackHost(), e.port, d.UUID, svcShort)

	req, err := http.NewRequest("SUBSCRIBE", d.baseURL()+svc.eventURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("CALLBACK", callback)
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("TIMEOUT", "Second-"+strconv.Itoa(int(subscribeTimeout.Seconds())))

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("subscribe %s/%s failed: %s", d.Name, svcShort, resp.Status)
	}
	sid := resp.Header.Get("SID")

	e.mu.Lock()
	e.subs[d.UUID+"|"+svcShort] = &genaSub{device: d, svc: svc, svcShort: svcShort, sid: sid}
	e.mu.Unlock()

	logger.Debug("Subscribed to GENA events", "device", d.Name, "service", svcShort, "sid", sid)
	return nil
}

// SubscribeDevice subscribes to the standard set of services for a device.
func (e *EventListener) SubscribeDevice(d *Device) {
	for _, s := range []struct {
		svc   service
		short string
	}{
		{avTransport, svcAVTransport},
		{renderingControl, svcRenderingControl},
	} {
		if err := e.Subscribe(d, s.svc, s.short); err != nil {
			logger.Warn("Failed to subscribe", "device", d.Name, "service", s.short, "error", err)
		}
	}
}

// SubscribeTopology subscribes to ZoneGroupTopology on one device so group
// changes are pushed.
func (e *EventListener) SubscribeTopology(d *Device) {
	if err := e.Subscribe(d, zoneGroupTopology, svcZoneGroupTopology); err != nil {
		logger.Warn("Failed to subscribe to topology", "device", d.Name, "error", err)
	}
}

func (e *EventListener) renew(s *genaSub) error {
	req, err := http.NewRequest("SUBSCRIBE", s.device.baseURL()+s.svc.eventURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("SID", s.sid)
	req.Header.Set("TIMEOUT", "Second-"+strconv.Itoa(int(subscribeTimeout.Seconds())))
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("renew failed: %s", resp.Status)
	}
	return nil
}

func (e *EventListener) unsubscribe(s *genaSub) {
	req, err := http.NewRequest("UNSUBSCRIBE", s.device.baseURL()+s.svc.eventURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("SID", s.sid)
	if resp, err := httpClient.Do(req); err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// renewLoop renews all subscriptions before they expire. A subscription that
// fails to renew is re-created from scratch.
func (e *EventListener) renewLoop() {
	interval := subscribeTimeout / 2
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			e.mu.Lock()
			subs := make([]*genaSub, 0, len(e.subs))
			for _, s := range e.subs {
				subs = append(subs, s)
			}
			e.mu.Unlock()
			for _, s := range subs {
				if err := e.renew(s); err != nil {
					logger.Warn("Renew failed, re-subscribing", "device", s.device.Name, "service", s.svcShort, "error", err)
					_ = e.Subscribe(s.device, s.svc, s.svcShort)
				}
			}
		}
	}
}

// outboundIP determines the local IP used to reach the LAN, without sending any
// packets (the UDP socket only selects a route).
func outboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
