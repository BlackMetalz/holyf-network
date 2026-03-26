//go:build linux

package kernelapi

import (
	"encoding/binary"
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// INET_DIAG constants not always exposed in golang.org/x/sys/unix.
const (
	_SOCK_DIAG_BY_FAMILY = 20 // nlmsg_type for inet_diag_req_v2 (modern kernels 4.2+)
	_TCPDIAG_GETSOCK     = 18 // legacy nlmsg_type (fallback for older kernels)
	_SOCK_DESTROY    = 21 // nlmsg_type for SOCK_DESTROY

	_INET_DIAG_INFO = 2 // nlattr type for tcp_info extension

	// TCP states used in state bitmask filters.
	_TCP_ESTABLISHED = 1
	_TCP_TIME_WAIT   = 6

	// Size of inet_diag_req_v2 (56 bytes).
	_INET_DIAG_REQ_V2_SIZE = 56

	// Netlink message header size.
	_NLM_HDR_SIZE = 16

	// Size of inet_diag_msg response header.
	_INET_DIAG_MSG_SIZE = 72

	// Offsets within tcp_info for bytes_acked and bytes_received (kernel 4.1+).
	// These are uint64 fields at offsets 100 and 108 in struct tcp_info.
	_TCP_INFO_BYTES_ACKED_OFF    = 100
	_TCP_INFO_BYTES_RECEIVED_OFF = 108
	_TCP_INFO_MIN_SIZE           = 116
)

// NetlinkSocketManager implements SocketManager using raw NETLINK_SOCK_DIAG.
type NetlinkSocketManager struct{}

func (n *NetlinkSocketManager) BackendName() string { return "netlink" }

// NewNetlinkSocketManager creates a new netlink-based socket manager.
func NewNetlinkSocketManager() *NetlinkSocketManager {
	return &NetlinkSocketManager{}
}

// QueryEstablished returns established sockets matching peerIP and localPort.
func (m *NetlinkSocketManager) QueryEstablished(peerIP string, localPort int) ([]SocketTuple, error) {
	msgs, err := m.dumpSockets(1<<_TCP_ESTABLISHED, 0) // no extensions needed
	if err != nil {
		return nil, fmt.Errorf("netlink sock_diag dump: %w", err)
	}

	normPeer := normalizeIP(peerIP)
	var result []SocketTuple
	for _, msg := range msgs {
		dm, ok := parseDiagMsg(msg)
		if !ok {
			continue
		}
		if dm.localPort != localPort {
			continue
		}
		if normalizeIP(dm.remoteIP) != normPeer {
			continue
		}
		result = append(result, SocketTuple{
			LocalIP:    dm.localIP,
			LocalPort:  dm.localPort,
			RemoteIP:   dm.remoteIP,
			RemotePort: dm.remotePort,
		})
	}
	return result, nil
}

// QueryPeerSnapshot returns a snapshot of socket states for a peer+port pair.
func (m *NetlinkSocketManager) QueryPeerSnapshot(peerIP string, localPort int) (PeerSocketSnapshot, error) {
	// Query all TCP states.
	allStates := uint32(0xFFF) // all 12 TCP states
	msgs, err := m.dumpSockets(allStates, 0)
	if err != nil {
		return PeerSocketSnapshot{}, fmt.Errorf("netlink sock_diag dump: %w", err)
	}

	normPeer := normalizeIP(peerIP)
	var snap PeerSocketSnapshot
	for _, msg := range msgs {
		dm, ok := parseDiagMsg(msg)
		if !ok {
			continue
		}
		if dm.localPort != localPort {
			continue
		}
		if normalizeIP(dm.remoteIP) != normPeer {
			continue
		}

		if dm.state == _TCP_TIME_WAIT {
			snap.TimeWaitCount++
		} else {
			snap.ActiveCount++
			snap.ExactTuples = append(snap.ExactTuples, SocketTuple{
				LocalIP:    dm.localIP,
				LocalPort:  dm.localPort,
				RemoteIP:   dm.remoteIP,
				RemotePort: dm.remotePort,
			})
		}
	}
	return snap, nil
}

// CollectTCPCounters returns per-socket byte counters for all established TCP connections.
func (m *NetlinkSocketManager) CollectTCPCounters() ([]SocketCounter, error) {
	msgs, err := m.dumpSockets(1<<_TCP_ESTABLISHED, 1<<(_INET_DIAG_INFO-1))
	if err != nil {
		return nil, fmt.Errorf("netlink sock_diag dump: %w", err)
	}

	var result []SocketCounter
	for _, msg := range msgs {
		dm, ok := parseDiagMsg(msg)
		if !ok {
			continue
		}
		var acked, received int64
		if ti := extractTCPInfo(msg); ti != nil {
			acked, received = parseTCPInfoCounters(ti)
		}
		result = append(result, SocketCounter{
			Tuple: FlowTuple{
				SrcIP:   dm.localIP,
				SrcPort: dm.localPort,
				DstIP:   dm.remoteIP,
				DstPort: dm.remotePort,
			},
			BytesAcked:    acked,
			BytesReceived: received,
		})
	}
	return result, nil
}

// KillSocket destroys a specific socket by exact 4-tuple using SOCK_DESTROY.
func (m *NetlinkSocketManager) KillSocket(tuple SocketTuple) error {
	return m.destroySocket(tuple.LocalIP, tuple.LocalPort, tuple.RemoteIP, tuple.RemotePort)
}

// KillByPeerAndPort kills all sockets matching peer IP and port.
func (m *NetlinkSocketManager) KillByPeerAndPort(peerIP string, port int) error {
	tuples, err := m.QueryEstablished(peerIP, port)
	if err != nil {
		return err
	}
	var lastErr error
	for _, t := range tuples {
		if err := m.KillSocket(t); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// BroadKill attempts to kill sockets using multiple address format variations.
func (m *NetlinkSocketManager) BroadKill(peerIP string, port string) {
	p := 0
	for _, c := range port {
		if c >= '0' && c <= '9' {
			p = p*10 + int(c-'0')
		}
	}
	if p == 0 {
		return
	}

	// Try the IP as given.
	_ = m.KillByPeerAndPort(peerIP, p)

	// Also try IPv4-mapped IPv6 variant and plain IPv4 variant.
	ip := net.ParseIP(peerIP)
	if ip == nil {
		return
	}
	if ip4 := ip.To4(); ip4 != nil {
		// Also try as ::ffff:x.x.x.x
		mapped := "::ffff:" + ip4.String()
		_ = m.KillByPeerAndPort(mapped, p)
	} else {
		// If it's an IPv6 that is actually a mapped v4, try the plain v4 too.
		if ip4 := ip.To4(); ip4 != nil {
			_ = m.KillByPeerAndPort(ip4.String(), p)
		}
	}
}

// dumpSockets sends an INET_DIAG dump request and collects all response messages.
// stateMask is a bitmask of TCP states. extMask requests extensions (e.g. INET_DIAG_INFO).
func (m *NetlinkSocketManager) dumpSockets(stateMask uint32, extMask uint32) ([][]byte, error) {
	var allMsgs [][]byte

	// Dump both AF_INET and AF_INET6.
	for _, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		msgs, err := m.dumpSocketsFamily(family, stateMask, extMask)
		if err != nil {
			// AF_INET6 may fail if IPv6 is disabled; ignore.
			if family == unix.AF_INET6 {
				continue
			}
			return nil, err
		}
		allMsgs = append(allMsgs, msgs...)
	}
	return allMsgs, nil
}

func (m *NetlinkSocketManager) dumpSocketsFamily(family uint8, stateMask, extMask uint32) ([][]byte, error) {
	// Try modern SOCK_DIAG_BY_FAMILY first, fall back to legacy TCPDIAG_GETSOCK.
	msgs, err := m.dumpSocketsFamilyWithType(family, stateMask, extMask, _SOCK_DIAG_BY_FAMILY)
	if err == nil {
		return msgs, nil
	}
	// Fallback to legacy type (pre-4.2 kernels).
	return m.dumpSocketsFamilyWithType(family, stateMask, extMask, _TCPDIAG_GETSOCK)
}

func (m *NetlinkSocketManager) dumpSocketsFamilyWithType(family uint8, stateMask, extMask uint32, msgType uint16) ([][]byte, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, fmt.Errorf("open netlink socket: %w", err)
	}
	defer unix.Close(fd)

	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("bind netlink: %w", err)
	}

	req := buildInetDiagReq(family, unix.IPPROTO_TCP, stateMask, extMask)

	nlmsgLen := uint32(_NLM_HDR_SIZE + len(req))
	hdr := make([]byte, _NLM_HDR_SIZE)
	binary.LittleEndian.PutUint32(hdr[0:4], nlmsgLen)                           // nlmsg_len
	binary.LittleEndian.PutUint16(hdr[4:6], msgType)                            // nlmsg_type
	binary.LittleEndian.PutUint16(hdr[6:8], unix.NLM_F_REQUEST|unix.NLM_F_DUMP) // nlmsg_flags
	binary.LittleEndian.PutUint32(hdr[8:12], 1)                                 // nlmsg_seq
	binary.LittleEndian.PutUint32(hdr[12:16], 0)                                // nlmsg_pid

	msg := append(hdr, req...)
	if _, err := unix.Write(fd, msg); err != nil {
		return nil, fmt.Errorf("write netlink: %w", err)
	}

	return recvNetlinkMsgs(fd)
}

