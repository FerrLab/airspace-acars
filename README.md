# Airspace ACARS

Desktop ACARS (Aircraft Communications Addressing and Reporting System) client for flight simulators. Connects to MSFS 2020 and X-Plane to track flights, report positions, and communicate with virtual airline networks.

Built with [Wails v3](https://wails.io) (Go + React).

## Features

- **Flight tracking** — Adaptive position reporting with automatic frequency adjustment based on flight phase
- **Simulator support** — MSFS 2020 (SimConnect) and X-Plane 11/12 (UDP) with auto-detection
- **Multi-tenant auth** — Connect to multiple virtual airline networks via device code authentication
- **In-app chat** — Pilot messaging and communication
- **Audio alerts** — Cabin audio and instruction playback
- **Auto-update** — OTA updates via GitHub Releases with beta channel support
- **Offline recording** — Local SQLite database for flight data persistence

## Requirements

- [Go 1.26+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- [Wails CLI v3](https://v3.wails.io/getting-started/installation/)

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

## Development

```bash
wails3 dev
```

Starts the app with hot-reload on the frontend (Vite dev server on port 9245).

## Build

```bash
# Production build for current OS
wails3 task windows:build

# With version injection
wails3 task windows:build VERSION=1.0.0
```

Output: `bin/airspace-acars-temp.exe`

## Project Structure

```
├── main.go                  # App entry point, service registration
├── auth_service.go          # Device code auth, tenant management
├── flight_data_service.go   # Simulator connection, live data streaming
├── flight_service.go        # Flight lifecycle, position reporting
├── chat_service.go          # Messaging
├── audio_service.go         # Audio fetch and playback
├── settings_service.go      # Persistent configuration
├── update_service.go        # OTA auto-update via GitHub Releases
├── sim_connector.go         # Simulator adapter interface
├── xplane_adapter.go        # X-Plane UDP adapter
├── db.go                    # SQLite initialization
│
├── frontend/                # React + TypeScript + Tailwind
│   ├── src/
│   │   ├── components/      # UI components
│   │   ├── context/         # Auth, theme providers
│   │   └── hooks/           # Custom hooks
│   └── bindings/            # Auto-generated Wails bindings
│
└── build/                   # Platform build configs & Taskfiles
```

## Releases

Releases are automated via GitHub Actions:

| Branch | Channel | Version format |
|--------|---------|----------------|
| `production` | Stable | `v1.YYYYMMDD.N` |
| `main` | Beta | `v0.YYYYMMDD.N-beta.SHA` |

The app checks for updates from Settings > About. Beta builds see beta releases; stable builds only see stable releases.

## License

Proprietary. All rights reserved.
