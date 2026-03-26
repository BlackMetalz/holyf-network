//go:build linux

package kernelapi

import (
	"fmt"
	"net"

	"github.com/ti-mo/conntrack"
)

// NetlinkConntrackManager implements ConntrackManager using the kernel
// conntrack netlink API via the ti-mo/conntrack library.
type NetlinkConntrackManager struct{}

func (n *NetlinkConntrackManager) BackendName() string { return "netlink" }

// NewNetlinkConntrackManager creates a new netlink-based conntrack manager.
func NewNetlinkConntrackManager() (*NetlinkConntrackManager, error) {
	// Verify connectivity by opening and immediately closing a connection.
	c, err := conntrack.Dial(nil)
	if err != nil {
		return nil, fmt.Errorf("conntrack netlink dial: %w", err)
	}
	c.Close()
	return &NetlinkConntrackManager{}, nil
}

// ReadStats returns cumulative conntrack insert and drop counters.
func (m *NetlinkConntrackManager) ReadStats() (inserts, drops int64, ok bool) {
	c, err := conntrack.Dial(nil)
	if err != nil {
		return 0, 0, false
	}
	defer c.Close()

	stats, err := c.Stats()
	if err != nil {
		return 0, 0, false
	}

	for _, s := range stats {
		inserts += int64(s.Inserted)
		drops += int64(s.Drop)
	}
	return inserts, drops, true
}

// CollectFlowsTCP returns all current TCP conntrack flows with byte counters.
func (m *NetlinkConntrackManager) CollectFlowsTCP() ([]ConntrackFlow, error) {
	c, err := conntrack.Dial(nil)
	if err != nil {
		return nil, fmt.Errorf("conntrack dial: %w", err)
	}
	defer c.Close()

	// Dump all conntrack entries.
	flows, err := c.Dump(nil)
	if err != nil {
		return nil, fmt.Errorf("conntrack dump: %w", err)
	}

	var result []ConntrackFlow
	for _, f := range flows {
		// Filter TCP only (protocol 6).
		if f.TupleOrig.Proto.Protocol != 6 {
			continue
		}

		cf := ConntrackFlow{
			State: protoInfoStateString(f),
			Orig: FlowTuple{
				SrcIP:   normalizeConntrackIP(f.TupleOrig.IP.SourceAddress),
				SrcPort: int(f.TupleOrig.Proto.SourcePort),
				DstIP:   normalizeConntrackIP(f.TupleOrig.IP.DestinationAddress),
				DstPort: int(f.TupleOrig.Proto.DestinationPort),
			},
			Reply: FlowTuple{
				SrcIP:   normalizeConntrackIP(f.TupleReply.IP.SourceAddress),
				SrcPort: int(f.TupleReply.Proto.SourcePort),
				DstIP:   normalizeConntrackIP(f.TupleReply.IP.DestinationAddress),
				DstPort: int(f.TupleReply.Proto.DestinationPort),
			},
			OrigBytes:  int64(f.CountersOrig.Bytes),
			ReplyBytes: int64(f.CountersReply.Bytes),
		}
		result = append(result, cf)
	}
	return result, nil
}

// DeleteFlows removes conntrack entries matching peerIP and port.
func (m *NetlinkConntrackManager) DeleteFlows(peerIP string, port int) error {
	c, err := conntrack.Dial(nil)
	if err != nil {
		return fmt.Errorf("conntrack dial: %w", err)
	}
	defer c.Close()

	// Dump all flows and delete matching ones.
	flows, err := c.Dump(nil)
	if err != nil {
		return fmt.Errorf("conntrack dump: %w", err)
	}

	normPeer := normalizeIP(peerIP)
	var lastErr error
	for _, f := range flows {
		if f.TupleOrig.Proto.Protocol != 6 {
			continue
		}

		origSrc := normalizeConntrackIP(f.TupleOrig.IP.SourceAddress)
		origDst := normalizeConntrackIP(f.TupleOrig.IP.DestinationAddress)
		origSPort := int(f.TupleOrig.Proto.SourcePort)
		origDPort := int(f.TupleOrig.Proto.DestinationPort)

		match := false
		// Match if peer is source and port matches either src or dst port.
		if origSrc == normPeer && (origSPort == port || origDPort == port) {
			match = true
		}
		// Match if peer is destination and port matches.
		if origDst == normPeer && (origSPort == port || origDPort == port) {
			match = true
		}

		if match {
			if err := c.Delete(f); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// normalizeConntrackIP converts a net.IP from conntrack to a normalized string.
func normalizeConntrackIP(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4.String()
	}
	return ip.String()
}

// protoInfoStateString returns the TCP state string from a conntrack flow.
func protoInfoStateString(f conntrack.Flow) string {
	if f.ProtoInfo.TCP != nil {
		return f.ProtoInfo.TCP.State.String()
	}
	return "UNKNOWN"
}

// Compile-time interface check.
var _ ConntrackManager = (*NetlinkConntrackManager)(nil)
