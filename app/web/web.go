package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/mqtt-home/mqtt-sonos/config"
	"github.com/mqtt-home/mqtt-sonos/sonos"
	"github.com/philipparndt/go-logger"
	loggerchi "github.com/philipparndt/go-logger/chi"
)

type SSEClient struct {
	ID      string
	Channel chan string
}

// WebServer exposes a REST + SSE API and the static SPA for controlling Sonos.
type WebServer struct {
	mgr          *sonos.Manager
	router       *chi.Mux
	sseClients   map[string]*SSEClient
	sseClientsMu sync.RWMutex

	unhealthySince *time.Time
	unhealthyMu    sync.Mutex
}

func NewWebServer(mgr *sonos.Manager) *WebServer {
	ws := &WebServer{
		mgr:        mgr,
		router:     chi.NewRouter(),
		sseClients: make(map[string]*SSEClient),
	}
	ws.setupRoutes()
	return ws
}

func (ws *WebServer) livenessGrace() time.Duration {
	if s := config.Get().Web.LivenessGraceSeconds; s > 0 {
		return time.Duration(s) * time.Second
	}
	return 4 * time.Minute
}

func (ws *WebServer) setupRoutes() {
	ws.router.Use(loggerchi.LoggerWithLevel(slog.LevelDebug))
	ws.router.Use(middleware.Recoverer)
	ws.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	ws.router.Route("/api", func(r chi.Router) {
		r.Get("/health", ws.healthCheck)
		r.Get("/livez", ws.liveness)

		r.Get("/devices", ws.getDevices)
		r.Get("/groups", ws.getGroups)
		r.Route("/devices/{id}", func(r chi.Router) {
			r.Get("/state", ws.getDeviceState)
			r.Post("/play", ws.simpleCommand("play"))
			r.Post("/pause", ws.simpleCommand("pause"))
			r.Post("/stop", ws.simpleCommand("stop"))
			r.Post("/next", ws.simpleCommand("next"))
			r.Post("/previous", ws.simpleCommand("previous"))
			r.Post("/leave", ws.simpleCommand("leavegroup"))
			r.Post("/volume", ws.setVolume)
			r.Post("/mute", ws.setMute)
			r.Post("/join", ws.joinGroup)
			r.Post("/command", ws.genericCommand)
		})

		r.Get("/events", ws.handleSSE)
	})

	distDir := "./web/dist/"
	fileServer := http.FileServer(http.Dir(distDir))
	ws.router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		path := "." + r.URL.Path
		if _, err := http.Dir(distDir).Open(path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, distDir+"index.html")
	})
}

// DeviceSummary is the per-device payload returned to the web UI.
type DeviceSummary struct {
	UUID        string       `json:"uuid"`
	Name        string       `json:"name"`
	Model       string       `json:"model"`
	Coordinator string       `json:"coordinator"`
	GroupName   string       `json:"groupName"`
	State       *sonos.State `json:"state"`
}

func (ws *WebServer) summaries() []DeviceSummary {
	snaps := ws.mgr.Snapshots()
	out := make([]DeviceSummary, 0, len(snaps))
	for _, d := range ws.mgr.Devices() {
		st := snaps[d.UUID]
		coord := ""
		group := ""
		if st != nil {
			coord = st.CoordinatorUUID
			group = st.GroupName
		}
		out = append(out, DeviceSummary{
			UUID:        d.UUID,
			Name:        d.Name,
			Model:       d.Model,
			Coordinator: coord,
			GroupName:   group,
			State:       st,
		})
	}
	return out
}

func (ws *WebServer) getDevices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, ws.summaries())
}

func (ws *WebServer) getGroups(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, ws.mgr.Groups())
}

func (ws *WebServer) getDeviceState(w http.ResponseWriter, r *http.Request) {
	d := ws.resolve(w, r)
	if d == nil {
		return
	}
	st := ws.mgr.Snapshot(d.UUID)
	if st == nil {
		http.Error(w, `{"error":"no state"}`, http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, st)
}

func (ws *WebServer) simpleCommand(command string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := ws.resolve(w, r)
		if d == nil {
			return
		}
		if err := ws.mgr.Command(d, command, nil); err != nil {
			logger.Error("Web command failed", "device", d.Name, "command", command, "error", err)
			http.Error(w, `{"error":"command failed"}`, http.StatusInternalServerError)
			return
		}
		jsonOK(w)
	}
}

