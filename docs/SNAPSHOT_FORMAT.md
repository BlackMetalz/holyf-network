# Daemon Snapshot Format

This document defines the on-disk snapshot format used by daemon/replay.

## Format Overview

- File format: JSON Lines (`.jsonl`)
- Spec reference: https://jsonlines.org/
- Encoding: UTF-8 text
- Contract: one complete JSON object per line
- Each line is one `SnapshotRecord`
- Append order is chronological

## Segment Naming and Rotation

- Segment filename pattern: `connections-YYYYMMDD.jsonl`
- Day boundary uses server local time
- Daemon appends into current-day segment
- New local day creates a new segment automatically

## Retention and Prune Timing

- Retention policy is age-based: remove segments older than `--retention-hours`.
- Daemon runtime prune cadence:
  - once at daemon startup
  - daily at local server `00:00`
- Manual on-demand prune:
  - `holyf-network daemon prune`
  - useful when you want immediate cleanup without waiting for midnight

## Record Schema

`SnapshotRecord` fields:

- `captured_at` (RFC3339 timestamp with zone)
- `interface` (network interface name, for example `eth0`)
- `top_limit_per_side` (max aggregate rows stored for `incoming_groups` and for `outgoing_groups`)
- `sample_seconds` (elapsed sampling interval used for rate calculations)
- `bandwidth_available` (whether conntrack bandwidth enrichment was available)
- `incoming_groups` (array of aggregate rows for listener-backed traffic)
- `outgoing_groups` (array of aggregate rows for dial-out traffic)
- `version` (application version string at write time)
- `cpu_cores` (daemon process CPU usage in logical cores, omitted if zero/first sample)
- `rss_bytes` (daemon process resident set size in bytes, omitted if zero)

`incoming_groups[]` and `outgoing_groups[]` row fields:

- `peer_ip`
- `port`
  - for `incoming_groups`: local service/listener port
  - for `outgoing_groups`: remote service/destination port
- `proc_name` (for example `sshd`, `ct/nat`)
- `conn_count`
- `tx_queue`
- `rx_queue`
- `total_queue`
- `tx_bytes_delta`
- `rx_bytes_delta`
- `total_bytes_delta`
- `tx_bytes_per_sec`
- `rx_bytes_per_sec`
- `total_bytes_per_sec`
- `states` (map, for example `{"ESTABLISHED": 2}`)

## Example (one line)

```json
{"captured_at":"2026-03-08T12:56:30.196962352+07:00","interface":"eth0","top_limit_per_side":500,"sample_seconds":29.999999695,"bandwidth_available":true,"incoming_groups":[{"peer_ip":"172.25.110.116","port":22,"proc_name":"sshd","conn_count":2,"tx_queue":0,"rx_queue":0,"total_queue":0,"tx_bytes_delta":377892,"rx_bytes_delta":41164,"total_bytes_delta":419056,"tx_bytes_per_sec":12596.400128063402,"rx_bytes_per_sec":1372.1333472833558,"total_bytes_per_sec":13968.533475346758,"states":{"ESTABLISHED":2}}],"outgoing_groups":[{"peer_ip":"20.205.243.168","port":443,"proc_name":"curl","conn_count":1,"tx_queue":0,"rx_queue":0,"total_queue":0,"tx_bytes_delta":0,"rx_bytes_delta":0,"total_bytes_delta":0,"tx_bytes_per_sec":0,"rx_bytes_per_sec":0,"total_bytes_per_sec":0,"states":{"ESTABLISHED":1}}],"version":"v0.3.46","cpu_cores":0.03,"rss_bytes":14876672}
```

Note: `cpu_cores` and `rss_bytes` are omitted (`omitempty`) when zero. Old snapshots without these fields load fine — the replay TUI simply does not display the CPU/RSS indicator.

## Compatibility Policy

- Current policy: single active format only (no legacy schema reader).
- Replay expects this aggregate `incoming_groups/outgoing_groups` snapshot shape.
- Corrupt/non-parseable lines are skipped by reader/indexer.

## Quick Verification Commands

```bash
# count snapshots in a segment
wc -l /var/lib/holyf-network/snapshots/connections-YYYYMMDD.jsonl

# inspect latest snapshots
tail -n 3 /var/lib/holyf-network/snapshots/connections-YYYYMMDD.jsonl
```
