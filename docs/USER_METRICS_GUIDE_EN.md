# User Metrics Guide (EN)

This guide explains how to read `holyf-network` metrics in practical operations.

If you are still new to TCP states / queues / conntrack, read this first:

- `docs/NETWORK_FOUNDATIONS_FOR_SRE_EN.md`

## 30-second incident scan

1. Check `Connection States`:
   - Is `HEALTH` yellow/red?
   - Are `Retrans`, `Drops`, `Conntrack%` rising?
2. Check `Interface Stats`:
   - Is `RX/TX` spiking?
   - Are `Errors/Drops` non-zero?
3. Open `Top Connections`:
   - Sort by bandwidth (`Shift+B`) to find heavy flows first.
   - Look for unusual `Send-Q/Recv-Q`.
4. If Docker/NAT is involved:
   - `ct/nat` rows are expected (real traffic, NAT/conntrack-derived ownership).
5. For forensic analysis:
   - Use `replay` timeline mode (read-only, no kill/block).

## Panel-by-panel meaning

## 1) Top Connections

Live panel 1 can switch between:

- `Dir=IN` (`Top Incoming`): listener-backed flows coming into local services.
- `Dir=OUT` (`Top Outgoing`): local processes dialing out to remote services.
  - Toggle with `o`.
  - `Enter` / `k` stay enabled only in `IN`.

Core columns:

- `PROCESS`: socket ownership.
  - `PID/NAME` (for example `44011/sshd`): mapped to a host process.
  - `ct/nat`: flow inferred from conntrack/NAT; not directly mapped to host PID.
  - `-`: process info unavailable.
- `SRC`, `PEER`: local endpoint and remote endpoint.
- `STATE`: TCP state (`ESTABLISHED`, `TIME_WAIT`, ...).
- `Send-Q`, `Recv-Q`: queue backlog snapshot at sample time.
- `TX/s`, `RX/s`: throughput computed from conntrack byte deltas in current interval.
- In `View=GROUP`, rows also summarize:
  - `PORTS`: local ports currently represented in the group for `IN`.
  - `RPORTS`: remote service ports currently represented in the group for `OUT`.

`View=CONN` vs `View=GROUP`:

- `CONN`: per-connection view (best for detailed flow debugging).
- `GROUP`: grouped by `(peer, process)` to see ownership split and heavier groups.
  - Example: same peer with `sshd` and `ct/nat` appears as separate rows.
- `IN` vs `OUT`:
  - `IN`: port filter and grouped `PORTS` refer to local service ports.
  - `OUT`: port filter and grouped `RPORTS` refer to remote destination ports; mitigation is disabled and the panel is read-only for visibility.

Quick interpretation:

- High `Send-Q` + low `TX/s`: sender is likely blocked by downstream path.
- High `Recv-Q`: application is reading too slowly.
- `TX/s`,`RX/s` at `0B/s`: idle flow or not enough baseline yet.
- `Selected Detail`: in live mode, the footer preview explains the currently selected row and what `Enter` / `k` would target.
  - In `GROUP`, this is especially useful because the action still resolves to one concrete `peer + local port` target.
  - The full grouped state mix is shown there (`States: EST ... - TW ... - CW ...`), not in the row list anymore.
- In `OUT`, `Selected Detail` switches to remote-port context and explicitly marks `Enter` / `k` as disabled.
- The live Top panel hides TCP connections owned by the current `holyf-network` process so update-check/control traffic does not pollute operator-facing rows.

## 2) Connection States

- Shows connection count distribution by TCP state.
- `Retrans` indicates TCP path quality.
- `LOW SAMPLE` means sample is too small for reliable retrans verdict.

When to worry:

- Persistently high retrans with sufficient `out seg/s`.
- Rapid `TIME_WAIT` growth with high churn.

## 3) Interface Stats

- `RX/TX`: NIC-level throughput (bytes/s).
- `Packets`: packet rate RX/TX.
- `Errors`, `Drops`: NIC error/drop counters.

How to correlate:

- High interface traffic but low Top rows: traffic may be short-lived between samples.
- Low interface traffic but one very high Top row: a few flows dominate usage.

## 4) Conntrack

