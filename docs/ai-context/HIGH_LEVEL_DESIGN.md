# High Level Design

## 1) Runtime Modes

This project now has 3 runtime paths:

1. `holyf-network` (live TUI, existing mode)
2. `holyf-network daemon start/stop/status/prune` (snapshot collector daemon lifecycle + manual retention prune)
3. `holyf-network replay` (read-only history TUI)

```mermaid
flowchart TD
    A[main.go] --> B[cmd.Execute]
    B --> C[root command]
    C --> D[Live TUI: tui.NewApp]
    C --> E[Daemon: collect + write snapshots]
    C --> F[Replay TUI: tui.NewHistoryApp]
```

## 2) Live TUI (Existing Path)

`refreshData()` in `internal/tui/app_core.go` remains the central full-refresh loop.
Live scheduling now has 3 lanes:

- Main lane: every `--refresh/-r` seconds (plus manual `r`) runs `refreshData()` end-to-end.
- Interface fast lane: every `1s` re-renders the `System Health` panel for faster RX/TX visibility.
- Warm-up lane: one early full refresh at ~`1s` after startup to settle first-sample volatility.

Inside `refreshData()`:

1. Collect conntrack snapshot and rates.
2. Collect TCP retrans snapshot and rates.
3. Collect connection state counts.
4. Collect interface stats and rates.
5. Collect top talkers (`/proc/net/tcp*`, PID mapping from `/proc/<pid>/fd`).
6. Collect conntrack TCP flows (hybrid parse: extended + plain output), then merge host-facing NAT tuples when `/proc` misses sockets (Docker/NAT case).
7. Compute conntrack byte deltas and per-row throughput metrics (`TX/s`, `RX/s`).
8. Fallback collect socket counters from `ss` and overlay missing bandwidth.
9. Enrich top talkers with throughput metrics and internal total-delta fields for ranking/sort.
10. Build live Top Connections diagnosis from:
   - connection states
   - retrans health/sample gate
   - conntrack pressure/drops
   - top talker culprit extraction for dominant TCP-state patterns
11. Render panels and status bar.

`ct/nat` in live Top Connections means the row is conntrack/NAT-derived visibility (not direct host PID ownership).

Live Top Connections also has a few important presentation behaviors:

- The panel can toggle `Dir=IN` / `Dir=OUT` via `o`.
  - `IN` uses local listener ports as the heuristic for service-facing traffic.
  - `OUT` keeps non-listener flows and is visibility-only (`Enter`/`k` disabled).
- When result rows exceed visible panel height, `[` / `]` move across pages (works in both `CONN` and `GROUP` views, for both `IN` and `OUT`).
  - Footer includes page context (`Showing A-B of N ... | Page X/Y`).
- `View=GROUP` groups by `(peer, process)` and keeps the row compact (`PORTS` in `IN`, `RPORTS` in `OUT`, queues, bandwidth, process).
- Live `GROUP` view is capped to the top 20 groups by `CONNS`; footer shows `shown / total` when the cap is active.
- The selected row gets an inline footer preview (`Selected Detail`) with the full grouped state breakdown and, in `IN`, the effective `Enter`/`k` target.
- Live Top Connections hides TCP connections owned by the current `holyf-network` PID so internal update/control traffic does not appear as an operator-facing top row.
- The Diagnosis panel is host-global in v1; it is not scoped to the current filter/search slice.
- `d` opens an in-memory Diagnosis History modal that records diagnosis changes for the current live session.

Live TUI is the only mode that can run active mitigation (`k`, block/kill flow).

### 2.1) Active mitigation path (block vs kill flow)

Mitigation is implemented in `internal/tui/blocking/runtime.go` and `internal/actions/peer_blocker.go`, using `internal/kernelapi` interfaces.

Execution paths:

1. Timed block (`minutes > 0`, from `k/Enter` flow):
   - Step 1: insert firewall DROP rules (`BlockPeer`) via `kernelapi.Firewall` (nftables netlink on Linux 4.9+, `iptables`/`ip6tables` fallback).
   - Step 2: clear active connections with bounded converge sweep:
     - `KillPeerFlows` loop (default: max `4s`, max `12` iterations, sleep `120ms`)
     - each iteration:
       - broad `ss -K` pass
       - exact tuple kill pass (`KillSockets`) from real-time full socket query + current snapshot tuples
       - `conntrack -D` pass
       - re-count active sockets
     - active kill scope: all matching states except `TIME_WAIT`
     - `TIME_WAIT` is tracked for diagnostics only (not kill-failure criterion)
   - Step 3: start timer and auto-unblock at expiry (`UnblockPeer` removes DROP rules).

2. Kill-only (`minutes = 0`):
   - Do not insert firewall rules.
   - Run the same bounded connection-clearing sweep.
   - Accept that new matching connections can appear during the sweep window under storm/race.

Important clarification:

- Firewall rules are managed via `kernelapi.Firewall` (nftables netlink or iptables fallback).
- Active flow termination uses `kernelapi.SocketManager` (SOCK_DESTROY netlink or `ss -K` fallback) and `kernelapi.ConntrackManager` (netlink delete or `conntrack -D` fallback).
- Under conn storm/race windows, converge can return partial (`remaining N (storm/race)`) by design when bounded limits are hit.
- `minutes = 0` is pure kill-only semantics.

### 2.2) Metric sources, formulas, and equivalent shell commands

All per-second metrics in app use the same pattern:

`rate = (current_counter - previous_counter) / elapsed_seconds`

If `previous` is missing (first sample) or `elapsed_seconds <= 0`, the app shows baseline/first-reading semantics.

1. Conntrack table pressure + counters (`internal/collector/conntrack.go`)
   - Source:
     - `/proc/sys/net/netfilter/nf_conntrack_count`
     - `/proc/sys/net/netfilter/nf_conntrack_max`
     - `kernelapi.ConntrackManager.ReadStats()` (netlink on Linux 4.9+, `conntrack -S` fallback)
   - Formulas:
     - `usage_percent = current / max * 100`
     - `inserts_per_sec = (curr_insert - prev_insert) / elapsed`
     - `drops_per_sec = (curr_drop - prev_drop) / elapsed`
   - UI emphasis:
     - live panel focuses on `Used / Max`, `Conntrack%`, and non-zero `Drops`
     - `inserts_per_sec` is still collected but is not shown in the main panel anymore
   - Commands:
     - `cat /proc/sys/net/netfilter/nf_conntrack_count`
     - `cat /proc/sys/net/netfilter/nf_conntrack_max`
     - `conntrack -S`

2. TCP retransmission (`internal/collector/tcp_retransmits.go`)
   - Source: `/proc/net/snmp` (`Tcp:` row, fields `OutSegs`, `RetransSegs`)
   - Formulas:
     - `retrans_per_sec = (curr_retrans - prev_retrans) / elapsed`
     - `out_segs_per_sec = (curr_out - prev_out) / elapsed`
     - `retrans_percent = delta_retrans / delta_out * 100` (only when `delta_out > 0`)
   - Health gate (LOW SAMPLE in UI):
     - evaluate retrans health only when:
       - `ESTABLISHED >= retrans_sample.min_established`
       - `OutSegsPerSec >= retrans_sample.min_out_segs_per_sec`
   - Commands:
     - `awk '/^Tcp:/{if(!h){h=$0;next} v=$0; print h; print v; exit}' /proc/net/snmp`

3. Connection states (`internal/collector/connections.go`)
   - Source:
     - `/proc/net/tcp`
     - `/proc/net/tcp6`
   - Logic:
     - count `st` (hex state) across both files using fixed map (`01=ESTABLISHED`, `06=TIME_WAIT`, `0A=LISTEN`, ...)
   - Commands:
     - raw hex states: `cat /proc/net/tcp /proc/net/tcp6 | awk 'NR>1{print $4}' | sort | uniq -c`
     - readable cross-check: `ss -tanH | awk '{print $1}' | sort | uniq -c | sort -nr`

