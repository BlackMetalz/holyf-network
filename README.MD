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
curl -sL https://github.com/BlackMetalz/holyf-network/releases/latest/download/holyf-network-linux-amd64 -o /tmp/holyf-network
chmod +x /tmp/holyf-network
sudo mv /tmp/holyf-network /usr/local/bin/holyf-network
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
# Up / Down: select row in Top Connections
# Enter / k: block selected Top Connections row
# Shift+B: sort by Bandwidth (press again toggles DESC/ASC)
# Shift+C: sort by Conns (press again toggles DESC/ASC)
# Shift+P: sort by Port (press again toggles DESC/ASC)
# i: explain Send-Q / Recv-Q / TX/s / RX/s
# Shift+I: explain Interface Stats (RX/TX, Packets, Errors, Drops)
# g: toggle grouped view (peer + process)
# /: search Top Connections by text (contains match)
# f: port filter (press f again to clear all filters)
# s: toggle sensitive IP masking
# b: list active blocks and remove selected peer block
# h: show action log (latest 20)
# p: pause/resume auto-refresh
# r: refresh now
# z: zoom/unzoom focused panel
# ?: show help
# q: quit
```

### Top Connections process labels (important)

- `PID/NAME` (example `44011/sshd`): socket is visible in host `/proc` and mapped to a host process.
- `ct/nat`: synthetic connection row inferred from conntrack host-facing NAT tuple.
  - This is used for Docker/NAT cases where host socket/PID is not directly visible in `/proc/net/tcp*`.
  - It means traffic is real and seen by conntrack, but process ownership is NAT-derived.
- `-`: process info not available.

Group view behavior:
- Live `View=GROUP` now groups by `(peer, process)` instead of peer-only.
- Example: same peer with `sshd=1` and `ct/nat=4999` will render as 2 separate rows for clarity.
- Footer shows `groups, peers, total connections`.

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

# Stop daemon
holyf-network daemon stop
```

Daemon notes:
- Storage format: daily NDJSON segments by server local time (`connections-YYYYMMDD.jsonl`).
- Snapshot payload stores aggregate rows by `peer_ip + local_port + proc_name` (no raw connection list).
- For Docker/NAT traffic, daemon snapshots can persist `proc_name=ct/nat` rows (same semantics as live view).
- `--top-limit` is the max aggregate rows stored per snapshot.
- Retention policy: remove old segments beyond `--retention-hours`.
- A lock file prevents multiple daemons writing the same `--data-dir`.
- Default paths on Linux root:
  - snapshots: `/var/lib/holyf-network/snapshots`
  - daemon log: `/var/log/holyf-network/daemon.log`
  - active state: `/run/holyf-network/daemon.state`
- `daemon status/stop` without explicit `--data-dir/--pid-file` uses active-state targeting.
- Explicit flags (`--data-dir` or `--pid-file`) force `status/stop` to target that explicit location.
- For bandwidth monitoring, use shorter interval (`--interval 5..10`) to capture bursts better.
- For connection trend monitoring, keep default interval (`30s`) to reduce noise/storage.

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
- `Up` / `Down`: select row
- `f`: port filter (press again to clear filters)
- `/`: grep-like contains filter for current snapshot
- `Shift+B/C/P`: sort mode (press same key to toggle DESC/ASC)
- `i`: explain Send-Q / Recv-Q / TX/s / RX/s
- `Shift+I`: alias of `i` (explain queue/bandwidth columns)
- `s`: mask IP display
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
Replay view is aggregate-only: rows are grouped by `peer_ip + local_port + proc_name`.
History reader accepts only this aggregate snapshot format.
`proc_name=ct/nat` in replay means the row came from conntrack/NAT visibility path (not direct host PID ownership).
Bandwidth columns:
- `TX/s`, `RX/s`: conntrack byte-delta throughput rate for current sample interval.
- `Send-Q/Recv-Q`: kernel queue backlog snapshot (not throughput counters).

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