This panel shows kernel state-table usage (`nf_conntrack`), not “how many outbound ports are open”.

- `Used / Max`: tracked entries vs capacity.
- `Drops`: failed inserts (often table pressure/full conditions).

In the live panel, focus on state-table pressure:

- prioritize `Used / Max` and `Conntrack%`
- pay special attention to `Drops` only when it is non-zero
  - `Drops: 0` is intentionally hidden in the panel to keep noise down

Common operating thresholds:

- `Conntrack% > 70%`: warning zone.
- `Conntrack% > 85%`: critical zone.

## 5) Diagnosis

- A dedicated live panel with:
  - `Issue`: the primary host-level problem right now
  - `Scope`: the dominant peer/port when one is clear, otherwise `host-wide`
  - `Signal`: a compact metric line that ties the issue back to TCP states / retrans / conntrack
  - `Likely`: the short operating interpretation
  - `Check`: the next thing to inspect
- It stays host-global in v1, so filter/search/group selection in `Top Connections` does not narrow it.
- It is meant to connect the other panels quickly, not replace them.

## Live vs Replay

- `Live`:
  - realtime view.
  - can execute block/kill actions (with proper privileges).
- `Replay`:
  - read-only timeline from snapshots.
  - default `holyf-network replay` loads current day (server local time).
  - use `-f`, `-b`, `-e` to narrow scope.
  - `Diagnosis` is not rendered in this phase.

## Block vs Kill

- `minutes > 0`:
  - the app inserts a block first
  - re-scans and kills all connections matching `peer + local port`
  - keeps the block until expiry
- `minutes = 0`:
  - the app only runs the kill sweep, with no block rule
  - if a conn storm is still in progress, new matching connections can appear during the sweep
- `TIME_WAIT` does not count as kill failure. If you see `remaining N (storm/race)`, active flows were still present when the bounded sweep ended.

## Troubleshooting Playbook

## Case 1: Interface traffic is high but Top is unclear

Quick checks:

```bash
sudo ss -ntp
sudo conntrack -L -p tcp | head -n 30
sudo sysctl net.netfilter.nf_conntrack_acct
```

Actions:

1. Reduce interval to `5-10s` for bandwidth-focused monitoring.
2. Press `r` to collect additional baseline samples.
3. Use `f` (port filter) and `/` (text search) to narrow context.

## Case 2: Docker/MySQL flow has no real process name

`ct/nat` is expected for host-facing NAT tuples.

```bash
sudo conntrack -L -p tcp | grep -E 'dport=3306|sport=3306'
docker ps
# optional deep netns debugging:
sudo nsenter -t <container_pid> -n ss -ntp | grep ':3306'
```

## Case 3: `TX/s` / `RX/s` stays zero

Typical causes:

1. First sample has no previous baseline.
2. Flows are too short-lived for current sampling window.
3. Privilege/accounting mismatch on host conntrack path.

## Case 4: Kill shows `remaining N (storm/race)`

Meaning:

1. The app already ran iterative kill sweep (`ss -K` + `conntrack -D`) but stopped at bounded limits.
2. This is common during conn storms where new flows keep appearing.
3. `TIME_WAIT` is informational only; the app only treats non-`TIME_WAIT` active states as not-yet-cleared.

Actions:

1. Use timed block (`minutes > 0`) when you need stronger mitigation than kill-only.
2. Keep port filter enabled and observe a few refresh cycles to confirm the trend is down.

## Quick action cheatsheet

| Symptom | Operational meaning | Immediate action |
|---|---|---|
| `Conntrack%` stays high | Kernel state table pressure | Check `nf_conntrack_max`, reduce churn, find flow source |
| `Conntrack Drops > 0` | New flows cannot be inserted | Address capacity/churn first, inspect firewall/NAT rules |
| `ct/nat` dominates | Traffic mostly NAT/container path | Filter by port and validate with conntrack |
| High `Send-Q` persists | Sender path is congested/blocked | Check downstream latency/receive behavior |
| High `Recv-Q` persists | Application cannot consume fast enough | Check app CPU/read loop bottlenecks |
| High retrans (not LOW SAMPLE) | TCP path quality issue | Inspect loss/RTT/path and NIC errors/drops |