// destroySocket sends a SOCK_DESTROY for the given 4-tuple.
func (m *NetlinkSocketManager) destroySocket(localIP string, localPort int, remoteIP string, remotePort int) error {
	family := uint8(unix.AF_INET)
	lip := net.ParseIP(localIP)
	rip := net.ParseIP(remoteIP)
	if lip == nil || rip == nil {
		return fmt.Errorf("invalid IP addresses: %s / %s", localIP, remoteIP)
	}
	if lip.To4() == nil || rip.To4() == nil {
		family = unix.AF_INET6
	}

	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return fmt.Errorf("open netlink socket: %w", err)
	}
	defer unix.Close(fd)

	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return fmt.Errorf("bind netlink: %w", err)
	}

	// Build inet_diag_req_v2 with exact 4-tuple.
	req := buildInetDiagReqExact(family, unix.IPPROTO_TCP, localIP, localPort, remoteIP, remotePort)

	nlmsgLen := uint32(_NLM_HDR_SIZE + len(req))
	hdr := make([]byte, _NLM_HDR_SIZE)
	binary.LittleEndian.PutUint32(hdr[0:4], nlmsgLen)
	binary.LittleEndian.PutUint16(hdr[4:6], _SOCK_DESTROY)
	binary.LittleEndian.PutUint16(hdr[6:8], unix.NLM_F_REQUEST|unix.NLM_F_ACK)
	binary.LittleEndian.PutUint32(hdr[8:12], 1)
	binary.LittleEndian.PutUint32(hdr[12:16], 0)

	msg := append(hdr, req...)
	if _, err := unix.Write(fd, msg); err != nil {
		return fmt.Errorf("write sock_destroy: %w", err)
	}

	// Read ACK.
	buf := make([]byte, 4096)
	n, err := unix.Read(fd, buf)
	if err != nil {
		return fmt.Errorf("read sock_destroy ack: %w", err)
	}
	if n >= _NLM_HDR_SIZE+4 {
		errCode := int32(binary.LittleEndian.Uint32(buf[_NLM_HDR_SIZE : _NLM_HDR_SIZE+4]))
		if errCode != 0 {
			return fmt.Errorf("sock_destroy error: %d", errCode)
		}
	}
	return nil
}

