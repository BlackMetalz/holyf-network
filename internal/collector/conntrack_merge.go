package collector

import (
	"net"
	"strconv"
	"strings"
)

// MergeConntrackHostFlows appends host-facing tuples from conntrack when they
// are missing from /proc socket view (for example Docker DNAT/container netns).
// Existing connections are kept unchanged.
func MergeConntrackHostFlows(conns []Connection, flows []ConntrackFlow) []Connection {
	localIPs := localHostIPSet()
	extendLocalSetFromConnections(localIPs, conns)
	return mergeConntrackHostFlowsWithLocalSet(conns, flows, localIPs)
}

func mergeConntrackHostFlowsWithLocalSet(conns []Connection, flows []ConntrackFlow, localIPs map[string]struct{}) []Connection {
	if len(flows) == 0 || len(localIPs) == 0 {
		return conns
	}

	out := make([]Connection, 0, len(conns)+len(flows))
	out = append(out, conns...)

	seen := make(map[string]struct{}, len(conns)+len(flows))
	for _, conn := range conns {
		seen[connectionTupleKey(conn.LocalIP, conn.LocalPort, conn.RemoteIP, conn.RemotePort)] = struct{}{}
	}

	for _, flow := range flows {
		state := strings.ToUpper(strings.TrimSpace(flow.State))
		if state != "ESTABLISHED" {
			continue
		}
		localIP, localPort, remoteIP, remotePort, ok := hostFacingTuple(flow.Orig, localIPs)
		if !ok {
			continue
		}
		key := connectionTupleKey(localIP, localPort, remoteIP, remotePort)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		out = append(out, Connection{
			LocalIP:    localIP,
			LocalPort:  localPort,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
			State:      "ESTABLISHED",
			TxQueue:    0,
			RxQueue:    0,
			Activity:   0,
			PID:        0,
			ProcName:   "ct/nat",
			Inode:      "",
		})
	}

	return out
}

func hostFacingTuple(orig FlowTuple, localIPs map[string]struct{}) (localIP string, localPort int, remoteIP string, remotePort int, ok bool) {
	srcIP := normalizeFlowIP(orig.SrcIP)
	dstIP := normalizeFlowIP(orig.DstIP)
	_, srcLocal := localIPs[srcIP]
	_, dstLocal := localIPs[dstIP]

	switch {
	case srcLocal && !dstLocal:
		return srcIP, orig.SrcPort, dstIP, orig.DstPort, true
	case dstLocal && !srcLocal:
		// Remote -> local request tuple (common for inbound/DNAT).
		return dstIP, orig.DstPort, srcIP, orig.SrcPort, true
	default:
		return "", 0, "", 0, false
	}
}

func localHostIPSet() map[string]struct{} {
	set := make(map[string]struct{}, 16)
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return set
	}
	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			if v == nil || v.IP == nil {
				continue
			}
			ip := normalizeFlowIP(v.IP.String())
			if ip != "" {
				set[ip] = struct{}{}
			}
		case *net.IPAddr:
			if v == nil || v.IP == nil {
				continue
			}
			ip := normalizeFlowIP(v.IP.String())
			if ip != "" {
				set[ip] = struct{}{}
			}
		}
	}
	return set
}

func extendLocalSetFromConnections(set map[string]struct{}, conns []Connection) {
	if len(conns) == 0 {
		return
	}
	for _, conn := range conns {
		ip := normalizeFlowIP(conn.LocalIP)
		if ip == "" {
			continue
		}
		set[ip] = struct{}{}
	}
}

func connectionTupleKey(localIP string, localPort int, remoteIP string, remotePort int) string {
	return normalizeFlowIP(localIP) + "|" +
		strconv.Itoa(localPort) + "|" +
		normalizeFlowIP(remoteIP) + "|" +
		strconv.Itoa(remotePort)
}
