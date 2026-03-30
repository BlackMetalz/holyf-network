# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

A Linux-only TUI network monitoring tool (Go + tview) with three runtime modes:
- **Live TUI** (`holyf-network`) — real-time dashboard with connection tracking, bandwidth, diagnosis
- **Daemon** (`holyf-network daemon start/stop/status/prune`) — background snapshot writer (JSONL)
- **Replay** (`holyf-network replay`) — read-only historical snapshot viewer

Requires Linux 4.9+, root/sudo, reads `/proc` and `/sys`, uses netlink kernel APIs directly.

## Build & Test

```bash
make build          # Build binary to bin/holyf-network
make local          # Build + run with sudo
make local ARGS="-r 5"  # Build + run with args
make test           # go test ./...
go test ./internal/collector/...  # Run tests for a single package
```

## Architecture

### Kernel API Abstraction (`internal/kernelapi/`)

Three interfaces in `interfaces.go`: `SocketManager`, `ConntrackManager`, `Firewall`. Each has a Linux netlink implementation and a CLI-exec fallback. Platform detection (`detect_linux.go` vs `detect_stub.go`) picks the backend at startup.

### Dependency Injection

Kernel API implementations are injected at startup via package-level `SetManagers()`:
- `internal/collector/managers.go` — receives `SocketManager` + `ConntrackManager` (read-only data collection)
- `internal/actions/managers.go` — receives `SocketManager` + `ConntrackManager` + `Firewall` (side-effecting ops: block/kill)

### Data Pipeline (Live)

`internal/tui/app_core.go` drives a refresh loop that calls collector functions in sequence:
1. Conntrack snapshot + rates
2. TCP retrans from `/proc/net/snmp`
3. Connection states from `/proc/net/tcp*`
4. Interface stats from `/sys/class/net/`
5. Top talkers with PID mapping
6. Conntrack flow merge for Docker/NAT visibility (`ct/nat` rows)
7. Byte delta + throughput calculation

Three async refresh lanes: main (full refresh at `-r` interval), fast (interface stats every 1s), warm-up (early settle).

### Snapshot Pipeline (Daemon)

`cmd/daemon.go` collects, aggregates by `(peer_ip, port, proc_name)` per direction (IN/OUT), caps by `--top-limit`, writes JSONL (`connections-YYYYMMDD.jsonl`). Daily rotation, age-based retention.

### History / Replay

- `internal/history/` — writer (JSONL append + lock file), reader (index + seek-by-offset), prune
- `internal/tui/history_app.go` + `internal/tui/replay/` — read-only TUI, snapshot navigation

### TUI Structure (`internal/tui/`)

- `app_core.go` — main refresh loop, hotkey dispatch, layout switching
- `blocking/` — peer block/unblock + kill convergence flow
- `diagnosis/` — rule-based health diagnosis engine
- `panels/` — pure render helpers for each panel
- `shared/` — formatting, health checks, ring buffer
- `overlays/` — modals and help screens

### Health Thresholds

TOML config in `config/health_thresholds.toml`, parsed by `internal/config/health_thresholds.go`. Controls WARN/CRIT coloring for retrans, drops, conntrack, bandwidth.

## Existing AI Context Docs

See `docs/ai-context/` for detailed design docs:
- `HIGH_LEVEL_DESIGN.md` — full 3-mode design, metric formulas, kernel API strategy
- `PROJECT_STRUCTURE.md` — package ownership and test map
- `KERNEL_API.md` — kernel API backend details