4. Interface throughput/packets (`internal/collector/interface_stats.go`)
   - Source: `/sys/class/net/<iface>/statistics/*`
     - `rx_bytes`, `tx_bytes`, `rx_packets`, `tx_packets`, `rx_errors`, `tx_errors`, `rx_dropped`, `tx_dropped`
   - Formulas:
     - `rx_bytes_per_sec = delta(rx_bytes) / elapsed`
     - `tx_bytes_per_sec = delta(tx_bytes) / elapsed`
     - `rx_pkts_per_sec = delta(rx_packets) / elapsed`
     - `tx_pkts_per_sec = delta(tx_packets) / elapsed`
     - errors/drops shown as cumulative counters (not per-sec)
  - Commands:
    - `cat /sys/class/net/<iface>/statistics/rx_bytes`
    - `cat /sys/class/net/<iface>/statistics/tx_bytes`
    - `ip -s link show dev <iface>`
  - Live alert overlay:
    - `Traffic` line is hidden while stable.
    - It appears only for interface spike warn/crit conditions.
    - Evaluation uses NIC speed when available, otherwise absolute throughput + spike ratio fallback.

5. Top Connections queue columns (`internal/collector/top_connections.go`)
   - Source:
     - `/proc/net/tcp`, `/proc/net/tcp6` field `tx_queue:rx_queue`
   - Formulas:
     - `Send-Q = tx_queue`
     - `Recv-Q = rx_queue`
     - internal activity score `Activity = tx_queue + rx_queue`
   - Extra enrichment:
     - PID/Proc mapping via `/proc/<pid>/fd/* -> socket:[inode]` and `/proc/<pid>/comm`
     - conntrack host-facing merge for NAT/Docker visibility (`internal/collector/conntrack_merge.go`)
       - synthetic process label: `ct/nat`
       - only injects `ESTABLISHED` tuples missing from `/proc` socket view
   - Commands:
     - `ss -tnap` (human-readable queue + process cross-check)

6. Per-connection bandwidth (`internal/collector/conntrack_flows.go`, `bandwidth_tracker.go`, `socket_counters.go`, `socket_bandwidth_tracker.go`)
   - Primary source:
     - `kernelapi.ConntrackManager.CollectFlowsTCP()` (netlink on Linux 4.9+)
     - Fallback: `conntrack -L -p tcp -o extended -n` + `conntrack -L -p tcp`
     - flows are de-duplicated by canonical tuple key
     - duplicate preference favors richer `bytes=` counters (then larger byte totals)
     - returns both directional byte counters per flow (orig/reply)
   - Primary formulas:
     - `tx_delta = clamp(curr_orig_bytes - prev_orig_bytes)`
     - `rx_delta = clamp(curr_reply_bytes - prev_reply_bytes)`
     - `tx_per_sec = tx_delta / elapsed`
     - `rx_per_sec = rx_delta / elapsed`
     - `clamp(x) = max(x, 0)` (handles counter reset/wrap)
   - Behavior:
     - first sample is baseline (no rates)
     - first-seen flow after baseline: delta = 0 (accumulated historical bytes are skipped; real delta appears on next sample)
     - sanity cap: per-flow delta capped at 12.5 GB/s (100 Gbps); anything above is treated as counter anomaly and zeroed
     - `clamp(x) = max(min(x, maxDeltaPerFlow), 0)`
   - Fallback source (only overlay rows still 0):
     - `kernelapi.SocketManager.CollectTCPCounters()` — always delegates to `ss -tinHn` exec because raw `tcp_info` byte counter offsets vary across kernel versions and reading them via netlink produces garbage
   - Kernel API note:
     - Conntrack flow dump uses netlink (no `conntrack` CLI needed)
     - Socket counter collection (`bytes_acked`/`bytes_received`) uses `ss -tinHn` for reliability
     - `cat /proc/sys/net/netfilter/nf_conntrack_acct` should be `1` for byte accounting
     - When `nf_conntrack_acct=0`, netlink conntrack returns zero byte counters

## 3) Daemon Snapshot Pipeline

Package: `internal/history` + `cmd/daemon.go`

