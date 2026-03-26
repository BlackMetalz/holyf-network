// Package kernelapi provides direct Linux kernel API access for network
// operations, replacing external CLI tools (ss, conntrack, iptables).
package kernelapi

// PeerBlockSpec describes a firewall block target.
type PeerBlockSpec struct {
	PeerIP    string
	LocalPort int
}

// SocketTuple identifies one TCP socket (local endpoint <-> remote endpoint).
type SocketTuple struct {
	LocalIP    string
	LocalPort  int
	RemoteIP   string
	RemotePort int
}

// FlowTuple identifies one directional TCP flow tuple.
type FlowTuple struct {
	SrcIP   string
	SrcPort int
	DstIP   string
	DstPort int
}

// SocketCounter holds monotonic TCP counters for one socket.
type SocketCounter struct {
	Tuple         FlowTuple
	BytesAcked    int64
	BytesReceived int64
}

// PeerSocketSnapshot captures socket state counts for a peer+port pair.
type PeerSocketSnapshot struct {
	ExactTuples   []SocketTuple
	ActiveCount   int
	TimeWaitCount int
}

// ConntrackFlow represents one bidirectional conntrack TCP flow with counters.
type ConntrackFlow struct {
	State      string
	Orig       FlowTuple
	Reply      FlowTuple
	OrigBytes  int64
	ReplyBytes int64
}

// Capabilities describes what kernel APIs are available at runtime.
type Capabilities struct {
	HasNetlinkSockDiag bool // can open NETLINK_SOCK_DIAG
	HasSockDestroy     bool // kernel 4.9+ SOCK_DESTROY
	HasNfConntrack     bool // can open NETLINK_NETFILTER for conntrack
	HasNftables        bool // can create nftables netlink connection
}
