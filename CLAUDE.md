# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

sml-exporter reads smart meter data via SML (Smart Message Language) from a serial device and exports it as Prometheus metrics (HTTP `:9761`) and/or publishes to an MQTT broker. OBIS code mappings are configured via YAML.

## Build & Run

```bash
# Build locally
go build -o sml-exporter

# Build Docker image (targets linux/arm64)
make build

# Push Docker image
make push

# Run
./sml-exporter -config config.yaml -serial /dev/ttyUSB0
```

Tests use `sml.bin` (real SML wire capture) as test data. Run with `go test -race ./...`.

## Architecture

Single-package (`main`) Go application in `main.go`. All logic lives in one file.

**Data flow:** Serial port → `SmartmeterReader.Run()` reads SML stream via `go-sml` library → `obisCallback()` dispatches each OBIS reading → registered handlers process it (Prometheus gauge update, MQTT publish, health timestamp update).

**Key types:**
- `SmartmeterReader` — owns the serial connection, runs the read loop in a goroutine, dispatches to handlers
- `SmartmeterValueHandler` — callback type `func(obis, val string)` for loose coupling between reader and outputs
- `ObisConfig` — per-OBIS-code YAML config specifying type, variable name, metric, and MQTT topic

**Handler pattern:** `RegisterHandler()` adds callbacks. The reader calls all handlers on each OBIS reading. Three handlers are registered in `main()`: Prometheus metrics, MQTT publishing (optional), and health check timestamp.

**Prometheus:** Uses a custom registry (no default Go runtime metrics). Metrics are `GaugeVec` with a `server_id` label resolved from a stored variable.

**Health check:** `/healthz` endpoint returns 200 if data was received within a configurable timeout, 503 otherwise. Uses mutex-protected timestamp.

## Dependencies

- `github.com/databus23/go-sml` — SML protocol parser
- `github.com/jacobsa/go-serial` — serial port I/O (9600 baud, 8N1)
- `github.com/eclipse/paho.mqtt.golang` — MQTT client
- `github.com/prometheus/client_golang` — Prometheus metrics
- `gopkg.in/yaml.v2` — YAML config parsing

## Releasing

Releases are automated via GitHub Actions + goreleaser. To ship a new version:

```bash
git tag v0.8.0
git push origin v0.8.0
```

This triggers `.github/workflows/release.yml` which:
- Cross-compiles binaries for linux/amd64 and linux/arm64
- Creates a GitHub Release with changelog and binaries
- Builds and pushes multi-arch Docker image to `databus23/sml-exporter` on Docker Hub

**Required GitHub secrets:** `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`

**Key files:** `.goreleaser.yml` (build/docker/changelog config), `Dockerfile.goreleaser` (slim image for releases), `Dockerfile` (multi-stage build for local dev)

## Configuration

See `example-config.yaml` for OBIS code mapping format. Each OBIS code entry can specify:
- `type`: `float` or `string`
- `var`: store as named variable (used for `server_id` label)
- `metric`: Prometheus metric name and help text
- `mqtt.topic`: MQTT publish topic
