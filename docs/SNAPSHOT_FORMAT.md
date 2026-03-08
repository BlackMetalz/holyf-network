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
- Prune is write-driven (not a separate cron):
  - runs right after segment rotate/open (first write after start, and day rollover)
  - runs periodically every 10 appended snapshots (`PruneEverySnapshots=10`)
- Effective periodic prune cadence in daemon mode is approximately:
  - `--interval * 10`
  - Example: `--interval=30s` => periodic prune check about every 5 minutes

## Record Schema

`SnapshotRecord` fields:

- `captured_at` (RFC3339 timestamp with zone)
- `interface` (network interface name, for example `eth0`)
- `top_limit` (max aggregate rows stored in this snapshot)
- `sample_seconds` (elapsed sampling interval used for rate calculations)
- `bandwidth_available` (whether conntrack bandwidth enrichment was available)
- `groups` (array of aggregate rows)
- `version` (application version string at write time)

`groups[]` row fields:

- `peer_ip`
- `local_port`
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
{"captured_at":"2026-03-08T12:56:30.196962352+07:00","interface":"eth0","top_limit":500,"sample_seconds":29.999999695,"bandwidth_available":true,"groups":[{"peer_ip":"172.25.110.116","local_port":22,"proc_name":"sshd","conn_count":2,"tx_queue":0,"rx_queue":0,"total_queue":0,"tx_bytes_delta":377892,"rx_bytes_delta":41164,"total_bytes_delta":419056,"tx_bytes_per_sec":12596.400128063402,"rx_bytes_per_sec":1372.1333472833558,"total_bytes_per_sec":13968.533475346758,"states":{"ESTABLISHED":2}}],"version":"v0.3.16"}
```

## Compatibility Policy

- Current policy: single active format only (no legacy schema reader).
- Replay expects this aggregate snapshot shape.
- Corrupt/non-parseable lines are skipped by reader/indexer.

## Quick Verification Commands

```bash
# count snapshots in a segment
wc -l /var/lib/holyf-network/snapshots/connections-YYYYMMDD.jsonl

# inspect latest snapshots
tail -n 3 /var/lib/holyf-network/snapshots/connections-YYYYMMDD.jsonl
```
