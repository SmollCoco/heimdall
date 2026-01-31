# Heimdall 🛡️

Distributed telemetry agent for Linux (Promtail-style): watch log files/directories → parse → label → batch → ship to Grafana Loki.

## Features (v1.0.0)

- Watches **files** and **directories** (auto-discovers new files)
- Streams new log lines (tail-from-end semantics)
- Detects basic log levels: `DEBUG/INFO/WARN/ERROR/FATAL`
- Ships to **Grafana Loki** with:
    - batching (size OR timeout)
    - exponential backoff retries
- Two binaries:
    - `heimdall` (daemon)
    - `heimdall-cli` (helper: validate config, sample config, version)

## Architecture

```

[Log Files] -> [Watcher] -> [Processor (worker pool)] -> [Shipper] -> [Loki]
fsnotify        parse + labels          batch + retry

```

## Requirements

- Linux
- Go (whatever is in `go.mod`)
- Docker (for Loki + Grafana in local dev)

## Quickstart (local Loki + Grafana)

1. Start Loki + Grafana:

```bash
docker compose up -d
```

2. Build binaries:

```bash
make build-all VERSION=v1.0.0
```

3. Use the example config:

```bash
./bin/heimdall-cli validate -config ./configs/heimdall.example.yaml
```

4. Run Heimdall:

```bash
./bin/heimdall -config ./configs/heimdall.example.yaml
```

5. Grafana:

- Open: [http://localhost:3000](http://localhost:3000)
- Query example:

```text
{service="system"}
```

## Configuration

Example (`configs/heimdall.example.yaml`):

```yaml
inputs:
    - path: /var/log/
      path_type: directory
      labels:
          service: system
          environment: production

output:
    loki:
        url: http://localhost:3100
        batch_size: 100
        batch_timeout: 5s

worker_pool_size: 4
channel_buffer_size: 1000

retry:
    max_attempts: 5
    initial_backoff: 1s
    max_backoff: 30s

self_monitoring:
    enabled: false
    interval: 30s
```

### Notes

- `batch_size` is number of entries.
- `batch_timeout` is a Go duration string (`5s`, `200ms`, `1m`).
- Watcher uses tail-from-end behavior (it does not replay existing file contents on startup).

## Commands

### Daemon (`heimdall`)

```bash
./bin/heimdall -config /etc/heimdall/config.yaml
./bin/heimdall -version
```

Exit codes:

- `0`: clean shutdown (SIGINT/SIGTERM)
- `1`: runtime error
- `2`: config/flag error

### Helper CLI (`heimdall-cli`)

```bash
./bin/heimdall-cli help
./bin/heimdall-cli version
./bin/heimdall-cli sample-config
./bin/heimdall-cli validate -config ./configs/heimdall.example.yaml
```

## Running as a systemd service

A unit file exists at:

```text
scripts/heimdall.service
```

Typical flow:

- Copy config to `/etc/heimdall/config.yaml`
- Install unit
- Enable + start service

(Exact commands depend on your distro and install layout.)

## Development

### Run all tests

```bash
go test ./...
go test ./... -race
```

### End-to-End test (requires Loki running)

```bash
go test ./cmd/heimdall -run TestHeimdall_E2E_Pipeline_ToLoki -v
```

### Build

```bash
make build-all VERSION=v1.0.0
```

## Troubleshooting

### “Logs shipped but Loki query returns nothing”

Use a range query in Grafana or wait a few seconds. Loki queries are time-based.

### “My Loki data disappears when I restart containers”

If you run `docker compose down -v`, volumes are deleted. Avoid `-v` if you want persistence.

### “validate fails because paths don’t exist”

`heimdall-cli validate` checks paths with `os.Stat`. Validate on the target machine or adjust paths.

## License

Personal Use License - See `LICENSE`.

TL;DR: Free for personal use and modification. Commercial use and redistribution prohibited.
