package kernelapi

// SocketManager abstracts TCP socket querying and killing.
// Replaces all ss command invocations.
type SocketManager interface {
	// QueryEstablished returns established sockets matching peer IP and local port.
	QueryEstablished(peerIP string, localPort int) ([]SocketTuple, error)

	// QueryPeerSnapshot returns a snapshot of socket states for a peer+port pair.
	QueryPeerSnapshot(peerIP string, localPort int) (PeerSocketSnapshot, error)

	// CollectTCPCounters returns per-socket byte counters for all TCP connections.
	CollectTCPCounters() ([]SocketCounter, error)

	// KillSocket destroys a specific socket by exact 4-tuple.
	KillSocket(tuple SocketTuple) error

	// KillByPeerAndPort kills all sockets matching peer IP and port.
	KillByPeerAndPort(peerIP string, port int) error

	// BroadKill attempts to kill sockets using multiple address format combinations.
	BroadKill(peerIP string, port string)
}

// ConntrackManager abstracts conntrack table operations.
// Replaces all conntrack command invocations.
type ConntrackManager interface {
	// ReadStats returns cumulative insert and drop counters.
	ReadStats() (inserts, drops int64, ok bool)

	// CollectFlowsTCP returns all current TCP conntrack flows with byte counters.
	CollectFlowsTCP() ([]ConntrackFlow, error)

	// DeleteFlows removes conntrack entries matching peer IP and port.
	DeleteFlows(peerIP string, port int) error
}

// Firewall abstracts firewall rule management.
// Replaces all iptables/ip6tables command invocations.
type Firewall interface {
	// ListBlockedPeers returns all peer blocks managed by holyf-network.
	ListBlockedPeers() ([]PeerBlockSpec, error)

	// BlockPeer inserts DROP rules for peer IP on the target local port.
	BlockPeer(spec PeerBlockSpec) error

	// UnblockPeer removes previously inserted block rules.
	UnblockPeer(spec PeerBlockSpec) error
}
