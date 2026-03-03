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

### macOS (Apple Silicon) - NOT SUPPORTED!
```bash
curl -sL https://github.com/BlackMetalz/holyf-network/releases/latest/download/holyf-network-darwin-arm64 -o /tmp/holyf-network
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

# Demo mode: hide first 2 IP prefixes/groups in Top Connections
holyf-network --sensitive-ip

# Runtime toggle inside TUI
# Press `s` to turn sensitive IP masking on/off
# Press `k` on Top Connections to block peer IP -> local port for 10 minutes
```