// -- inet_diag request building --

// buildInetDiagReq builds an inet_diag_req_v2 for a dump query.
// Layout (56 bytes):
//
//	u8  sdiag_family
//	u8  sdiag_protocol
//	u8  idiag_ext
//	u8  pad
//	u32 idiag_states
//	inet_diag_sockid (48 bytes, zeroed for dump)
func buildInetDiagReq(family uint8, proto uint8, states uint32, extMask uint32) []byte {
	req := make([]byte, _INET_DIAG_REQ_V2_SIZE)
	req[0] = family
	req[1] = proto
	req[2] = uint8(extMask)
	req[3] = 0 // pad
	binary.LittleEndian.PutUint32(req[4:8], states)
	// Remaining 48 bytes (inet_diag_sockid) are zero for dump.
	return req
}

// buildInetDiagReqExact builds an inet_diag_req_v2 with a specific sockid for SOCK_DESTROY.
// inet_diag_sockid layout (48 bytes):
//
//	__be16 sport
//	__be16 dport
//	__be32 src[4]  (16 bytes)
//	__be32 dst[4]  (16 bytes)
//	u32    if
//	u32    cookie[2]
func buildInetDiagReqExact(family uint8, proto uint8, localIP string, localPort int, remoteIP string, remotePort int) []byte {
	req := make([]byte, _INET_DIAG_REQ_V2_SIZE)
	req[0] = family
	req[1] = proto
	req[2] = 0
	req[3] = 0
	// states: match established
	binary.LittleEndian.PutUint32(req[4:8], 0xFFFFFFFF)

	// sockid starts at offset 8
	sid := req[8:]
	// sport, dport in big-endian (network byte order)
	binary.BigEndian.PutUint16(sid[0:2], uint16(localPort))
	binary.BigEndian.PutUint16(sid[2:4], uint16(remotePort))

	// src address (16 bytes starting at offset 4 in sockid)
	writeIPToSlice(sid[4:20], localIP, family)
	// dst address (16 bytes starting at offset 20 in sockid)
	writeIPToSlice(sid[20:36], remoteIP, family)

	// interface and cookie are zero (wildcard)
	return req
}

