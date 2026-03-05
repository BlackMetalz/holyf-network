# High Level Design

## 1) Runtime Modes

This project now has 3 runtime paths:

1. `holyf-network` (live TUI, existing mode)
2. `holyf-network daemon start/stop/status` (snapshot collector daemon lifecycle)
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

`refreshData()` in `internal/tui/app_core.go` remains the central loop:

1. Collect conntrack snapshot and rates.
2. Collect TCP retrans snapshot and rates.
3. Collect connection state counts.
4. Collect interface stats and rates.
5. Collect top talkers (`/proc/net/tcp*`, PID mapping from `/proc/<pid>/fd`).
6. Collect conntrack TCP flow byte counters and compute interval deltas.
7. Enrich top talkers with throughput metrics (`TX/s`, `RX/s`) and internal total-delta fields for ranking.
8. Render panels and status bar.

Live TUI is the only mode that can run active mitigation (`k`, block/kill flow).

## 3) Daemon Snapshot Pipeline

Package: `internal/history` + `cmd/daemon.go`

1. `daemon start` launches internal worker in background and writes PID/log paths.
2. Worker resolves interface (`--interface`) and starts `SnapshotWriter` with lock file (`.daemon.lock`) under `--data-dir`.
3. Every `--interval` seconds:
   - call `collector.CollectTopTalkers(0)` to sample current connections
   - call `collector.CollectConntrackFlowsTCP()` and compute byte deltas from previous sample
   - enrich connections with bandwidth fields
   - aggregate by `peer_ip + local_port + proc_name`
   - write one aggregate `SnapshotRecord` as NDJSON line
4. Segment file naming by server local day: `connections-YYYYMMDD.jsonl`.
5. Retention:
   - remove segments older than `--retention-hours`
6. `daemon stop` sends `SIGTERM` (fallback `SIGKILL`) and removes PID file.
7. `daemon status` reads PID file and reports running/stopped state.
8. Worker handles `SIGINT/SIGTERM` and closes cleanly.
9. Interval guidance:
   - bandwidth-focused monitoring: `5-10s`
   - connection trend monitoring: `30s` default

## 4) Snapshot Storage Model

Single format policy:

- aggregate snapshot format only
- no compatibility reader for older raw-connection snapshot schemas

### Record model (`internal/history/types.go`)

- `SnapshotRecord`
  - `CapturedAt`
  - `Interface`
  - `TopLimit` (max aggregate rows)
  - `SampleSeconds`
  - `BandwidthAvailable`
  - `Groups []SnapshotGroup`
  - `Version`

- `SnapshotGroup`
  - queue snapshot fields: `TxQueue`, `RxQueue`, `TotalQueue`
  - bandwidth fields: `TxBytesDelta`, `RxBytesDelta`, `TotalBytesDelta`, `TxBytesPerSec`, `RxBytesPerSec`, `TotalBytesPerSec`

- `SnapshotRef`
  - `FilePath`
  - `Offset`
  - `CapturedAt`
  - `ConnCount`

### Reader model (`internal/history/reader.go`)

- `LoadIndex(dataDir)`:
  - scan segment files oldest->latest
  - parse each line to produce refs
  - skip malformed JSON lines and count `Corrupt`
- `ReadSnapshot(ref)`:
  - seek by byte offset
  - decode one line into `SnapshotRecord`

## 5) Replay TUI (Read-only)

Package: `internal/tui/history_*.go` + `cmd/replay.go`

State includes:

- snapshot refs + current index
- current snapshot record
- optional single-file scope (`replay --file <segment>`)
- filter/search/sort/mask/selection
- follow-latest toggle (`L`)

Navigation keys:

- `[` previous snapshot
- `]` next snapshot
- `a` oldest
- `e` latest
- `t` jump to specific timestamp

Behavior constraints:

- replay renders aggregate rows only
- kill/block hotkeys (`Enter`, `k`, `b`) are explicitly blocked with status note
- search/filter apply only to current snapshot
- replay renders queue + bandwidth columns from aggregate rows

## 6) UI Composition

### Live mode (`layout.go`)

- Left: `Top Connections`
- Right stack: `Connection States`, `Interface Stats`, `Conntrack`
- Bottom: status bar

### Replay mode (`history_layout.go`)

- Single panel: `Connection History`
- Bottom: replay status bar
- Overlay help/filter/search pages only

## 7) Persistence

1. Action history (live mode)
   - `~/.holyf-network/history.log`
   - rolling 500 events
   - `h` shows latest 20

2. Connection snapshots (daemon/replay)
   - `~/.holyf-network/snapshots` by default
   - daily NDJSON segment files (`connections-YYYYMMDD.jsonl`)
   - retention via age (`--retention-hours`)

## 8) Concurrency Model

- TUI updates always go through `tview.Application.QueueUpdateDraw`.
- Live mode and replay mode each own their own app state struct.
- `SnapshotWriter` serializes appends with mutex and lock file for single-writer safety.

## 9) External Dependencies and OS Assumptions

- Linux runtime (`/proc`, `/sys`, netfilter tooling).
- Collector path relies on kernel network procfs/sysfs files.
- Bandwidth enrichment relies on `conntrack -L -p tcp -o extended` counters (TCP flows).
- Mitigation path relies on `iptables`/`ip6tables`, `conntrack`, `ss`.
- `sudo` recommended for full live-mode visibility/mitigation.

## 10) Extension Guidelines

1. Put read-only scraping into `internal/collector`.
2. Put side effects into `internal/actions` or `internal/history` (for snapshot persistence).
3. Keep renderer files (`panel_*.go`) side-effect free.
4. Keep interaction flow split by mode (`app_*` for live, `history_*` for replay).
5. Add tests for parsing/indexing/retention and key handling regressions.
