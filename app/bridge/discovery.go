package bridge

import (
	"encoding/json"

	"github.com/mqtt-home/mqtt-sonos/sonos"
	"github.com/philipparndt/go-logger"
	"github.com/philipparndt/mqtt-gateway/mqtt"
)

// availableCommands mirrors the command set advertised by sonos2mqtt's Home
// Assistant discovery payload.
var availableCommands = []string{
	"adv-command", "clearqueue", "command", "crossfade", "groupvolume",
	"groupvolumedown", "groupvolumeup", "joingroup", "leavegroup", "mute", "next",
	"pause", "play", "playmode", "previous", "queue", "repeat", "seek",
	"selecttrack", "setavtransporturi", "setbass", "setbuttonlockstate",
	"setledstate", "setnightmode", "settreble", "shuffle", "sleep", "snooze",
	"stop", "switchtoline", "switchtoqueue", "switchtotv", "toggle", "unmute",
	"volume", "volumedown", "volumeup",
}

// publishDiscovery emits a retained Home Assistant MQTT discovery message for a
// device, on `<discoveryPrefix>/music_player/<uuid>/sonos/config` (absolute,
// not under the state topic prefix), matching sonos2mqtt.
func (b *Bridge) publishDiscovery(d *sonos.Device) {
	cfg := b.cfg
	payload := map[string]any{
		"available_commands": availableCommands,
		"command_topic":      cfg.MQTT.Topic + "/" + d.UUID + "/control",
		"device": map[string]any{
			"identifiers":  []string{d.UUID},
			"manufacturer": "Sonos",
			"name":         d.Name,
			"model":        d.Model,
		},
		"device_class":          "speaker",
		"icon":                  "mdi:speaker",
		"json_attributes":       true,
		"json_attributes_topic": cfg.MQTT.Topic + "/" + d.UUID,
		"name":                  d.Name,
		"state_topic":           cfg.MQTT.Topic + "/" + d.UUID,
		"unique_id":             "sonos2mqtt_" + d.UUID + "_speaker",
		"availability_topic":    cfg.MQTT.Topic + "/connected",
		"payload_available":     statusConnected,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to marshal discovery", "error", err)
		return
	}

	topic := cfg.Sonos.DiscoveryPrefix + "/music_player/" + d.UUID + "/sonos/config"
	mqtt.PublishAbsolute(topic, string(data), true)
	logger.Debug("Published HA discovery", "device", d.Name, "topic", topic)
}