// writeIPToSlice writes an IP address to a 16-byte slice in network byte order.
func writeIPToSlice(dst []byte, ipStr string, family uint8) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return
	}
	if family == unix.AF_INET {
		ip4 := ip.To4()
		if ip4 != nil {
			copy(dst[0:4], ip4)
		}
	} else {
		ip16 := ip.To16()
		if ip16 != nil {
			copy(dst[0:16], ip16)
		}
	}
}

// -- Response parsing --

// diagMsgParsed holds parsed fields from an inet_diag_msg.
type diagMsgParsed struct {
	family     uint8
	state      uint8
	localIP    string
	localPort  int
	remoteIP   string
	remotePort int
}

// parseDiagMsg extracts socket info from a raw inet_diag_msg payload.
// inet_diag_msg layout:
//
//	u8  family
//	u8  state
//	u8  timer
//	u8  retrans
//	inet_diag_sockid id (48 bytes at offset 4)
//	u32 expires
//	u32 rqueue
//	u32 wqueue
//	u32 uid
//	u32 inode
func parseDiagMsg(data []byte) (diagMsgParsed, bool) {
	if len(data) < _INET_DIAG_MSG_SIZE {
		return diagMsgParsed{}, false
	}

	family := data[0]
	state := data[1]

	// sockid at offset 4
	sid := data[4:]
	sport := binary.BigEndian.Uint16(sid[0:2])
	dport := binary.BigEndian.Uint16(sid[2:4])

	var localIP, remoteIP string
	if family == unix.AF_INET {
		localIP = net.IP(sid[4:8]).String()
		remoteIP = net.IP(sid[20:24]).String()
	} else {
		localIP = net.IP(sid[4:20]).String()
		remoteIP = net.IP(sid[20:36]).String()
	}

	return diagMsgParsed{
		family:     family,
		state:      state,
		localIP:    localIP,
		localPort:  int(sport),
		remoteIP:   remoteIP,
		remotePort: int(dport),
	}, true
}

