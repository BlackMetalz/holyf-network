# Network Visibility tool
For learning Go purpose and a tool that I can use daily for network monitor when a server has alert related to network issues!
I'm suck at Linux Network!

# Vibe Code
Yes, fucking yes.

# Download

### Requirements

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
# o: cycle sort mode (Queue -> State -> Peer -> Process)
# Shift+Q / Shift+S / Shift+P / Shift+R: set sort mode directly
# g: toggle group-by-peer view
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
