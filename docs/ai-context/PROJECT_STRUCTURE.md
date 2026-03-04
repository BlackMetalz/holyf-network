# Project Structure

This is the current high-signal layout (non-essential folders omitted):

```text
.
├── main.go
├── cmd/
│   └── root.go
├── config/
│   └── health_thresholds.toml
├── internal/
│   ├── actions/
│   │   └── peer_blocker.go
│   ├── collector/
│   │   ├── connections.go
│   │   ├── top_connections.go
│   │   ├── tcp_retransmits.go
│   │   ├── interface_stats.go
│   │   └── conntrack.go
│   ├── config/
│   │   └── health_thresholds.go
│   ├── network/
│   │   └── interface.go
│   └── tui/
│       ├── app_core.go
│       ├── app_top_connections.go
│       ├── app_history.go
│       ├── app_shared.go
│       ├── app_blocking_state.go
│       ├── app_blocking_targets.go
│       ├── app_blocking_kill_flow.go
│       ├── app_blocking_runtime.go
│       ├── app_blocking_blocked_modal.go
│       ├── layout.go
│       ├── help.go
│       ├── panel_connections.go
│       ├── panel_interface.go
│       ├── panel_conntrack.go
│       └── panel_top_connections.go
├── .github/
│   └── workflows/
│       └── release.yml
└── README.MD
```

## Ownership By Package

- `cmd`
  - CLI flags, version resolution, startup wiring.
  - Builds `tui.App` and launches UI.

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
    - `conntrack -S` command

- `internal/actions`
  - Side-effecting runtime actions:
    - block/unblock peer by IP + local port
    - kill/drop active flows
    - list active firewall blocks
  - Shells out to `iptables`/`ip6tables`, `ss`, `conntrack`.

- `internal/config`
  - Health threshold model + parser for TOML-like file.

- `internal/tui`
  - App state machine, keyboard handling, modal flows, rendering panels.
  - Grouped by concern:
    - `app_core.go`: lifecycle, refresh loop, global key handling, status bar.
    - `app_top_connections.go`: top-connection selection/filter/sort/search orchestration.
    - `app_blocking_*.go`: block/kill flows and blocked peers modal.
    - `app_history.go`: action log modal + persistence (`~/.holyf-network/history.log`).
    - `panel_*.go`: pure rendering text for each panel.
    - `layout.go`: grid composition.

## Test Map

- `internal/tui/*_test.go`
  - Interaction behavior and regressions (kill flow, sorting hotkeys, filters, action log).

- `internal/config/health_thresholds_test.go`
  - Config parsing/normalization behavior.

When adding features:
- prefer adding tests in the same package,
- keep render-only helpers side-effect free for easier testing.
