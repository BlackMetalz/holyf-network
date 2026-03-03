# Network Visibility tool
For learning Go purpose and a tool that I can use daily for network monitor when a server has alert related to network issues!
I'm suck at Linux Network!

# Vibe Code
Yes, fucking yes.

# Download

### Requirements

Required root to run this tool:

```bash
apt install conntrack
```

### Ubuntu/Linux (amd64)
```bash
curl -sL https://github.com/BlackMetalz/holyf-network/releases/latest/download/holyf-network-linux-amd64 -o /tmp/holyf-network
chmod +x /tmp/holyf-network
sudo mv /tmp/holyf-network /usr/local/bin/holyf-network
holyf-network -v
```

# Usage
```bash
# Default refresh rate is 30 seconds
holyf-network

# Set refresh rate to 5 seconds
holyf-network -r 5

# Runtime toggle inside TUI
# Press `s` to turn sensitive IP masking on/off
# Press `k` on Top Connections to input peer IP/local port + block minutes (default: 10)
# Press `b` to list active blocks and remove selected peer block
# Press `f` to filter Top Connections by port
# Press `p` to pause/resume auto-refresh
# Press `r` to refresh now
# Press `z` to zoom/unzoom panel
# Press `?` to show help
# Press `q` to quit
```

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
```

In case you want to custom threshold:
```bash
# Use custom health strip thresholds
holyf-network --health-config /etc/holyf-network/health_thresholds.toml
```