1. `daemon start` launches internal worker in background and writes PID/log/runtime state paths.
2. Optional daemon defaults file: `/etc/holyf-network/daemon.json`
   - file is optional; missing file means built-in defaults
   - partial overrides are allowed (`data-dir`, `interface`, `interval`, `top-limit`, `retention-hours`)
   - precedence:
     - `daemon start/run`: CLI flags -> config file -> built-in defaults
     - `replay`: explicit `--data-dir/--file` -> active daemon state -> config file -> built-in defaults
     - `daemon status/stop/prune`: explicit target flags -> active daemon state -> config file -> built-in defaults
3. Worker resolves interface (`--interface`) and starts `SnapshotWriter` with lock file (`.daemon.lock`) under `--data-dir`.
4. Every `--interval` seconds:
   - call `collector.CollectTopTalkers(0)` to sample current connections
   - call `collector.CollectListenPorts()` to classify connections into `IN` vs `OUT` using listener-backed local ports
   - call `collector.CollectConntrackFlowsTCP()` and merge host-facing conntrack NAT tuples into live sample
   - synthetic NAT tuples are persisted as `proc_name=ct/nat` when host PID ownership is unavailable
   - compute byte deltas from previous sample
   - enrich connections with bandwidth fields
   - drop self-traffic from the current `holyf-network` PID before snapshot aggregation
   - aggregate per direction with sums:
     - `IN`: `peer_ip + local service port + proc_name`
     - `OUT`: `peer_ip + remote service port + proc_name`
     - `conn_count = count(rows)`
     - `tx_queue = sum(TxQueue)`, `rx_queue = sum(RxQueue)`, `total_queue = sum(Activity)`
     - `tx_bytes_delta = sum(TxBytesDelta)`, `rx_bytes_delta = sum(RxBytesDelta)`, `total_bytes_delta = sum(TotalBytesDelta)`
     - `tx_bytes_per_sec = sum(TxBytesPerSec)`, `rx_bytes_per_sec = sum(RxBytesPerSec)`
   - sort + cap (`--top-limit`) independently per side by:
     - `total_bytes_delta DESC`
     - `conn_count DESC`
     - `total_queue DESC`
     - then deterministic tie-break: `peer_ip`, `port`, `proc_name`
   - collect daemon process CPU/memory via `collector.CollectSystemUsage()` (getrusage + /proc/self/statm)
   - write one aggregate `SnapshotRecord` as JSON Lines record (includes `cpu_cores` and `rss_bytes`)
5. Segment file naming by server local day: `connections-YYYYMMDD.jsonl`.
6. Retention:
   - remove segments older than `--retention-hours`
   - daemon runtime prune schedule:
     - once at startup
     - daily at local `00:00`
   - manual prune command:
     - `holyf-network daemon prune`
7. Active daemon state file (`daemon.state`) is the default source of truth for `status/stop/prune` without explicit targeting flags.
   - includes runtime metadata such as `retention_hours` for prune default resolution
8. `daemon prune` without explicit target flags uses active-state target resolution; if daemon is not running, it falls back to config file defaults and then built-ins.
9. `daemon stop` sends `SIGTERM` (fallback `SIGKILL`) and removes PID file + active state.
10. `daemon status` reports running/stopped from active-state or explicit flags.
11. Default Linux root paths:
   - snapshots: `/var/lib/holyf-network/snapshots`
   - daemon log: `/var/log/holyf-network/daemon.log`
   - active-state: `/run/holyf-network/daemon.state`
12. Worker handles `SIGINT/SIGTERM` and closes cleanly.
13. Interval guidance:
   - bandwidth-focused monitoring: `5-10s`
   - connection trend monitoring: `30s` default
   - large intervals can miss short-lived flows in snapshots/replay

File format note:

- Snapshot segments follow JSON Lines format: https://jsonlines.org/
- `.jsonl` file contract:
  - UTF-8 text
  - one complete JSON object per line
  - line order is chronological append order
  - each line is one `SnapshotRecord`
- Full on-disk format reference: `docs/SNAPSHOT_FORMAT.md`

## 4) Snapshot Storage Model

Single format policy:

- aggregate snapshot format only
- no compatibility reader for older raw-connection snapshot schemas

### Record model (`internal/history/types.go`)

