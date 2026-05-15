//go:build !linux

package kernelapi

// On non-Linux platforms, return no-op stubs that return empty results
// instead of exec fallbacks that would fail (ss/conntrack/iptables don't exist).

func NewSocketManager() SocketManager       { return &stubSocketManager{} }
func NewConntrackManager() ConntrackManager { return &stubConntrackManager{} }
func NewFirewall() Firewall                 { return &stubFirewall{} }

// --- stub implementations ---

type stubSocketManager struct{}

func (s *stubSocketManager) BackendName() string                              { return "stub" }
func (s *stubSocketManager) QueryEstablished(string, int) ([]SocketTuple, error) { return nil, nil }
func (s *stubSocketManager) QueryPeerSnapshot(string, int) (PeerSocketSnapshot, error) {
	return PeerSocketSnapshot{}, nil
}
func (s *stubSocketManager) CollectTCPCounters() ([]SocketCounter, error) { return nil, nil }
func (s *stubSocketManager) KillSocket(SocketTuple) error                 { return nil }
func (s *stubSocketManager) KillByPeerAndPort(string, int) error          { return nil }
func (s *stubSocketManager) BroadKill(string, string)                     {}

type stubConntrackManager struct{}

func (s *stubConntrackManager) BackendName() string                          { return "stub" }
func (s *stubConntrackManager) ReadStats() (int64, int64, bool)              { return 0, 0, false }
func (s *stubConntrackManager) CollectFlowsTCP() ([]ConntrackFlow, error)    { return nil, nil }
func (s *stubConntrackManager) DeleteFlows(string, int) error                { return nil }

type stubFirewall struct{}

func (s *stubFirewall) BackendName() string                       { return "stub" }
func (s *stubFirewall) ListBlockedPeers() ([]PeerBlockSpec, error) { return nil, nil }
func (s *stubFirewall) BlockPeer(PeerBlockSpec) error              { return nil }
func (s *stubFirewall) UnblockPeer(PeerBlockSpec) error            { return nil }
