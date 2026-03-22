# Alert Threshold Profiles (EN)

This document explains profile-based alerting in live mode.

## Goal

Different workloads have different "normal" traffic shapes.  
A single threshold set is noisy for one role and too lax for another.

Profiles solve that by switching alert sensitivity by host role:

- `WEB`
- `DB`
- `CACHE`

## Controls

- Start app with profile:

```bash
sudo holyf-network --alert-profile web
sudo holyf-network --alert-profile db
sudo holyf-network --alert-profile cache
```

- Switch at runtime in live mode:
  - Press `y` to cycle `WEB -> DB -> CACHE`.
  - Press `Shift+Y` to open quick in-app explain modal.

## Current Scope (Phase 1)

Profile thresholds currently affect:

1. `3. Interface Stats` (`Traffic` line spike verdict)
2. `4. Conntrack` warn/crit messaging
3. Conntrack severity in:
   - `Connection States` health strip
   - `5. Diagnosis` decision path

## Profile Thresholds

When NIC speed is known, Interface uses utilization thresholds:

| Profile | Conntrack Warn/Crit | Interface Util Warn/Crit | Spike Ratio Warn/Crit |
|---|---|---|---|
| WEB | `70% / 85%` | `60% / 85%` | `2.0x / 3.0x` |
| DB | `55% / 70%` | `45% / 70%` | `1.6x / 2.2x` |
| CACHE | `75% / 90%` | `70% / 90%` | `2.5x / 3.8x` |

If NIC speed is unavailable/unknown, Interface falls back to absolute peak thresholds:

- `WEB`: `80 / 200 MiB/s`
- `DB`: `40 / 120 MiB/s`
- `CACHE`: `120 / 320 MiB/s`

## Interface Spike Logic

The live panel uses both:

1. **Absolute threshold**
   - If NIC speed is known: compare interface utilization `%` against profile util thresholds.
   - If NIC speed is unknown: compare `peak = max(RX_bytes_per_sec, TX_bytes_per_sec)` against profile absolute thresholds.
2. **Relative threshold**
   - `ratio = peak / max(EWMA_baseline, baseline_floor)`

Severity is the max of both checks.

## Notes

- Baseline warm-up needs a few samples at startup.
- When switching profile (`y`), interface baseline resets intentionally to avoid cross-profile drift.
- Conntrack small non-zero percentages render as `<0.1%` to avoid confusing `0%`.
