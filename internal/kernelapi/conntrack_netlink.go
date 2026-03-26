//go:build linux

package kernelapi

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/ti-mo/conntrack"
)

// NetlinkConntrackManager implements ConntrackManager using the kernel
// conntrack netlink API via the ti-mo/conntrack library.
type NetlinkConntrackManager struct{}

func (n *NetlinkConntrackManager) BackendName() string { return "netlink" }

// NewNetlinkConntrackManager creates a new netlink-based conntrack manager.
func NewNetlinkConntrackManager() (*NetlinkConntrackManager, error) {
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
		inserts += int64(s.Insert)
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

	flows, err := c.Dump(nil)
	if err != nil {
		return nil, fmt.Errorf("conntrack dump: %w", err)
	}

	var result []ConntrackFlow
	for _, f := range flows {
		if f.TupleOrig.Proto.Protocol != 6 {
			continue
		}

		cf := ConntrackFlow{
			State: tcpConntrackStateString(f),
			Orig: FlowTuple{
				SrcIP:   normalizeNetipAddr(f.TupleOrig.IP.SourceAddress),
				SrcPort: int(f.TupleOrig.Proto.SourcePort),
				DstIP:   normalizeNetipAddr(f.TupleOrig.IP.DestinationAddress),
				DstPort: int(f.TupleOrig.Proto.DestinationPort),
			},
			Reply: FlowTuple{
				SrcIP:   normalizeNetipAddr(f.TupleReply.IP.SourceAddress),
				SrcPort: int(f.TupleReply.Proto.SourcePort),
				DstIP:   normalizeNetipAddr(f.TupleReply.IP.DestinationAddress),
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

	flows, err := c.Dump(nil)
	if err != nil {
		return fmt.Errorf("conntrack dump: %w", err)
	}

	normPeer := ctNormalizeIP(peerIP)
	var lastErr error
	for _, f := range flows {
		if f.TupleOrig.Proto.Protocol != 6 {
			continue
		}

		origSrc := normalizeNetipAddr(f.TupleOrig.IP.SourceAddress)
		origDst := normalizeNetipAddr(f.TupleOrig.IP.DestinationAddress)
		origSPort := int(f.TupleOrig.Proto.SourcePort)
		origDPort := int(f.TupleOrig.Proto.DestinationPort)

		match := false
		if origSrc == normPeer && (origSPort == port || origDPort == port) {
			match = true
		}
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

// normalizeNetipAddr converts a netip.Addr to a normalized IP string.
func normalizeNetipAddr(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	// Unmap IPv4-in-IPv6 (::ffff:x.x.x.x → x.x.x.x)
	if addr.Is4In6() {
		addr = netip.AddrFrom4(addr.As4())
	}
	return addr.String()
}

// tcpConntrackStateString maps the kernel TCP conntrack state uint8 to a string.
func tcpConntrackStateString(f conntrack.Flow) string {
	if f.ProtoInfo.TCP == nil {
		return "UNKNOWN"
	}
	return tcpStateToString(f.ProtoInfo.TCP.State)
}

// tcpStateToString maps Linux kernel TCP conntrack state values to names.
// These match the nf_conntrack_tcp.h enum values.
func tcpStateToString(state uint8) string {
	switch state {
	case 0:
		return "NONE"
	case 1:
		return "SYN_SENT"
	case 2:
		return "SYN_RECV"
	case 3:
		return "ESTABLISHED"
	case 4:
		return "FIN_WAIT"
	case 5:
		return "CLOSE_WAIT"
	case 6:
		return "LAST_ACK"
	case 7:
		return "TIME_WAIT"
	case 8:
		return "CLOSE"
	case 9:
		return "SYN_SENT2"
	default:
		return "UNKNOWN"
	}
}

// ctNormalizeIP strips ::ffff: prefix and normalizes for comparison.
func ctNormalizeIP(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "::ffff:")
	return s
}

// Compile-time interface check.
var _ ConntrackManager = (*NetlinkConntrackManager)(nil)