// extractTCPInfo finds the INET_DIAG_INFO netlink attribute in the response
// and returns the raw tcp_info bytes.
func extractTCPInfo(data []byte) []byte {
	if len(data) < _INET_DIAG_MSG_SIZE+4 {
		return nil
	}

	// Netlink attributes follow the inet_diag_msg.
	// Each attr: u16 nla_len, u16 nla_type, then payload.
	offset := _INET_DIAG_MSG_SIZE
	for offset+4 <= len(data) {
		nlaLen := binary.LittleEndian.Uint16(data[offset : offset+2])
		nlaType := binary.LittleEndian.Uint16(data[offset+2 : offset+4])
		if nlaLen < 4 {
			break
		}
		payloadLen := int(nlaLen) - 4
		if offset+4+payloadLen > len(data) {
			break
		}
		if nlaType == _INET_DIAG_INFO {
			return data[offset+4 : offset+4+payloadLen]
		}
		// Align to 4 bytes.
		offset += int((nlaLen + 3) & ^uint16(3))
	}
	return nil
}

// parseTCPInfoCounters extracts bytes_acked and bytes_received from raw tcp_info.
func parseTCPInfoCounters(ti []byte) (acked, received int64) {
	if len(ti) < _TCP_INFO_MIN_SIZE {
		return 0, 0
	}
	acked = int64(binary.LittleEndian.Uint64(ti[_TCP_INFO_BYTES_ACKED_OFF:]))
	received = int64(binary.LittleEndian.Uint64(ti[_TCP_INFO_BYTES_RECEIVED_OFF:]))
	return acked, received
}

// recvNetlinkMsgs reads all netlink response messages until NLMSG_DONE or error.
func recvNetlinkMsgs(fd int) ([][]byte, error) {
	var result [][]byte
	buf := make([]byte, 65536)

	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			return result, fmt.Errorf("read netlink: %w", err)
		}
		if n < _NLM_HDR_SIZE {
			break
		}

		offset := 0
		for offset+_NLM_HDR_SIZE <= n {
			nlmsgLen := binary.LittleEndian.Uint32(buf[offset : offset+4])
			nlmsgType := binary.LittleEndian.Uint16(buf[offset+4 : offset+6])

			if nlmsgLen < _NLM_HDR_SIZE || offset+int(nlmsgLen) > n {
				break
			}

			switch nlmsgType {
			case unix.NLMSG_DONE:
				return result, nil
			case unix.NLMSG_ERROR:
				if int(nlmsgLen) >= _NLM_HDR_SIZE+4 {
					errCode := int32(binary.LittleEndian.Uint32(buf[offset+_NLM_HDR_SIZE : offset+_NLM_HDR_SIZE+4]))
					if errCode != 0 {
						return result, fmt.Errorf("netlink error: %d", errCode)
					}
				}
				return result, nil
			default:
				// Payload is after the nlmsg header.
				payload := make([]byte, int(nlmsgLen)-_NLM_HDR_SIZE)
				copy(payload, buf[offset+_NLM_HDR_SIZE:offset+int(nlmsgLen)])
				result = append(result, payload)
			}

			// Align to 4 bytes.
			offset += int((nlmsgLen + 3) & ^uint32(3))
		}
	}
	return result, nil
}

// normalizeIP normalizes an IP string, converting IPv4-mapped IPv6 to plain IPv4.
func normalizeIP(s string) string {
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4.String()
	}
	return ip.String()
}

// Compile-time interface check.
var _ SocketManager = (*NetlinkSocketManager)(nil)
