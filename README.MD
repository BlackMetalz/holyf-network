# Network Visibility tool
For learning Go purpose and a tool that I can use daily for network monitor when a server has alert related to network issues!
I'm suck at Linux Network!

# Vibe Code
Yes, fucking yes.

# AI Onboarding Docs
- See `docs/ai-context/`:
  - `README.md`
  - `PROJECT_STRUCTURE.md`
  - `HIGH_LEVEL_DESIGN.md`

# Requirements

Runtime requirements:

```bash
# Required commands (Ubuntu/Debian)
sudo apt update
sudo apt install -y conntrack iproute2 iptables
```

```bash
# Required commands (CentOS/RHEL)
sudo yum install -y conntrack-tools iproute iptables
```

Kernel / distro notes:
- Linux only (this app reads `/proc` network data and uses netfilter tooling).
- Conntrack panel needs `nf_conntrack` support (`/proc/sys/net/netfilter/nf_conntrack_count` and `nf_conntrack_max`).
- Practical baseline: kernel `3.10+` (CentOS 7+) and newer.
- Release binary is built static to run across common Linux distros (including Ubuntu 20.04+ and CentOS 7+).

Privilege notes:
- Run with `sudo` (or root) for full functionality.
- `sudo` is required for: block/unblock peer, kill active flows, accurate conntrack stats on hardened hosts, and full process mapping from `/proc/[pid]/fd`.

# Install
### Ubuntu/Linux (amd64)
```bash
curl -sL https://github.com/BlackMetalz/holyf-network/releases/latest/download/holyf-network-linux-amd64 -o /tmp/holyf-network.bin
chmod +x /tmp/holyf-network.bin
sudo mv /tmp/holyf-network.bin /usr/local/bin/holyf-network
sudo holyf-network -v
```

# Usage
```bash
# Default refresh rate is 30 seconds
sudo holyf-network

# Set refresh rate to 5 seconds
sudo holyf-network -r 5

# Keyboard shortcuts inside TUI
# Tab / Shift+Tab: move focus between panels
# Ctrl+1..5: focus panel (1=Top, 2=States, 3=Interface, 4=Conntrack, 5=Diagnosis)
# Up / Down: select row in Top Connections
# [ / ]: previous / next page in Top Connections when rows exceed visible height
# o: toggle Top Connections IN/OUT mode
# Enter / k: block selected Top Connections row (IN mode only)
# T: trace packet for selected Top Connections row (bounded tcpdump flow)
# t: open Trace History (latest trace runs, Enter=view detail)
# Shift+B: sort by Bandwidth (press again toggles DESC/ASC)
# Shift+C: sort by Conns (press again toggles DESC/ASC)
# Shift+P: sort by Port (press again toggles DESC/ASC)
# i: explain Send-Q / Recv-Q / TX/s / RX/s
# Shift+I: explain Interface Stats (RX/TX, Packets, Errors, Drops)
# g: toggle grouped view (peer + process, capped to top 20 groups by CONNS)
# /: search Top Connections by text (contains match)
# f: port filter (local port in IN mode, remote port in OUT mode; press f again to clear all filters)
# m: toggle sensitive IP masking
# s: sort Connection States by count (toggle DESC/ASC)
# b: list active blocks and remove selected peer block
# d: show Diagnosis History (latest 20 changes in current live session)
# h: show action log (latest 20)
# p: pause/resume auto-refresh
# r: refresh now
# z: zoom/unzoom Top Connections only
# ?: show help
# q: quit
```

Live refresh model:
- `-r/--refresh` controls the main full refresh loop (Top/States/Conntrack/Diagnosis and full data recompute).
- `3. Interface Stats` has a dedicated `1s` refresh lane in live mode for faster RX/TX visibility.
- `p` (pause) pauses both lanes.

Mitigation behavior:
- `minutes > 0`: block first, then kill active connections, then auto-unblock when timer expires.
- `minutes = 0`: kill active connections only (no block rule, no timer).
- Kill success ignores `TIME_WAIT`; partial results show as `remaining N (storm/race)`.

## Metrics Guides

- Beginner TCP/network foundation (English, operator-first): `docs/NETWORK_FOUNDATIONS_FOR_SRE_EN.md`
- Beginner TCP/network foundation (Vietnamese, operator-first): `docs/NETWORK_FOUNDATIONS_FOR_SRE_VI.md`
- Tcpdump for beginners (Vietnamese, practical packet-capture basics): `docs/TCPDUMP_FOR_BEGINNERS_VI.md`
- Tcpdump trace feature in holyf-network (Vietnamese, UX/guardrails): `docs/TCPDUMP_TRACE_FEATURE_VI.md`
- Incident mental checklist (English, 1-page quick reference): `docs/INCIDENT_MENTAL_CHECKLIST_EN.md`
- Incident mental checklist (Vietnamese, 1-page quick reference): `docs/INCIDENT_MENTAL_CHECKLIST_VI.md`
- Vietnamese (practical ops): `docs/USER_METRICS_GUIDE_VI.md`
- English (practical ops): `docs/USER_METRICS_GUIDE_EN.md`
- Daemon snapshot file spec: `docs/SNAPSHOT_FORMAT.md`

