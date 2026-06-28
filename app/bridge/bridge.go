package bridge

import (
	"encoding/json"
	"strings"

	"github.com/mqtt-home/mqtt-sonos/config"
	"github.com/mqtt-home/mqtt-sonos/sonos"
	"github.com/philipparndt/go-logger"
	"github.com/philipparndt/mqtt-gateway/mqtt"
)

// Connection status values published to `<prefix>/connected`, matching
// sonos2mqtt: 0 = disconnected, 1 = MQTT only, 2 = MQTT + speakers.
const (
	statusDisconnected = "0"
	statusMQTTOnly     = "1"
	statusConnected    = "2"
)

// Bridge wires the Sonos manager to the local MQTT broker, preserving the
// sonos2mqtt topic and payload contract.
type Bridge struct {
	cfg config.Config
	mgr *sonos.Manager
	// extra is an optional second state listener (used by the web UI for SSE).
	extra func(uuid string, state *sonos.State)
}

func New(cfg config.Config, mgr *sonos.Manager) *Bridge {
	return &Bridge{cfg: cfg, mgr: mgr}
}

// SetStateListener registers an additional listener for state changes.
func (b *Bridge) SetStateListener(fn func(uuid string, state *sonos.State)) {
	b.extra = fn
}

// Manager exposes the underlying Sonos manager.
func (b *Bridge) Manager() *sonos.Manager { return b.mgr }

func (b *Bridge) topic() string { return b.cfg.MQTT.Topic }

// Start connects to MQTT, discovers speakers, publishes initial state and
// discovery messages, and subscribes to the command topics.
func (b *Bridge) Start() error {
	mqtt.Start(b.cfg.MQTT, "mqtt_sonos")

	b.mgr.SetStateChangeCallback(b.onState)

	if err := b.mgr.Start(); err != nil {
		return err
	}

	// Publish discovery + initial retained state for every device.
	for _, d := range b.mgr.Devices() {
		if b.cfg.Sonos.DiscoveryEnabled() {
			b.publishDiscovery(d)
		}
	}
	for uuid, st := range b.mgr.Snapshots() {
		b.publishState(uuid, st)
	}

	b.subscribe()

	b.publishConnected(statusConnected)
	logger.Info("Sonos MQTT bridge started", "devices", len(b.mgr.Devices()))
	return nil
}

// Stop publishes a graceful disconnect and stops the manager.
func (b *Bridge) Stop() {
	b.publishConnected(statusDisconnected)
	b.mgr.Stop()
}

func (b *Bridge) onState(uuid string, state *sonos.State) {
	b.publishState(uuid, state)
	if b.extra != nil {
		b.extra(uuid, state)
	}
}

func (b *Bridge) publishState(uuid string, state *sonos.State) {
	data, err := json.Marshal(state)
	if err != nil {
		logger.Error("Failed to marshal state", "error", err)
		return
	}
	mqtt.PublishAbsolute(b.topic()+"/"+uuid, string(data), true)
}

func (b *Bridge) publishConnected(value string) {
	mqtt.PublishAbsolute(b.topic()+"/connected", value, true)
}

// subscribe registers the three command-topic patterns.
func (b *Bridge) subscribe() {
	prefix := b.topic()

	// Per-device control topic: <prefix>/<id>/control
	mqtt.Subscribe(prefix+"/+/control", func(topic string, payload []byte) {
		id := topicSegment(topic, prefix, 0)
		b.handleControl(id, payload)
	})

	// Set topic: <prefix>/set/<id>/<command>, payload is the command input.
	mqtt.Subscribe(prefix+"/set/+/+", func(topic string, payload []byte) {
		parts := strings.Split(strings.TrimPrefix(topic, prefix+"/set/"), "/")
		if len(parts) != 2 {
			return
		}
		b.dispatch(parts[0], parts[1], json.RawMessage(payload))
	})

	// Universal command topic: <prefix>/cmd/<command>
	mqtt.Subscribe(prefix+"/cmd/+", func(topic string, payload []byte) {
		command := strings.TrimPrefix(topic, prefix+"/cmd/")
		b.handleGlobal(command, payload)
	})

	logger.Info("Subscribed to Sonos command topics", "prefix", prefix)
}

type controlPayload struct {
	Command      string          `json:"command"`
	Cmd          string          `json:"cmd"`
	Input        json.RawMessage `json:"input"`
	SonosCommand string          `json:"sonosCommand"`
}

func (b *Bridge) handleControl(id string, payload []byte) {
	var p controlPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		logger.Error("Invalid control payload", "id", id, "error", err)
		return
	}
	command := p.Command
	if command == "" {
		command = p.Cmd
	}
	if command == "" {
		logger.Warn("Control message without command", "id", id)
		return
	}
	b.dispatch(id, command, p.Input)
}

func (b *Bridge) dispatch(id, command string, input json.RawMessage) {
	d := b.mgr.Resolve(id)
	if d == nil {
		logger.Warn("Command for unknown device", "id", id, "command", command)
		return
	}
	logger.Debug("Executing command", "device", d.Name, "command", command)
	if err := b.mgr.Command(d, command, input); err != nil {
		logger.Error("Command failed", "device", d.Name, "command", command, "error", err)
		mqtt.PublishAbsolute(b.topic()+"/"+d.UUID+"/error", err.Error(), false)
	}
}

func (b *Bridge) handleGlobal(command string, _ []byte) {
	switch strings.ToLower(command) {
	case "pauseall":
		b.mgr.PauseAll()
	default:
		logger.Warn("Unsupported global command", "command", command)
	}
}

// topicSegment returns the Nth segment after the prefix in a topic.
func topicSegment(topic, prefix string, n int) string {
	rest := strings.TrimPrefix(topic, prefix+"/")
	parts := strings.Split(rest, "/")
	if n < len(parts) {
		return parts[n]
	}
	return ""
}