- `SnapshotRecord`
  - `CapturedAt`
  - `Interface`
  - `TopLimitPerSide` (max aggregate rows for `IN` and `OUT` independently)
  - `SampleSeconds`
  - `BandwidthAvailable`
  - `IncomingGroups []SnapshotGroup`
  - `OutgoingGroups []SnapshotGroup`
  - `Version`

- `SnapshotGroup`
  - queue snapshot fields: `TxQueue`, `RxQueue`, `TotalQueue`
  - bandwidth fields: `TxBytesDelta`, `RxBytesDelta`, `TotalBytesDelta`, `TxBytesPerSec`, `RxBytesPerSec`, `TotalBytesPerSec`

- `SnapshotRef`
  - `FilePath`
  - `Offset`
  - `CapturedAt`
  - `IncomingCount`
  - `OutgoingCount`
  - `TotalCount`

### On-disk example (`.jsonl` one line)

```json
{"captured_at":"2026-03-08T12:56:30.196962352+07:00","interface":"eth0","top_limit_per_side":500,"sample_seconds":29.999999695,"bandwidth_available":true,"incoming_groups":[{"peer_ip":"172.25.110.116","port":22,"proc_name":"sshd","conn_count":2,"tx_queue":0,"rx_queue":0,"total_queue":0,"tx_bytes_delta":377892,"rx_bytes_delta":41164,"total_bytes_delta":419056,"tx_bytes_per_sec":12596.400128063402,"rx_bytes_per_sec":1372.1333472833558,"total_bytes_per_sec":13968.533475346758,"states":{"ESTABLISHED":2}}],"outgoing_groups":[{"peer_ip":"20.205.243.168","port":443,"proc_name":"curl","conn_count":1,"tx_queue":0,"rx_queue":0,"total_queue":0,"tx_bytes_delta":0,"rx_bytes_delta":0,"total_bytes_delta":0,"tx_bytes_per_sec":0,"rx_bytes_per_sec":0,"total_bytes_per_sec":0,"states":{"ESTABLISHED":1}}],"version":"v0.3.46"}
```

### Reader model (`internal/history/reader.go`)

- `LoadIndex(dataDir)`:
  - scan segment files oldest->latest
  - parse each line to produce refs
  - skip malformed JSON lines and count `Corrupt`
- `ReadSnapshot(ref)`:
  - seek by byte offset
  - decode one line into `SnapshotRecord`

## 5) Replay TUI (Read-only)

Package: `internal/tui/history_app.go` + `internal/tui/replay/` + `cmd/replay.go`

State includes:

- snapshot refs + current index
- current snapshot record
- optional single-file scope (`replay --file <segment>`)
- optional inclusive time window scope (`replay -b/--begin`, `-e/--end`)
- filter/search/sort/mask/selection
- follow-latest toggle (`L`)

Navigation keys:

- `[` previous snapshot
- `]` next snapshot
- `a` oldest
- `e` latest
- `t` jump to specific timestamp

Behavior constraints:

- replay index is filtered by `file scope ∩ time window` before UI state/navigation
- when no `--file/-f` and no `--begin/-b`/`--end/-e`, replay defaults to current local day window
- `-b/-e` parse with replay jump-time semantics; clock-only inputs use:
  - selected segment date when `--file` is provided
  - current local date when `--file` is not provided
- when only one bound is provided:
  - `-b` only => end bound auto-completes to end-of-day
  - `-e` only => begin bound auto-completes to start-of-day
- replay renders aggregate rows only
- replay toggles aggregate `IN/OUT` via `o`
- replay keeps all stored rows for the selected direction; it does not inherit the live top-20 group cap
- kill/block hotkeys (`Enter`, `k`, `b`) are explicitly blocked with status note
- `/` search/filter applies only to current snapshot
- `Shift+S` timeline search scans all loaded snapshots in current replay scope
- replay renders queue + bandwidth columns from aggregate rows
- replay rows keep `proc_name` exactly as persisted (including `ct/nat` synthetic NAT label)
- replay `skip-empty`, active timeline position, and idle streak are all direction-aware
- replay data source resolution:
  - explicit hidden `--data-dir` override
  - else active daemon state `data_dir`
  - else runtime default snapshot dir

## 6) UI Composition

### Live mode — two views

