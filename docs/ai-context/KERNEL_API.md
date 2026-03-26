# Kernel API Layer (`internal/kernelapi/`)

## Why

The project originally shelled out to CLI tools (`ss`, `conntrack`, `iptables`/`ip6tables`) for socket operations, connection tracking, and firewall management. This had problems:

- Hard dependency on external binaries being installed
- Process spawning overhead on every TUI tick (30+ `fork`/`exec` per refresh)
- Fragile text parsing of command output
- `ss -K` returning exit 0 even when no socket was killed (ambiguous)

The `kernelapi` package replaces these with direct Linux kernel APIs via netlink sockets. On Linux 4.9+ with `CAP_NET_ADMIN`, zero CLI tools are spawned at runtime. On systems where kernel APIs aren't available, it falls back to the original exec-based behavior transparently.

## Architecture

```
cmd/root.go / cmd/daemon.go
  │
  ├── kernelapi.NewSocketManager()     → netlink or exec fallback
  ├── kernelapi.NewConntrackManager()  → netlink or exec fallback
  ├── kernelapi.NewFirewall()          → nftables or exec fallback
  │
  ├── actions.SetManagers(sm, cm, fw)  → wires into blocking/kill flows
  └── collector.SetManagers(sm, cm)    → wires into data collection
```

### Detection Flow (startup, once)

`kernelapi.Detect()` probes kernel capabilities:

1. **HasNetlinkSockDiag**: try opening `AF_NETLINK` / `NETLINK_SOCK_DIAG` socket
2. **HasSockDestroy**: check kernel version >= 4.9 via `uname`
3. **HasNfConntrack**: try opening `AF_NETLINK` / `NETLINK_NETFILTER` socket
4. **HasNftables**: try `nftables.New()` + `ListTables()` (needs `CAP_NET_ADMIN`)

Each `New*()` constructor picks the netlink implementation if the probe succeeds, otherwise falls back to exec.

### Status Bar Indicator

The TUI status bar shows the active backend:

- `API:kernel` (green) — all 3 subsystems use kernel APIs
- `API:netlink|exec(conntrack)|nftables` (yellow) — mixed, shows which subsystem uses CLI fallback
- `API:stub|stub|stub` — non-Linux (no-op stubs, graceful empty results)

## Package Structure

```
internal/kernelapi/
├── types.go               — Shared types (SocketTuple, ConntrackFlow, PeerBlockSpec, etc.)
├── interfaces.go          — SocketManager, ConntrackManager, Firewall interfaces
├── backend_info.go        — BackendInfo struct for status bar reporting
│
├── socket_netlink.go      — [linux] INET_DIAG netlink implementation
├── socket_exec.go         — Fallback: wraps ss exec calls
│
├── conntrack_netlink.go   — [linux] nfnetlink_conntrack via ti-mo/conntrack
├── conntrack_exec.go      — Fallback: wraps conntrack exec calls
│
├── firewall_nft.go        — [linux] nftables via google/nftables
├── firewall_exec.go       — Fallback: wraps iptables exec calls
│
├── detect_linux.go        — [linux] Runtime capability detection
├── detect_stub.go         — [!linux] Returns empty Capabilities
│
├── new_linux.go           — [linux] Constructors: pick netlink or exec
└── new_stub.go            — [!linux] Constructors: return no-op stubs
```

Build tags: `_netlink.go`, `_nft.go`, `detect_linux.go`, `new_linux.go` compile only on Linux. Everything else compiles cross-platform.

## Interfaces

### SocketManager (replaces `ss`)

```go
type SocketManager interface {
    QueryEstablished(peerIP string, localPort int) ([]SocketTuple, error)
    QueryPeerSnapshot(peerIP string, localPort int) (PeerSocketSnapshot, error)
    CollectTCPCounters() ([]SocketCounter, error)
    KillSocket(tuple SocketTuple) error
    KillByPeerAndPort(peerIP string, port int) error
    BroadKill(peerIP string, port string)
}
```

### ConntrackManager (replaces `conntrack`)

```go
type ConntrackManager interface {
    ReadStats() (inserts, drops int64, ok bool)
    CollectFlowsTCP() ([]ConntrackFlow, error)
    DeleteFlows(peerIP string, port int) error
}
```

### Firewall (replaces `iptables`/`ip6tables`)

```go
type Firewall interface {
    ListBlockedPeers() ([]PeerBlockSpec, error)
    BlockPeer(spec PeerBlockSpec) error
    UnblockPeer(spec PeerBlockSpec) error
}
```

## Kernel API Details

### 1. INET_DIAG / sock_diag (SocketManager)

**What it does**: Query and destroy TCP sockets directly via the kernel's `NETLINK_SOCK_DIAG` interface — the same mechanism `ss` uses internally.

**How it works**:

```
User space                         Kernel
──────────                         ──────
Open AF_NETLINK socket
  (NETLINK_SOCK_DIAG)
         │
Send inet_diag_req_v2    ──────►  SOCK_DIAG_BY_FAMILY (type 20, modern)
                                  fallback: TCPDIAG_GETSOCK (type 18, legacy)
  - state bitmask                  - filters sockets at kernel level
  - address/port filter            - returns matching sockets only
  - extension flags
         │
Receive nlmsghdr[]        ◄──────  Binary response per socket:
  - inet_diag_msg                  - family, state, src/dst IP+port
  - [tcp_info extension]           - bytes_acked, bytes_received
         │
For kills: send same msg  ──────►  SOCK_DESTROY handler (kernel 4.9+)
  with nlmsg_type=SOCK_DESTROY     - destroys exact socket
                                   - returns confirmed success/failure
```

**Key advantages over `ss`**:
- No process spawn per query (netlink socket stays open)
- Kernel-side filtering by state + address (no client-side text grep)
- `SOCK_DESTROY` returns real success/failure (`ss -K` always exits 0)
- `SOCK_DESTROY` returns real success/failure (`ss -K` always exits 0)

**Known limitation: `CollectTCPCounters()` delegates to exec (`ss -tinHn`)**:
- The `tcp_info` struct offsets for `bytes_acked`/`bytes_received` vary across kernel versions and compile-time config
- Reading them via raw netlink at hardcoded offsets (100/108) produces garbage on some kernels
- The netlink `SocketManager` delegates `CollectTCPCounters()` to the exec fallback which uses `ss -tinHn` — `ss` parses `tcp_info` correctly using the kernel's own formatting code
- Socket query and kill operations still use netlink (struct layout for those is stable)

**Constants** (from `socket_netlink.go`):
- `_SOCK_DIAG_BY_FAMILY = 20` — modern netlink message type for socket query (kernel 4.2+)
- `_TCPDIAG_GETSOCK = 18` — legacy netlink message type (fallback)
- `_SOCK_DESTROY = 21` — netlink message type for socket kill
- `_INET_DIAG_INFO = 2` — extension flag to request tcp_info
- State bitmask: `1 << TCP_ESTABLISHED`, `0xFFF` for all states
- The implementation tries `SOCK_DIAG_BY_FAMILY` first, falls back to `TCPDIAG_GETSOCK` if the kernel returns EINVAL

**IPv4/IPv6 handling**: Sends separate requests for `AF_INET` and `AF_INET6`. IPv4-mapped IPv6 addresses (`::ffff:x.x.x.x`) are normalized by stripping the prefix.

### 2. nfnetlink_conntrack (ConntrackManager)

**What it does**: Query, dump, and delete conntrack table entries via the kernel's `NETLINK_NETFILTER` subsystem.

**Library**: `github.com/ti-mo/conntrack` (pure Go, no CGO)

**How it works**:

```
User space                         Kernel
──────────                         ──────
conntrack.Dial(nil)       ──────►  Open NETLINK_NETFILTER socket
                                   (NFNL_SUBSYS_CTNETLINK)
         │
c.Dump(nil)               ──────►  IPCTNL_MSG_CT_GET_CTRZERO
                                   - dumps all conntrack entries
         │
Receive []Flow             ◄──────  Structured Flow objects:
  - TupleOrig/Reply                - src/dst IP (netip.Addr)
  - Proto (protocol, ports)        - src/dst ports
  - CountersOrig/Reply             - byte counters per direction
  - ProtoInfo.TCP.State            - TCP state (uint8)
         │
c.Delete(flow)             ──────►  IPCTNL_MSG_CT_DELETE
                                   - removes specific conntrack entry
         │
c.Stats()                  ──────►  IPCTNL_MSG_CT_GET_STATS_CPU
                                   - per-CPU insert/drop counters
```

**Key advantages over `conntrack` CLI**:
- No process spawn per dump (was spawning `conntrack -L` twice per tick with different `-o` flags)
- Structured Flow objects instead of 175+ lines of text parsing
- TCP state as typed uint8 instead of string parsing
- Byte counters as uint64 instead of text parsing
- Delete by flow object instead of constructing 4 different arg combinations

**Byte accounting guard**: The netlink implementation checks `/proc/sys/net/netfilter/nf_conntrack_acct` before using byte counters. When `nf_conntrack_acct=0`, all byte counters are forced to zero to prevent garbage values.

**TCP state mapping** (kernel `nf_conntrack_tcp.h` values):
```
0=NONE  1=SYN_SENT  2=SYN_RECV  3=ESTABLISHED
4=FIN_WAIT  5=CLOSE_WAIT  6=LAST_ACK  7=TIME_WAIT
8=CLOSE  9=SYN_SENT2
```

### 3. nftables (Firewall)

**What it does**: Manage firewall DROP rules for peer blocking using the nftables netlink API instead of `iptables`/`ip6tables` commands.

**Library**: `github.com/google/nftables` (pure Go, no CGO)

**Table design**:

```
table inet holyf-network {        ← inet family: handles IPv4 + IPv6
    chain input {
        type filter hook input priority 0;
        tcp dport 8080 ip saddr 10.0.0.1 drop
            comment "holyf-network-peer-block:8080"
    }
    chain output {
        type filter hook output priority 0;
        tcp sport 8080 ip daddr 10.0.0.1 drop
            comment "holyf-network-peer-block:8080"
    }
}
```

**Key advantages over `iptables`**:
- **One table for both IPv4 and IPv6** (`inet` family) instead of calling both `iptables` and `ip6tables`
- Atomic commit: add multiple rules and flush once
- Rule `UserData` field for tagging instead of `-m comment` module
- Direct rule deletion by handle instead of matching exact rule text
- Auto-creates table and chains on first use (`ensureTable` helper)

**How rules are identified**: Each rule's `UserData` field stores `holyf-network-peer-block:<port>` as a tag. `ListBlockedPeers()` reads this tag to find managed rules. No text parsing of `iptables -S` output.

## Wiring

### Initialization (cmd/root.go, cmd/daemon.go)

```go
sm := kernelapi.NewSocketManager()       // auto-detect: netlink or exec
cm := kernelapi.NewConntrackManager()    // auto-detect: netlink or exec
fw := kernelapi.NewFirewall()            // auto-detect: nftables or exec
actions.SetManagers(sm, cm, fw)          // wire into blocking/kill flows
collector.SetManagers(sm, cm)            // wire into data collection
```

### Call Flow (per TUI tick)

```
refreshData()
  │
  ├── collector.CollectConntrackFlowsTCP()
  │     └── conntrackMgr.CollectFlowsTCP()     → netlink dump or exec fallback
  │
  ├── collector.CollectSocketTCPCounters()
  │     └── socketMgr.CollectTCPCounters()      → INET_DIAG or exec fallback
  │
  └── (on block/kill action)
        ├── actions.BlockPeer()
        │     └── firewall.BlockPeer()           → nftables or iptables exec
        ├── actions.KillPeerFlows()
        │     ├── socketMgr.BroadKill()          → SOCK_DESTROY or ss -K
        │     ├── socketMgr.QueryPeerSnapshot()  → INET_DIAG or ss -tnp
        │     ├── socketMgr.KillSocket()         → SOCK_DESTROY or ss -K
        │     └── conntrackMgr.DeleteFlows()     → netlink delete or conntrack -D
        └── actions.UnblockPeer()
              └── firewall.UnblockPeer()         → nftables or iptables exec
```

## Go Dependencies

| Library | CGO | Purpose |
|---------|-----|---------|
| `golang.org/x/sys/unix` | No | Raw INET_DIAG netlink, uname, getrusage |
| `github.com/ti-mo/conntrack` | No | Conntrack netlink (query/delete/stats) |
| `github.com/google/nftables` | No | nftables netlink (rules management) |

All pure Go. No CGO required. `tcpdump` is the only remaining external tool (for packet capture feature only).

## Requirements

- **Linux kernel 4.9+** for `SOCK_DESTROY` support
- **`CAP_NET_ADMIN`** (or root) for netlink socket operations
- **`nf_conntrack` module loaded** for conntrack netlink
- **`nf_tables` module loaded** for nftables (most modern distros have this)

When any of these aren't met, the specific subsystem falls back to exec (CLI tools) transparently.

## Fallback Behavior

| Condition | Socket query/kill | Socket counters | Conntrack | Firewall |
|-----------|-------------------|-----------------|-----------|----------|
| Linux 4.9+ with CAP_NET_ADMIN | netlink | exec(ss)* | netlink | nftables |
| Linux without CAP_NET_ADMIN | exec(ss) | exec(ss) | exec(conntrack) | exec(iptables) |
| Linux without nf_conntrack | netlink | exec(ss) | exec(conntrack) | nftables |
| Non-Linux (macOS, etc.) | stub (no-op) | stub (no-op) | stub (no-op) | stub (no-op) |
| CLI tool not installed | error on action | graceful empty | graceful empty | error on action |

*Socket counters (`CollectTCPCounters`) always delegate to `ss -tinHn` even when netlink is available, because `tcp_info` byte counter offsets in the raw struct are unreliable across kernel versions.

## Bandwidth Sanity Guards

The bandwidth tracker (`internal/collector/bandwidth_tracker.go`) applies two guards:

1. **First-seen skip**: When a flow appears for the first time after baseline, its accumulated historical byte count is NOT treated as a single-interval delta. Delta = 0 for first-seen; real delta shows on next sample.
2. **Per-flow cap**: Any per-flow delta exceeding 12.5 GB/s (100 Gbps) per direction is zeroed as a counter anomaly. Prevents display of petabyte/sec rates from counter bugs or kernel quirks.

These guards apply to both the conntrack bandwidth tracker and the socket bandwidth tracker.

"Graceful empty" means passive collection returns empty results (no error). Active actions (block/kill) return errors since the user needs to know the action failed.
