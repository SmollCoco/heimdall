# Heimdall 🛡️
*Distributed Telemetry Agent for Linux*

[badges will go here later: build status, Go version, license]

## Table of Contents
- [About](#about)
- [Why Heimdall?](#why-heimdall)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Getting Started](#getting-started)
- [Development Roadmap](#development-roadmap)
- [Contributing](#contributing)

---

## About
**Heimdall** is a distributed telemetry agent. It manages the logging of input directories and their processing into Grafana Loki with the ability to query those logs. It's built using Go with use of concurrency (Goroutines).

---

## Why Heimdall?
This project was built to master Go concurrency patterns, Linux systems programming, and distributed telemetry. 
After doing some research, I learned that there are no lightweight, low-setup tools that allow for directory logging. Existing tools like Protail and Fluent Bit are resource-heavy and quite complex. My intention with this project is to develop a lightweight, zero-dependency alternative.

---

## Architecture

**High-Level Flow:**
```
[Log Files] → [Watcher] → [Processor] → [Shipper] → [Grafana Loki]
                  ↓            ↓            ↓
              fsnotify     Transform    Retry Logic
```

**Concurrency Model:**
Heimdall uses a pipeline where:
- **Watcher goroutines** detect file changes (using `fsnotify`) and send raw data into a buffered channel.
- A **processor worker pool** reads from said channel, transforms the data, and forwards to the shipper channel.
- **Shipper goroutines** batch and send entries to Loki with exponential backoff.

Components communicate through typed channels, not direct imports, ensuring loose coupling and independence.

---

## Tech Stack

**Language:** Go 1.24.9

**Core Dependencies:**
- `fsnotify` - File system event monitoring
- Standard library for everything else

**Infrastructure:**
- Grafana Loki (Docker)
- Grafana (Docker)

**Deployment:**
- Systemd (daemon mode)
- Standalone binary

---

## Project Structure
```
heimdall/
├── cmd/
│   ├── heimdall/
│   │   └── main.go
│   └── heimdall-cli/
│       └── main.go
├── internal/
│   ├── watcher/
│   ├── processor/
│   ├── shipper/
│   ├── config/
│   ├── setup/
│   ├── monitor/
│   └── types/
├── web/
│   └── templates/
├── configs/
│   └── heimdall.example.yaml
├── scripts/
│   └── heimdall.service
├── go.mod
└── README.md
```

**Package Responsibilities:**
- `cmd/heimdall`: Main orchestrator. To be used by the daemon.
- `cmd/heimdall-cli`: Main orchestrator with ability to query the Loki database directly from the terminal.
- `internal/watcher`: Uses `fsnotify`. Responsable of detecting the changes in the files and the directories.
- `internal/processor`: The transformar: parses raw bytes into structured JSON blocks.
- `internal/shipper`: Ships the JSON into Grafana Loki.
- `internal/monitor`: Used to monitor the activity of the tool (CPU, RAM).
- `internal/config`: Loads the config file.
- `internal/setup`: Responsible of the initial setup of the tool.
- `internal/types`: Shared imported types.
- `scripts/heimdall.service`: The Systemd unit file.
- `web/templates`: Web front-end.

---

## Getting Started

### Prerequisites
- Go 1.24.9+
- Docker
- Linux (developed on Kali, tested on Kali and Ubuntu/Debian)

### Installation
```bash
# [YOUR TASK: Fill in later when we have build commands]
```

### Running Heimdall
```bash
# [YOUR TASK: Placeholder for now]
```

---

## Development Roadmap

### Phase 1: Foundation ✅ (In Progress)
- [x] Project structure
- [ ] Configuration loading (YAML)
- [ ] Basic file watcher (single file)
- [ ] Log entry parsing (JSON lines)

### Phase 2: Core Features
- [ ] Directory watching with fsnotify
- [ ] Log rotation handling
- [ ] Loki shipper with retry logic
- [ ] Worker pool for processing

### Phase 3: Resilience
- [ ] Exponential backoff
- [ ] Self-monitoring (CPU/RAM)
- [ ] Journald integration

### Phase 4: UX
- [ ] Setup mode web GUI
- [ ] CLI query tool
- [ ] Systemd service

### Phase 5: Production
- [ ] Performance testing
- [ ] Documentation
- [ ] Release builds

---

## Contributing
This is a learning project. Feedback and suggestions are welcome via issues!

---

## License
Personal Use License - See [LICENSE](LICENSE) for details.

**TL;DR:** Free for personal use and modification. Commercial use and redistribution prohibited.