func (ws *WebServer) setVolume(w http.ResponseWriter, r *http.Request) {
	d := ws.resolve(w, r)
	if d == nil {
		return
	}
	var req struct {
		Volume int `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	input, _ := json.Marshal(req.Volume)
	if err := ws.mgr.Command(d, "volume", input); err != nil {
		http.Error(w, `{"error":"command failed"}`, http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (ws *WebServer) setMute(w http.ResponseWriter, r *http.Request) {
	d := ws.resolve(w, r)
	if d == nil {
		return
	}
	var req struct {
		Mute bool `json:"mute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	command := "unmute"
	if req.Mute {
		command = "mute"
	}
	if err := ws.mgr.Command(d, command, nil); err != nil {
		http.Error(w, `{"error":"command failed"}`, http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (ws *WebServer) joinGroup(w http.ResponseWriter, r *http.Request) {
	d := ws.resolve(w, r)
	if d == nil {
		return
	}
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Target == "" {
		http.Error(w, `{"error":"target is required"}`, http.StatusBadRequest)
		return
	}
	input, _ := json.Marshal(req.Target)
	if err := ws.mgr.Command(d, "joingroup", input); err != nil {
		http.Error(w, `{"error":"command failed"}`, http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (ws *WebServer) genericCommand(w http.ResponseWriter, r *http.Request) {
	d := ws.resolve(w, r)
	if d == nil {
		return
	}
	var req struct {
		Command string          `json:"command"`
		Input   json.RawMessage `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command == "" {
		http.Error(w, `{"error":"command is required"}`, http.StatusBadRequest)
		return
	}
	if err := ws.mgr.Command(d, req.Command, req.Input); err != nil {
		logger.Error("Web command failed", "device", d.Name, "command", req.Command, "error", err)
		http.Error(w, `{"error":"command failed"}`, http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (ws *WebServer) resolve(w http.ResponseWriter, r *http.Request) *sonos.Device {
	id := chi.URLParam(r, "id")
	d := ws.mgr.Resolve(id)
	if d == nil {
		http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
		return nil
	}
	return d
}

func (ws *WebServer) healthCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"status":     "ok",
		"goroutines": runtime.NumGoroutine(),
		"devices":    ws.mgr.DeviceCount(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

func (ws *WebServer) liveness(w http.ResponseWriter, _ *http.Request) {
	healthy := ws.mgr.DeviceCount() > 0
	now := time.Now()
	grace := ws.livenessGrace()

	ws.unhealthyMu.Lock()
	statusCode, newSince, stuckFor := evaluateLiveness(healthy, ws.unhealthySince, now, grace)
	ws.unhealthySince = newSince
	ws.unhealthyMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"healthy":         healthy,
		"devices":         ws.mgr.DeviceCount(),
		"stuckForSeconds": int(stuckFor.Seconds()),
		"graceSeconds":    int(grace.Seconds()),
		"timestamp":       now.UTC().Format(time.RFC3339),
	})
}

// evaluateLiveness is the pure liveness decision: the probe fails (503) only
// once the bridge has been continuously unhealthy for longer than the grace
// window, so transient blips don't trigger a restart.
func evaluateLiveness(healthy bool, unhealthySince *time.Time, now time.Time, grace time.Duration) (statusCode int, newSince *time.Time, stuckFor time.Duration) {
	if healthy {
		return http.StatusOK, nil, 0
	}
	if unhealthySince == nil {
		t := now
		unhealthySince = &t
	}
	stuckFor = now.Sub(*unhealthySince)
	statusCode = http.StatusOK
	if stuckFor > grace {
		statusCode = http.StatusServiceUnavailable
	}
	return statusCode, unhealthySince, stuckFor
}

// --- SSE ---

// BroadcastState pushes a device state update to all connected SSE clients.
func (ws *WebServer) BroadcastState(uuid string, state *sonos.State) {
	payload := struct {
		Type   string       `json:"type"`
		Device string       `json:"device"`
		State  *sonos.State `json:"state"`
	}{Type: "state", Device: uuid, State: state}

	message, err := json.Marshal(payload)
	if err != nil {
		return
	}
	messageStr := string(message)

	ws.sseClientsMu.RLock()
	for _, client := range ws.sseClients {
		select {
		case client.Channel <- messageStr:
		default:
		}
	}
	ws.sseClientsMu.RUnlock()
}

func (ws *WebServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	channel := make(chan string, 10)

	ws.sseClientsMu.Lock()
	ws.sseClients[clientID] = &SSEClient{ID: clientID, Channel: channel}
	ws.sseClientsMu.Unlock()

	// Send initial state for all devices.
	for uuid, st := range ws.mgr.Snapshots() {
		payload := struct {
			Type   string       `json:"type"`
			Device string       `json:"device"`
			State  *sonos.State `json:"state"`
		}{Type: "state", Device: uuid, State: st}
		msg, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", string(msg))
	}

	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	defer func() {
		ws.sseClientsMu.Lock()
		delete(ws.sseClients, clientID)
		close(channel)
		ws.sseClientsMu.Unlock()
	}()

	for {
		select {
		case msg := <-channel:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
			if ok {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (ws *WebServer) Start(port int) error {
	addr := ":" + strconv.Itoa(port)
	logger.Info("Starting web server", "address", addr)
	return http.ListenAndServe(addr, ws.router)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonOK(w http.ResponseWriter) {
	writeJSON(w, map[string]string{"status": "success"})
}
