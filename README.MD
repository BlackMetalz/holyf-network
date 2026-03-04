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

# Keyboard shortcuts inside TUI
# Tab / Shift+Tab: move focus between panels
# Up / Down: select row in Top Connections
# Enter / k: block selected Top Connections row
# o: cycle sort mode (Queue -> State -> Peer -> Process)
# Shift+Q / Shift+S / Shift+P / Shift+R: set sort mode directly
# g: toggle group-by-peer view
# /: search Top Connections by text (contains match)
# f: port filter (press f again to clear all filters)
# s: toggle sensitive IP masking
# b: list active blocks and remove selected peer block
# h: show action log (latest 10)
# p: pause/resume auto-refresh
# r: refresh now
# z: zoom/unzoom focused panel
# ?: show help
# q: quit
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
