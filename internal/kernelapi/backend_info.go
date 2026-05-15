package kernelapi

import "fmt"

// BackendInfo describes which implementation a manager is using.
type BackendInfo struct {
	Socket    string // "netlink" or "exec(ss)"
	Conntrack string // "netlink" or "exec(conntrack)"
	Firewall  string // "nftables" or "exec(iptables)"
}

// Summary returns a compact status string like "netlink|netlink|nftables"
// or "exec(ss)|exec(conntrack)|exec(iptables)".
func (b BackendInfo) Summary() string {
	return fmt.Sprintf("%s|%s|%s", b.Socket, b.Conntrack, b.Firewall)
}

// IsAllKernel returns true if all backends use kernel APIs (no CLI fallback).
func (b BackendInfo) IsAllKernel() bool {
	return b.Socket == "netlink" && b.Conntrack == "netlink" && b.Firewall == "nftables"
}

// IsStub returns true if all backends are stubs (non-Linux).
func (b BackendInfo) IsStub() bool {
	return b.Socket == "stub" && b.Conntrack == "stub" && b.Firewall == "stub"
}

// backendDescriber is implemented by managers that can report their backend name.
type backendDescriber interface {
	BackendName() string
}

// GetBackendInfo inspects the actual implementations and returns BackendInfo.
func GetBackendInfo(sm SocketManager, cm ConntrackManager, fw Firewall) BackendInfo {
	return BackendInfo{
		Socket:    describeManager(sm, "exec(ss)"),
		Conntrack: describeManager(cm, "exec(conntrack)"),
		Firewall:  describeManager(fw, "exec(iptables)"),
	}
}

func describeManager(m interface{}, fallback string) string {
	if d, ok := m.(backendDescriber); ok {
		return d.BackendName()
	}
	return fallback
}
