//go:build linux

package kernelapi

import "log"

// NewSocketManager returns a netlink-based SocketManager if the kernel supports
// NETLINK_SOCK_DIAG, otherwise falls back to the exec-based implementation.
func NewSocketManager() SocketManager {
	caps := Detect()
	if caps.HasNetlinkSockDiag {
		log.Println("[kernelapi] using netlink socket manager")
		return NewNetlinkSocketManager()
	}
	log.Println("[kernelapi] falling back to exec socket manager")
	return NewExecSocketManager()
}

// NewConntrackManager returns a netlink-based ConntrackManager if the kernel
// supports NETLINK_NETFILTER for conntrack, otherwise falls back to exec.
func NewConntrackManager() ConntrackManager {
	caps := Detect()
	if caps.HasNfConntrack {
		mgr, err := NewNetlinkConntrackManager()
		if err == nil {
			log.Println("[kernelapi] using netlink conntrack manager")
			return mgr
		}
		log.Printf("[kernelapi] netlink conntrack init failed: %v, falling back to exec", err)
	}
	log.Println("[kernelapi] falling back to exec conntrack manager")
	return NewExecConntrackManager()
}

// NewFirewall returns an nftables-based Firewall if the kernel supports it,
// otherwise falls back to the exec-based implementation.
func NewFirewall() Firewall {
	caps := Detect()
	if caps.HasNftables {
		fw, err := NewNftFirewall()
		if err == nil {
			log.Println("[kernelapi] using nftables firewall")
			return fw
		}
		log.Printf("[kernelapi] nftables init failed: %v, falling back to exec", err)
	}
	log.Println("[kernelapi] falling back to exec firewall")
	return NewExecFirewall()
}