### Top Connections process labels (short)

- `PID/NAME` (example `44011/sshd`): socket mapped to a host process.
- `ct/nat`: synthetic row from conntrack/NAT host-facing visibility path.
- `-`: process info not available.

## Daemon Snapshot Mode

```bash
# Start daemon in background (default interval = 30s)
holyf-network daemon start

# Typical production start with explicit controls
holyf-network daemon start \
  --interface eth0 \
  --interval 30 \
  --top-limit 500 \
  --data-dir ~/.holyf-network/snapshots \
  --retention-hours 168

# Check status
holyf-network daemon status

# Prune old segment files immediately (on-demand)
holyf-network daemon prune

# Stop daemon
holyf-network daemon stop
```

Daemon notes:
- Storage format: daily **JSON Lines** (`.jsonl`, one JSON object per line) by server local time (`connections-YYYYMMDD.jsonl`).
  - Spec: https://jsonlines.org/
  - Full format contract: `docs/SNAPSHOT_FORMAT.md`
- Snapshot payload stores aggregate-only rows in two arrays:
  - `incoming_groups`: grouped by `peer_ip + local service port + proc_name`
  - `outgoing_groups`: grouped by `peer_ip + remote service port + proc_name`
- No raw connection list is persisted in history.
- For Docker/NAT traffic, daemon snapshots can persist `proc_name=ct/nat` rows (same semantics as live view).
- `--top-limit` is the max aggregate rows stored **per side** per snapshot (`IN` cap + `OUT` cap).
- Retention policy: remove old segments beyond `--retention-hours`.
  - In daemon runtime, prune runs once at startup and then daily at local `00:00`.
  - Use `holyf-network daemon prune` for immediate manual prune.
- A lock file prevents multiple daemons writing the same `--data-dir`.
- Default paths on Linux root:
  - snapshots: `/var/lib/holyf-network/snapshots`
  - daemon log: `/var/log/holyf-network/daemon.log`
  - active state: `/run/holyf-network/daemon.state`
- Optional daemon defaults file: `/etc/holyf-network/daemon.json`
  - file is optional; if absent, built-in defaults are used
  - partial config is allowed, for example only overriding `data-dir`
  - precedence:
    - `daemon start/run`: CLI flags -> `daemon.json` -> built-in defaults
    - `replay`: explicit `--data-dir/--file` -> active daemon state -> `daemon.json` -> built-in defaults
    - `daemon status/stop/prune`: explicit target flags -> active daemon state -> `daemon.json` -> built-in defaults
  - example:

```json
{
  "data-dir": "/var/lib/holyf-network/snapshots"
}
```

- `daemon status/stop` without explicit `--data-dir/--pid-file` uses active-state targeting.
- `daemon prune` without explicit `--data-dir/--pid-file` also uses active-state targeting.
- `daemon prune` retention source precedence:
  - `--retention-hours` flag
  - active-state `retention_hours` (if present)
  - `daemon.json` `retention-hours` (if present)
  - default `168h`
- Explicit flags (`--data-dir` or `--pid-file`) force `status/stop/prune` to target that explicit location.
- For bandwidth monitoring, use shorter interval (`--interval 5..10`) to capture bursts better.
- For connection trend monitoring, keep default interval (`30s`) to reduce noise/storage.

Snapshot file quick inspect:

```bash
# count snapshots in one day file
wc -l /var/lib/holyf-network/snapshots/connections-YYYYMMDD.jsonl

# read latest 3 snapshots
tail -n 3 /var/lib/holyf-network/snapshots/connections-YYYYMMDD.jsonl
```

One line = one snapshot record (JSON object), for example:

