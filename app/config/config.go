package config

import (
	"encoding/json"
	"os"

	"github.com/philipparndt/go-logger"
	"github.com/philipparndt/mqtt-gateway/config"
)

var cfg Config

type Config struct {
	MQTT     config.MQTTConfig `json:"mqtt"`
	Sonos    SonosConfig       `json:"sonos"`
	Web      WebConfig         `json:"web"`
	LogLevel string            `json:"loglevel,omitempty"`
}

type SonosConfig struct {
	// Device is the IP of one known Sonos speaker. When set, the household
	// topology is read from it (ZoneGroupTopology), skipping SSDP multicast —
	// the reliable path inside Docker/Kubernetes. Equivalent to
	// sonos2mqtt's SONOS2MQTT_DEVICE.
	Device string `json:"device,omitempty"`
	// ListenerHost is the LAN IP the speakers call back to for GENA event
	// notifications. Must be reachable from the speakers. Equivalent to
	// SONOS_LISTENER_HOST. Auto-detected when empty.
	ListenerHost string `json:"listener_host,omitempty"`
	// ListenerPort is the local port the GENA callback HTTP server listens on.
	// Defaults to 6329 (the node-sonos-ts default).
	ListenerPort int `json:"listener_port,omitempty"`
	// Discovery toggles Home Assistant MQTT discovery messages. Defaults to true.
	Discovery *bool `json:"discovery,omitempty"`
	// DiscoveryPrefix is the Home Assistant discovery topic prefix. Defaults to
	// "homeassistant".
	DiscoveryPrefix string `json:"discovery_prefix,omitempty"`
	// PollingInterval is how often (seconds) the bridge re-polls each speaker as
	// a fallback to GENA events, keeping state correct even when an event
	// callback is missed or the listener host is unreachable. Defaults to 30.
	// Set to a negative value to disable polling entirely.
	PollingInterval int `json:"polling_interval,omitempty"`
	// FriendlyNames controls the identifier used in distinct status sub-topics:
	// "name" (cleaned room name) or "uuid". Defaults to "name". The main state,
	// control and discovery topics always use the UUID, matching sonos2mqtt.
	FriendlyNames string `json:"friendly_names,omitempty"`
	// Distinct enables the per-property status/<id>/... sub-topics. Defaults to
	// false, matching sonos2mqtt.
	Distinct bool `json:"distinct,omitempty"`
}

// DiscoveryEnabled reports whether Home Assistant discovery should be published.
// Defaults to true when unset, matching sonos2mqtt's effective default.
func (s SonosConfig) DiscoveryEnabled() bool {
	if s.Discovery == nil {
		return true
	}
	return *s.Discovery
}

type WebConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
	// LivenessGraceSeconds is how long the bridge may stay unhealthy (no speaker
	// reachable) before the /livez probe fails. Defaults to 240 (4 min).
	LivenessGraceSeconds int `json:"liveness_grace_seconds,omitempty"`
}

func LoadConfig(file string) (Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		logger.Error("Error reading config file", "error", err)
		return Config{}, err
	}

	data = config.ReplaceEnvVariables(data)

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		logger.Error("Unmarshaling JSON", "error", err)
		return Config{}, err
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.MQTT.Topic == "" {
		cfg.MQTT.Topic = "sonos"
	}

	if cfg.Sonos.ListenerPort == 0 {
		cfg.Sonos.ListenerPort = 6329
	}

	switch {
	case cfg.Sonos.PollingInterval == 0:
		cfg.Sonos.PollingInterval = 30
	case cfg.Sonos.PollingInterval < 0:
		cfg.Sonos.PollingInterval = 0 // explicitly disabled
	}

	if cfg.Sonos.DiscoveryPrefix == "" {
		cfg.Sonos.DiscoveryPrefix = "homeassistant"
	}

	if cfg.Sonos.FriendlyNames == "" {
		cfg.Sonos.FriendlyNames = "name"
	}

	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8080
	}

	return cfg, nil
}

func Get() Config {
	return cfg
}
