# BW Test
```bash
# Create 1 long TCP flow, stable!
curl --http1.1 -L http://speedtest.tele2.net/1GB.zip -o /dev/null
```

# Daemon Retention/Prune Quick Checks
```bash
# Start daemon with short retention for lab testing
holyf-network daemon start --interval 30 --retention-hours 24

# Manual prune now (uses active-state target by default)
holyf-network daemon prune

# Manual prune with explicit target + override retention
holyf-network daemon prune --data-dir /var/lib/holyf-network/snapshots --retention-hours 12

# Stop daemon
holyf-network daemon stop
```
