# mqtt-sonos

A lightweight bridge between Sonos speakers and a local MQTT broker, written in
Go for low memory usage, with a built-in web UI for control.

It is a drop-in replacement for [`svrooij/sonos2mqtt`](https://github.com/svrooij/sonos2mqtt):
the MQTT topic structure and payloads are kept compatible, so existing
automations and Home Assistant discovery keep working.

## Features

- Bridges Sonos devices to a local MQTT broker (sonos2mqtt-compatible contract)
- Pushed state via UPnP GENA event subscriptions (no polling)
- Group/topology aware: transport commands are routed to the group coordinator
- Home Assistant MQTT discovery
- Web UI for live control (play/pause, next/previous, volume, mute, grouping)
- Single static binary, ~single-digit MB RSS
- No external Sonos library — minimal SOAP/SSDP/GENA implemented in the stdlib

## Quick Start

### Docker

```bash
docker run -d \
  --network host \
  -v /path/to/config:/var/lib/mqtt-sonos \
  pharndt/mqtt-sonos:latest
```

Host networking (or a routable pod IP) is required so the speakers can reach the
GENA event listener and SSDP works.

### From Source

```bash
cd app
make dev
```

This builds the frontend, builds the backend, and starts the server using
`production/config/config.json`.

## Configuration

Create a `config.json` file:

```json
{
  "mqtt": {
    "url": "tcp://localhost:1883",
    "topic": "sonos",
    "qos": 0,
    "retain": true
  },
  "sonos": {
    "device": "192.168.1.50",
    "listener_host": "192.168.1.100",
    "listener_port": 6329,
    "discovery": true,
    "discovery_prefix": "homeassistant"
  },
  "web": {
    "enabled": true,
    "port": 8080
  },
  "loglevel": "info"
}
```

Environment variables can be used in the config file with `${VAR_NAME}` syntax.

| Key | Description |
|-----|-------------|
| `mqtt.url` | Broker URL (`tcp://host:1883`) |
| `mqtt.topic` | Topic prefix (default `sonos`) |
| `sonos.device` | IP of one Sonos speaker; the household topology is read from it, skipping SSDP multicast (recommended in Docker/Kubernetes). Equivalent to `SONOS2MQTT_DEVICE`. |
| `sonos.listener_host` | LAN IP the speakers call back to for events. Equivalent to `SONOS_LISTENER_HOST`. Auto-detected when empty. |
| `sonos.listener_port` | GENA callback port (default `6329`) |
| `sonos.discovery` | Publish Home Assistant discovery (default `true`) |
| `sonos.discovery_prefix` | Discovery topic prefix (default `homeassistant`) |
| `web.port` | Web UI port (default `8080`) |

## MQTT Contract

Topic prefix defaults to `sonos`. `<uuid>` is the speaker UUID (`RINCON_…`).

### Published

| Topic | Retained | Description |
|-------|----------|-------------|
| `sonos/<uuid>` | yes | Full device state (JSON) |
| `sonos/connected` | yes | `0` disconnected, `1` MQTT only, `2` MQTT + speakers |
| `sonos/<uuid>/error` | no | Last command error for a device |
| `homeassistant/music_player/<uuid>/sonos/config` | yes | Home Assistant discovery |

Additionally, the underlying gateway publishes `sonos/bridge/state`
(`online`/`offline`) with an MQTT Last Will, used for liveness.

State payload (matches sonos2mqtt's `SonosState`):

```json
{
  "uuid": "RINCON_000E5000000001400",
  "model": "Sonos One",
  "name": "Living Room",
  "groupName": "Living Room",
  "coordinatorUuid": "RINCON_000E5000000001400",
  "volume": { "Master": 25 },
  "mute": { "Master": false },
  "currentTrack": { "Artist": "Daft Punk", "Title": "Get Lucky", "Album": "Random Access Memories", "AlbumArtUri": "http://…/getaa?…", "Duration": "0:06:09", "TrackUri": "x-sonos-spotify:…" },
  "transportState": "PLAYING",
  "playmode": "NORMAL",
  "bass": 0,
  "treble": 0,
  "ts": 1656414000000
}
```

### Subscribed (commands)

Per-device control topic — `<id>` is the UUID or the cleaned room name
(`Living Room` → `living-room`):

```
sonos/<id>/control      payload: { "command": "play" }
                                 { "command": "volume", "input": 30 }
                                 { "command": "joingroup", "input": "Kitchen" }
```

Set topic — the command is taken from the topic, the payload is the input:

```
sonos/set/<id>/volume   payload: 30
```

Universal command topic:

```
sonos/cmd/pauseall
```

Supported commands: `play`, `pause`, `stop`, `toggle`, `next`, `previous`,
`seek`, `selecttrack`, `playmode`, `shuffle`, `repeat`, `crossfade`, `queue`,
`clearqueue`, `switchtoqueue`, `switchtoline`, `switchtotv`,
`setavtransporturi`, `volume`, `volumeup`, `volumedown`, `groupvolume`,
`groupvolumeup`, `groupvolumedown`, `mute`, `unmute`, `setbass`, `settreble`,
`sleep`, `snooze`, `joingroup`, `leavegroup`, `setledstate`,
`setbuttonlockstate`, `setnightmode`, `command`, `adv-command`. Transport
commands are automatically routed to the group coordinator.

Not implemented: `notify`, `notifytwo`, `speak`, `speaktwo` — these need a TTS
endpoint and queue save/restore.

#### Advanced commands

`adv-command` calls any UPnP action on a speaker directly, for anything the
named commands don't cover:

```json
{
  "command": "adv-command",
  "input": {
    "cmd": "RenderingControlService.SetVolume",
    "val": { "InstanceID": 0, "Channel": "Master", "DesiredVolume": 40 }
  }
}
```

`cmd` is `<Service>.<Action>`; every service the speaker exposes is reachable
(`AVTransport`, `RenderingControl`, `GroupRenderingControl`, `DeviceProperties`,
`ZoneGroupTopology`, `ContentDirectory`, `AlarmClock`, `MusicServices`,
`SystemProperties`, `Queue`, `GroupManagement`, `HTControl`, `VirtualLineIn`,
`AudioIn`, `ConnectionManager`, `QPlay`). The `Service` suffix is optional.
`val` holds the action's arguments — names are case sensitive, and their order
is preserved because UPnP is order-sensitive.

Add a `reply` name to have the action's response published to
`sonos/<uuid>/<reply>`:

```json
{
  "command": "adv-command",
  "input": {
    "cmd": "RenderingControlService.GetVolume",
    "val": { "InstanceID": 0, "Channel": "Master" },
    "reply": "GetVolumeResponse"
  }
}
```

```
sonos/<uuid>/GetVolumeResponse   { "CurrentVolume": 40 }
```

The same passthrough is available as the `sonosCommand` control field
(`{ "sonosCommand": "AVTransportService.Play", "input": { "InstanceID": 0, "Speed": "1" } }`),
and `command` wraps another command
(`{ "command": "command", "input": { "cmd": "volume", "val": 30 } }`).

## Web UI

Available at `http://localhost:8080` (default). Lists all rooms with live
now-playing, transport controls, a volume slider, mute, and grouping.

### REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/livez` | Liveness probe |
| `GET` | `/api/devices` | List devices + state |
| `GET` | `/api/groups` | Current grouping |
| `GET` | `/api/devices/{id}/state` | Device state |
| `POST` | `/api/devices/{id}/play` \| `/pause` \| `/stop` \| `/next` \| `/previous` \| `/leave` | Transport / group |
| `POST` | `/api/devices/{id}/volume` | `{ "volume": 30 }` |
| `POST` | `/api/devices/{id}/mute` | `{ "mute": true }` |
| `POST` | `/api/devices/{id}/join` | `{ "target": "Kitchen" }` |
| `POST` | `/api/devices/{id}/command` | `{ "command": "...", "input": ... }` |
| `GET` | `/api/events` | SSE event stream |

## Development

```bash
cd app

make dev            # build frontend + backend, run
make dev-frontend   # frontend dev server (hot reload)
make dev-backend    # backend only
make test           # go test ./...
make docker         # build Docker image
```
