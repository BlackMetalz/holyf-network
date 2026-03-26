package actions

import (
	"fmt"
	"net"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

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

// ListBlockedPeers inspects firewall INPUT rules and returns all peer blocks
// that were created by holyf-network (based on rule comment prefix).
func ListBlockedPeers() ([]PeerBlockSpec, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("peer blocking is only supported on Linux")
	}

	specs, err := firewall.ListBlockedPeers()
	if err != nil {
		return nil, err
	}

	blocks := make([]PeerBlockSpec, 0, len(specs))
	for _, s := range specs {
		blocks = append(blocks, fromKernelPeerBlockSpec(s))
	}

	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].PeerIP != blocks[j].PeerIP {
			return blocks[i].PeerIP < blocks[j].PeerIP
		}
		return blocks[i].LocalPort < blocks[j].LocalPort
	})

	return blocks, nil
}

// BlockPeer inserts INPUT+OUTPUT DROP rules for peer IP on the target local port.
func BlockPeer(spec PeerBlockSpec) error {
	_, _, err := validateSpec(spec)
	if err != nil {
		return err
	}

	return firewall.BlockPeer(toKernelPeerBlockSpec(spec))
}

// UnblockPeer removes a previously inserted block rule.
func UnblockPeer(spec PeerBlockSpec) error {
	_, _, err := validateSpec(spec)
	if err != nil {
		return err
	}

	return firewall.UnblockPeer(toKernelPeerBlockSpec(spec))
}

// DropPeerConnections removes matching conntrack entries (best effort).
func DropPeerConnections(spec PeerBlockSpec) error {
	_, peerIP, err := validateSpec(spec)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)
	ssErr := killSocketByPeerAndPort(peerIP, port)
	ctErr := deleteConntrackFlows(peerIP, spec.LocalPort)

	// At least one mechanism succeeded.
	if ssErr == nil || ctErr == nil {
		return nil
	}

	return fmt.Errorf("%s; %s", ssErr.Error(), ctErr.Error())
}

// CountEstablishedPeerSockets returns the number of established sockets that
// currently match peer IP + local port.
func CountEstablishedPeerSockets(spec PeerBlockSpec) (int, error) {
	_, peerIP, err := validateSpec(spec)
	if err != nil {
		return 0, err
	}

	tuples, err := queryEstablishedSockets(peerIP, spec.LocalPort)
	if err != nil {
		return 0, err
	}
	return len(tuples), nil
}

// QueryAndKillPeerSockets queries ss in real-time for established connections
// matching the peer IP and local port, then kills each one with ss -K.
//
// Because ss -K always returns exit 0 (even if no socket was killed), we use a
// brute-force strategy: try multiple family/address formats, then re-query to
// verify sockets are actually gone.
func QueryAndKillPeerSockets(spec PeerBlockSpec) error {
	_, peerIP, err := validateSpec(spec)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)

	// Phase 1: Broad kill — try every family/address combination.
	broadKillPeerSockets(peerIP, port)

	// Phase 2: Exact kill — query for remaining sockets, kill each one.
	tuples, _ := queryEstablishedSockets(peerIP, spec.LocalPort)
	if len(tuples) > 0 {
		_ = KillSockets(tuples)
	}

	// Phase 3: Verify — re-query to check if sockets survived.
	remaining, verifyErr := queryEstablishedSockets(peerIP, spec.LocalPort)
	if verifyErr != nil {
		return nil // can't verify, assume success
	}
	if len(remaining) > 0 {
		return fmt.Errorf("%d sockets still alive after kill", len(remaining))
	}
	return nil
}

// broadKillPeerSockets tries to kill sockets using multiple address format
// combinations via the kernelapi SocketManager.
func broadKillPeerSockets(peerIP, port string) {
	if socketMgr == nil {
		return
	}
	socketMgr.BroadKill(peerIP, port)
}

// queryEstablishedSockets uses the kernelapi SocketManager to find established
// connections matching peerIP and localPort.
func queryEstablishedSockets(peerIP string, localPort int) ([]SocketTuple, error) {
	if socketMgr == nil {
		return nil, fmt.Errorf("socket manager not initialized")
	}

	kTuples, err := socketMgr.QueryEstablished(peerIP, localPort)
	if err != nil {
		return nil, err
	}

	return fromKernelSocketTuples(kTuples), nil
}

// KillSockets tries to close exact established sockets via the kernelapi SocketManager.
// Returns nil if at least one socket was closed or if there are no tuples.
func KillSockets(tuples []SocketTuple) error {
	if len(tuples) == 0 {
		return nil
	}

	var errs []string
	success := false
	for _, tuple := range tuples {
		if err := killSocketExact(tuple); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		success = true
	}

	if success {
		return nil
	}
	if len(errs) == 0 {
		return fmt.Errorf("socket-kill: no tuples were closed")
	}
	return fmt.Errorf("socket-kill: %s", strings.Join(errs, " | "))
}

func killSocketExact(tuple SocketTuple) error {
	if socketMgr == nil {
		return fmt.Errorf("socket manager not initialized")
	}

	return socketMgr.KillSocket(toKernelSocketTuple(tuple))
}

func killSocketByPeerAndPort(peerIP, port string) error {
	if socketMgr == nil {
		return fmt.Errorf("socket manager not initialized")
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port: %s", port)
	}

	return socketMgr.KillByPeerAndPort(peerIP, portInt)
}

func deleteConntrackFlows(peerIP string, port int) error {
	if conntrackMgr == nil {
		return fmt.Errorf("conntrack manager not initialized")
	}

	return conntrackMgr.DeleteFlows(peerIP, port)
}

func validateSpec(spec PeerBlockSpec) (ip net.IP, normalized string, err error) {
	if runtime.GOOS != "linux" {
		return nil, "", fmt.Errorf("peer blocking is only supported on Linux")
	}
	if spec.LocalPort < 1 || spec.LocalPort > 65535 {
		return nil, "", fmt.Errorf("invalid local port: %d", spec.LocalPort)
	}

	rawIP := strings.TrimSpace(spec.PeerIP)
	rawIP = strings.TrimPrefix(rawIP, "::ffff:")
	ip = net.ParseIP(rawIP)
	if ip == nil {
		return nil, "", fmt.Errorf("invalid peer IP: %s", spec.PeerIP)
	}

	if v4 := ip.To4(); v4 != nil {
		return v4, v4.String(), nil
	}
	return ip, ip.String(), nil
}