**View switching:**
- `Ctrl+1`: Dashboard view (default)
- `Ctrl+2`: Bandwidth chart view (full-screen dual time-series charts)

**Dashboard view (Ctrl+1)** (`layout/live.go`):
- Left: `Top Connections` (spans full height)
- Right top: `System Health` (merged: connection states + interface stats + conntrack in one panel with dim section separators)
- Right bottom: `Diagnosis` — operator card: `Issue`, `Scope`, `Signal`, `Likely Cause`, `Confidence`, `Why`, `Next Actions`
- Bottom: status bar
- 3 panels total
- `GROUP` view groups by `(peer, process)` for clarity under mixed ownership (`sshd` + `ct/nat`, etc.)
- Top Connections can render a live bandwidth note above the table when needed.
- Top Connections can also render a footer preview for the selected row when panel height allows.
- Status bar indicators:
  - `API:kernel` (green) or `API:<backend details>` (yellow) — shows kernel API vs CLI fallback status
  - `LINK:<speed>Mb/s` — only shown when NIC speed is known (hidden otherwise)
- Navigation: Tab cycles 3 panels

**Bandwidth chart view (Ctrl+2)** (`panels/chart.go`):
- Two side-by-side time-series charts: `Incoming (RX)` and `Outgoing (TX)`
- Rendered with Braille Unicode characters (U+2800-U+28FF) for high-resolution line graphs
- Connected lines between data points using Bresenham's line algorithm on Braille grid
- Y-axis: auto-scaled bandwidth labels (B, KB, MB, GB)
- X-axis: time labels (-60s → now)
- Data source: ring buffer of last 60 interface rate samples (1 sample/second)
- In chart view, most hotkeys are disabled — only `q`, `?`, and `Ctrl+1` work
- Ring buffer: `internal/tui/shared/ring.go` (fixed 60-sample circular buffer)

### Replay mode (`layout/replay.go`)

- Single panel: `Connection History`
- Bottom: replay status bar
- Overlay help/filter/search pages only
- Status bar shows daemon process metrics when available in snapshot:
  - `CPU:<cores>c RSS:<size>MB` — daemon CPU and memory at capture time
  - Omitted for old snapshots that don't include these fields

## 7) Persistence

1. Action history (live mode)
   - `~/.holyf-network/history.log`
   - rolling 500 events
   - `h` shows latest 20

2. Connection snapshots (daemon/replay)
   - root default: `/var/lib/holyf-network/snapshots`
   - non-root/dev default: `~/.holyf-network/snapshots`
   - daily JSON Lines segment files (`connections-YYYYMMDD.jsonl`)
   - retention via age (`--retention-hours`)

## 8) Concurrency Model

- TUI updates always go through `tview.Application.QueueUpdateDraw`.
- Live mode and replay mode each own their own app state struct.
- `SnapshotWriter` serializes appends with mutex and lock file for single-writer safety.

## 9) External Dependencies and OS Assumptions

- Linux runtime (`/proc`, `/sys`, netfilter tooling).
- Collector path relies on kernel network procfs/sysfs files.
- Bandwidth/NAT enrichment uses `kernelapi.ConntrackManager` (netlink on Linux 4.9+, conntrack CLI fallback).
- `ct/nat` indicates conntrack-derived NAT visibility, not direct host process PID ownership.
- Kernel API layer (`internal/kernelapi/`):
  - On Linux 4.9+ with `CAP_NET_ADMIN`: uses direct netlink sockets (no CLI tools needed)
  - Fallback: `iptables`/`ip6tables`, `ss`, `conntrack` CLI tools
  - `tcpdump` is the only remaining external tool dependency (for packet capture feature)
  - See `docs/ai-context/KERNEL_API.md` for full architecture details
- `sudo` recommended for full live-mode visibility/mitigation (required for netlink access).

## 10) Extension Guidelines

1. Put read-only scraping into `internal/collector`.
2. Put side effects into `internal/actions` or `internal/history` (for snapshot persistence).
3. Keep renderer files (`panels/`) side-effect free.
4. Keep interaction flow split by mode (`app_*` for live, `history_app.go` + `replay/` for replay).
5. Add tests for parsing/indexing/retention and key handling regressions.
