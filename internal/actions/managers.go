package actions

import "github.com/BlackMetalz/holyf-network/internal/kernelapi"

var (
	socketMgr    kernelapi.SocketManager
	conntrackMgr kernelapi.ConntrackManager
	firewall     kernelapi.Firewall
)

// SetManagers wires the kernelapi implementations into the actions package.
func SetManagers(sm kernelapi.SocketManager, cm kernelapi.ConntrackManager, fw kernelapi.Firewall) {
	socketMgr = sm
	conntrackMgr = cm
	firewall = fw
}

// --- conversion helpers ---

func toKernelPeerBlockSpec(s PeerBlockSpec) kernelapi.PeerBlockSpec {
	return kernelapi.PeerBlockSpec{
		PeerIP:    s.PeerIP,
		LocalPort: s.LocalPort,
	}
}

func fromKernelPeerBlockSpec(s kernelapi.PeerBlockSpec) PeerBlockSpec {
	return PeerBlockSpec{
		PeerIP:    s.PeerIP,
		LocalPort: s.LocalPort,
	}
}

func toKernelSocketTuple(t SocketTuple) kernelapi.SocketTuple {
	return kernelapi.SocketTuple{
		LocalIP:    t.LocalIP,
		LocalPort:  t.LocalPort,
		RemoteIP:   t.RemoteIP,
		RemotePort: t.RemotePort,
	}
}

func fromKernelSocketTuple(t kernelapi.SocketTuple) SocketTuple {
	return SocketTuple{
		LocalIP:    t.LocalIP,
		LocalPort:  t.LocalPort,
		RemoteIP:   t.RemoteIP,
		RemotePort: t.RemotePort,
	}
}

func fromKernelSocketTuples(ts []kernelapi.SocketTuple) []SocketTuple {
	out := make([]SocketTuple, len(ts))
	for i, t := range ts {
		out[i] = fromKernelSocketTuple(t)
	}
	return out
}
