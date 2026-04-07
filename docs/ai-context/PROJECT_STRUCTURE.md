# Project Structure

This is the current high-signal layout (non-essential folders omitted):

```text
.
├── main.go
├── cmd/
│   ├── root.go
│   ├── daemon.go
│   └── replay.go
├── config/
│   └── health_thresholds.toml
├── internal/
│   ├── actions/
│   │   └── peer_blocker.go
│   ├── collector/
│   │   ├── connections.go
│   │   ├── top_connections.go
│   │   ├── listen_ports.go
│   │   ├── conntrack_flows.go
│   │   ├── conntrack_merge.go
│   │   ├── bandwidth_tracker.go
│   │   ├── socket_counters.go
│   │   ├── socket_bandwidth_tracker.go
│   │   ├── tcp_retransmits.go
│   │   ├── interface_stats.go
│   │   └── conntrack.go
│   ├── config/
│   │   └── health_thresholds.go
│   ├── history/
│   │   ├── types.go
│   │   ├── files.go
│   │   ├── writer.go
│   │   └── reader.go
│   ├── kernelapi/
│   │   ├── interfaces.go
│   │   ├── types.go
│   │   ├── backend_info.go
│   │   ├── socket_netlink.go
│   │   ├── socket_exec.go
│   │   ├── conntrack_netlink.go
│   │   ├── conntrack_exec.go
│   │   ├── firewall_nft.go
│   │   ├── firewall_exec.go
│   │   ├── detect_linux.go
│   │   ├── detect_stub.go
│   │   ├── new_linux.go
│   │   └── new_stub.go
│   ├── network/
│   │   └── interface.go
│   ├── podlookup/
│   │   ├── types.go
│   │   ├── namespace_scan.go
│   │   └── pod_resolve.go
│   └── tui/
│       ├── app_core.go
│       ├── app_connections.go
│       ├── app_trace_packet.go
│       ├── app_trace_history.go
│       ├── history_app.go
│       ├── blocking/
│       ├── diagnosis/
│       ├── podlookup/
│       ├── layout/
│       ├── livetrace/
│       ├── overlays/
│       ├── panels/
│       ├── replay/
│       ├── shared/
│       ├── trace/
│       └── traffic/
├── .github/
│   └── workflows/
│       └── release.yml
└── README.MD
```

## Ownership By Package

- `cmd`
  - CLI flags, version resolution, startup wiring.
  - Entry points for live mode (`root`), daemon mode (`daemon start/stop/status/prune`), and replay mode (`replay`).
  - Optional daemon defaults loader lives in `cmd/daemon_config.go` (`/etc/holyf-network/daemon.json` merge over built-ins).

- `internal/network`
  - Interface detection/listing helpers used by CLI.

- `internal/collector`
  - Read-only metric collection from Linux sources.
  - No UI logic.
  - Main sources:
    - `/proc/net/tcp`, `/proc/net/tcp6`
    - `/proc/net/snmp`
    - `/sys/class/net/<iface>/statistics/*`
    - `/proc/sys/net/netfilter/nf_conntrack_*`
    - Conntrack stats and flows via `kernelapi.ConntrackManager` (netlink or exec fallback)
    - Socket TCP counters via `kernelapi.SocketManager` (netlink or exec fallback)
  - Docker/NAT visibility:
    - `conntrack_merge.go` injects host-facing NAT tuples missing in `/proc/net/tcp*`
    - synthetic process label `ct/nat` marks conntrack-derived ownership

- `internal/kernelapi`
  - Direct Linux kernel API access for socket, conntrack, and firewall operations.
  - Replaces CLI tools (`ss`, `conntrack`, `iptables`) with netlink sockets.
  - Auto-detects kernel capabilities at startup; falls back to exec if unavailable.
  - See `docs/ai-context/KERNEL_API.md` for full details.

- `internal/actions`
  - Side-effecting runtime actions:
    - block/unblock peer by IP + local port
    - kill/drop active flows
    - list active firewall blocks
  - Uses `kernelapi` interfaces (netlink on Linux 4.9+, exec fallback otherwise).

- `internal/podlookup`
  - On-demand K8s pod identification by port number.
  - Enumerates network namespaces via `/proc/*/ns/net`, scans `/proc/{pid}/net/tcp{,6}` per NS.
  - Resolves PID → pod name → deployment via layered strategy:
    - Parse `/proc/{pid}/cgroup` for pod UID and container ID (containerd + CRI-O formats).
    - Read `HOSTNAME` from `/proc/{pid}/environ` (fast, no exec).
    - Fallback to `crictl inspect` / `crictl inspectp` for pod metadata + labels.
    - Infer deployment name from pod name pattern (`<deploy>-<rs-hash>-<pod-hash>`).
  - Reuses `collector.ParseTCPConnections` and `collector.GetProcessName`.

- `internal/config`
  - Health threshold model + parser for TOML-like file.

- `internal/history`
  - Snapshot persistence/indexing for daemon + replay flow.
  - JSON Lines (`.jsonl`) writer with retention and lock file.
  - Reader/indexer for timeline replay.
  - Aggregate snapshots persist queue + bandwidth metrics per peer/port/proc row.

- `internal/tui`
  - App state machine, keyboard handling, modal flows, rendering panels.
  - Two view modes: Dashboard (Ctrl+1) and Bandwidth Chart (Ctrl+2).
  - Root files (5 source):
    - `app_core.go`: lifecycle, refresh loop, global key handling, status bar, shared constants/utils, UIContext adapter, diagnosis history modal, action log modal, view mode switching.
    - `app_connections.go`: top-connection selection/filter/sort/search orchestration + panel layout for note/preview + kill target selection.
    - `app_trace_packet.go`: packet trace capture UI (form, progress, result display).
    - `app_trace_history.go`: trace history persistence/modals + trace packet analyzer logic.
    - `history_app.go`: read-only replay mode UI, key handling, UIContext, navigation, timeline search.
  - Sub-packages by concern:
    - `blocking/`: block/kill flow manager, runtime control, target definitions, UI context.
    - `diagnosis/`: rule-based live diagnosis synthesis engine.
    - `podlookup/`: K8s pod lookup prompt and result modals (uses `internal/podlookup` core).
    - `layout/`: grid composition for live (dashboard + chart) and replay modes.
    - `overlays/`: help text, modal, text overlay components.
    - `panels/`: pure rendering for each panel (system health, diagnosis, top connections, history aggregate, sparkline, Braille chart).
    - `replay/`: historical data replay/timeline UI (search, navigation, trace visualization).
    - `shared/`: shared utilities (formatting, health checks, conntrack stats, trace formatting, states, update checks, ring buffer).
    - `trace/`: trace data storage and rendering.
    - `traffic/`: traffic manager and monitoring.
    - `livetrace/`: live packet trace engine.
    - `actionlog/`: action/event logging.

## Test Map

- `internal/tui/*_test.go`
  - Interaction behavior and regressions (kill flow, sorting hotkeys, filters, action log).

- `internal/config/health_thresholds_test.go`
  - Config parsing/normalization behavior.
- `internal/history/*_test.go`
  - Snapshot writer/reader behavior (append, rotate, prune, index, corrupt skip).

When adding features:
- prefer adding tests in the same package,
- keep render-only helpers side-effect free for easier testing.
