package collector

import "github.com/BlackMetalz/holyf-network/internal/kernelapi"

var (
	socketMgr    kernelapi.SocketManager
	conntrackMgr kernelapi.ConntrackManager
)

// SetManagers injects kernel API implementations for socket and conntrack
// operations. When set, the collector functions use the kernel API instead
// of shelling out to ss/conntrack commands.
func SetManagers(sm kernelapi.SocketManager, cm kernelapi.ConntrackManager) {
	socketMgr = sm
	conntrackMgr = cm
}