```json
{"captured_at":"2026-03-08T12:56:30.196962352+07:00","interface":"eth0","top_limit_per_side":500,"sample_seconds":29.999999695,"bandwidth_available":true,"incoming_groups":[{"peer_ip":"172.25.110.116","port":22,"proc_name":"sshd","conn_count":2,"tx_queue":0,"rx_queue":0,"total_queue":0,"tx_bytes_delta":377892,"rx_bytes_delta":41164,"total_bytes_delta":419056,"tx_bytes_per_sec":12596.400128063402,"rx_bytes_per_sec":1372.1333472833558,"total_bytes_per_sec":13968.533475346758,"states":{"ESTABLISHED":2}}],"outgoing_groups":[{"peer_ip":"20.205.243.168","port":443,"proc_name":"curl","conn_count":1,"tx_queue":0,"rx_queue":0,"total_queue":0,"tx_bytes_delta":0,"rx_bytes_delta":0,"total_bytes_delta":0,"tx_bytes_per_sec":0,"rx_bytes_per_sec":0,"total_bytes_per_sec":0,"states":{"ESTABLISHED":1}}],"version":"v0.3.46"}
```

## Replay (History) Mode

```bash
# Open replay UI for current day (server local time)
holyf-network replay

# Open replay UI with masked IP prefixes
holyf-network replay --sensitive-ip

# Open one specific daily snapshot segment file
holyf-network replay --file connections-20260304.jsonl
# shorthand:
holyf-network replay -f connections-20260304.jsonl

# Replay only snapshots inside a time window (inclusive)
holyf-network replay -b 20:00 -e 23:59

# With --file, clock-only time binds to that file's date
holyf-network replay --file connections-20260304.jsonl -b 20:00 -e 23:59
# shorthand:
holyf-network replay -f connections-20260304.jsonl -b 20:00 -e 23:59
```

Replay path resolution:
- Default (`holyf-network replay`): uses active daemon `data_dir` from state file.
- If no active daemon state is found, falls back to system default snapshot path.
- `--data-dir` still exists as an advanced override (hidden from help output).

Replay hotkeys:
- left/right bracket (`[` / `]`): previous/next snapshot
- `a` / `e`: oldest/latest snapshot
- `t`: jump to specific timestamp
- `L`: follow latest snapshots (tail-like)
- `o`: toggle replay `IN/OUT`
- `Up` / `Down`: select row
- `f`: port filter (press again to clear filters)
- `/`: grep-like contains filter for current snapshot
- `Shift+B/C/P`: sort mode (press same key to toggle DESC/ASC)
- `i`: explain Send-Q / Recv-Q / TX/s / RX/s
- `Shift+I`: alias of `i` (explain queue/bandwidth columns)
- `m`: mask IP display
- `x`: toggle skip-empty snapshot navigation
- `?`: help
- `q`: quit

`t` accepts: `YYYY-MM-DD HH:MM[:SS]`, `HH:MM[:SS]` (today), `yesterday HH:MM`, or RFC3339.
`--begin/-b` and `--end/-e` use the same time parsing semantics as `t`.
When `--file` is set and `-b/-e` are clock-only (`HH:MM[:SS]`), replay uses that segment's date.
If only `-b` is provided, replay auto-sets `-e` to end-of-day of `-b`.
If only `-e` is provided, replay auto-sets `-b` to start-of-day of `-e`.
Time-window filtering is inclusive (`captured_at >= begin` and `captured_at <= end`).

Replay mode is read-only: no block/kill actions are executed.
Replay view is aggregate-only:
- `IN`: rows are grouped by `peer_ip + local service port + proc_name`
- `OUT`: rows are grouped by `peer_ip + remote service port + proc_name`
- replay does not inherit the live top-20 group cap; it shows all stored rows in the snapshot, limited only by panel height
History reader accepts only this aggregate snapshot format.
For metric semantics (`Send-Q`, `Recv-Q`, `TX/s`, `RX/s`, `Conntrack Used/Max`, `ct/nat`) and troubleshooting,
see `docs/USER_METRICS_GUIDE_EN.md` (or Vietnamese version: `docs/USER_METRICS_GUIDE_VI.md`).

Default threshold file:

```toml
# config/health_thresholds.toml
[retrans_percent]
warn = 2.0
crit = 5.0

[drops_per_sec]
warn = 10
crit = 50

[conntrack_percent]
warn = 70
crit = 85

[bandwidth_per_sec]
warn = 52428800
crit = 157286400

[bandwidth_per_snapshot]
warn = 524288000
crit = 2147483648

[retrans_sample]
# Only evaluate retrans health if BOTH conditions are met.
min_established = 20
min_out_segs_per_sec = 60
```

In case you want to custom threshold:
```bash
# Use custom health strip thresholds
holyf-network --health-config /etc/holyf-network/health_thresholds.toml
```

Notes:
- If retrans sample is below thresholds, UI shows `LOW SAMPLE` and does not trigger retrans WARN/CRIT.
- `bandwidth_per_sec` controls BW column warn/crit coloring (`TX/s`, `RX/s`).
- `bandwidth_per_snapshot` remains used for internal total-delta evaluation/sorting.
