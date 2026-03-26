package kernelapi

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

// ExecSocketManager implements SocketManager by shelling out to the ss command.
type ExecSocketManager struct{}

func (e *ExecSocketManager) BackendName() string { return "exec(ss)" }

// NewExecSocketManager returns a new ExecSocketManager.
func NewExecSocketManager() *ExecSocketManager {
	return &ExecSocketManager{}
}

// QueryEstablished returns established sockets matching peer IP and local port.
func (m *ExecSocketManager) QueryEstablished(peerIP string, localPort int) ([]SocketTuple, error) {
	if _, err := exec.LookPath("ss"); err != nil {
		return nil, fmt.Errorf("ss: command not found")
	}

	out, err := exec.Command("ss", "-tnp", "state", "established").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ss query: %w", err)
	}

	localPortStr := strconv.Itoa(localPort)
	normalizedPeer := strings.TrimPrefix(peerIP, "::ffff:")
	lines := strings.Split(string(out), "\n")
	seen := make(map[string]struct{})
	var tuples []SocketTuple

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Recv-Q") {
			continue
		}

		localAddr, remoteAddr, ok := execParseSSLine(line)
		if !ok {
			continue
		}

		localIP, localP, ok := execSplitHostPort(localAddr)
		if !ok || localP != localPortStr {
			continue
		}

		remoteIP, remoteP, ok := execSplitHostPort(remoteAddr)
		if !ok {
			continue
		}

		normalizedRemote := strings.TrimPrefix(remoteIP, "::ffff:")
		if normalizedRemote != normalizedPeer {
			continue
		}

		localPortInt, err := strconv.Atoi(localP)
		if err != nil {
			continue
		}
		remotePortInt, err := strconv.Atoi(remoteP)
		if err != nil {
			continue
		}

		key := fmt.Sprintf("%s|%d|%s|%d", localIP, localPortInt, remoteIP, remotePortInt)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		tuples = append(tuples, SocketTuple{
			LocalIP:    localIP,
			LocalPort:  localPortInt,
			RemoteIP:   remoteIP,
			RemotePort: remotePortInt,
		})
	}

	return tuples, nil
}

// QueryPeerSnapshot returns a snapshot of socket states for a peer+port pair.
func (m *ExecSocketManager) QueryPeerSnapshot(peerIP string, localPort int) (PeerSocketSnapshot, error) {
	if _, err := exec.LookPath("ss"); err != nil {
		return PeerSocketSnapshot{}, fmt.Errorf("ss: command not found")
	}

	out, err := exec.Command("ss", "-tnp").CombinedOutput()
	if err != nil {
		return PeerSocketSnapshot{}, fmt.Errorf("ss query: %w", err)
	}

	localPortStr := strconv.Itoa(localPort)
	normalizedPeer := strings.TrimPrefix(peerIP, "::ffff:")
	lines := strings.Split(string(out), "\n")

	exactSeen := make(map[string]struct{})
	activeSeen := make(map[string]struct{})
	timeWaitSeen := make(map[string]struct{})

	var snapshot PeerSocketSnapshot
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "State") {
			continue
		}

		state, localAddr, remoteAddr, ok := execParseSSStateLine(line)
		if !ok {
			continue
		}

		localIP, localP, ok := execSplitHostPort(localAddr)
		if !ok || localP != localPortStr {
			continue
		}
		remoteIP, remoteP, ok := execSplitHostPort(remoteAddr)
		if !ok {
			continue
		}
		if strings.TrimPrefix(remoteIP, "::ffff:") != normalizedPeer {
			continue
		}

		localPortInt, err := strconv.Atoi(localP)
		if err != nil {
			continue
		}
		remotePortInt, err := strconv.Atoi(remoteP)
		if err != nil {
			continue
		}

		tuple := SocketTuple{
			LocalIP:    localIP,
			LocalPort:  localPortInt,
			RemoteIP:   remoteIP,
			RemotePort: remotePortInt,
		}
		key := execSocketTupleKey(tuple)
		if _, exists := exactSeen[key]; !exists {
			exactSeen[key] = struct{}{}
			snapshot.ExactTuples = append(snapshot.ExactTuples, tuple)
		}

		switch {
		case state == "TIME_WAIT":
			if _, exists := timeWaitSeen[key]; exists {
				continue
			}
			timeWaitSeen[key] = struct{}{}
			snapshot.TimeWaitCount++
		case execIsKillActiveState(state):
			if _, exists := activeSeen[key]; exists {
				continue
			}
			activeSeen[key] = struct{}{}
			snapshot.ActiveCount++
		}
	}

	return snapshot, nil
}

// CollectTCPCounters returns per-socket byte counters for all TCP connections.
func (m *ExecSocketManager) CollectTCPCounters() ([]SocketCounter, error) {
	out, err := exec.Command("ss", "-tinHn").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ss tcp counters failed: %s", msg)
	}

	lines := strings.Split(string(out), "\n")
	counters := make([]SocketCounter, 0, len(lines)/2)

	var current FlowTuple
	haveTuple := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if tuple, ok := execParseSSStateLineTuple(line); ok {
			current = tuple
			haveTuple = true
			continue
		}

		if !haveTuple {
			continue
		}
		acked, hasAcked := execParseSSMetric(line, "bytes_acked:")
		if !hasAcked {
			acked, hasAcked = execParseSSMetric(line, "bytes_sent:")
		}
		recv, hasRecv := execParseSSMetric(line, "bytes_received:")
		if !hasAcked && !hasRecv {
			continue
		}
		if !hasAcked {
			acked = 0
		}
		if !hasRecv {
			recv = 0
		}

		counters = append(counters, SocketCounter{
			Tuple:         execNormalizeFlowTuple(current),
			BytesAcked:    acked,
			BytesReceived: recv,
		})
		haveTuple = false
	}

	return counters, nil
}

// KillSocket destroys a specific socket by exact 4-tuple.
func (m *ExecSocketManager) KillSocket(tuple SocketTuple) error {
	if _, err := exec.LookPath("ss"); err != nil {
		return fmt.Errorf("ss: command not found")
	}

	localIP, localV4, err := execNormalizeIPForSS(tuple.LocalIP)
	if err != nil {
		return fmt.Errorf("local ip: %w", err)
	}
	remoteIP, remoteV4, err := execNormalizeIPForSS(tuple.RemoteIP)
	if err != nil {
		return fmt.Errorf("remote ip: %w", err)
	}
	if localV4 != remoteV4 {
		return fmt.Errorf("ip family mismatch: %s <-> %s", localIP, remoteIP)
	}

	base := []string{}
	if localV4 {
		base = append(base, "-4")
	} else {
		base = append(base, "-6")
	}
	base = append(base, "-K")

	core := []string{
		"src", localIP,
		"sport", "=", ":" + strconv.Itoa(tuple.LocalPort),
		"dst", remoteIP,
		"dport", "=", ":" + strconv.Itoa(tuple.RemotePort),
	}

	candidates := [][]string{
		append([]string{"state", "established"}, core...),
		core,
	}

	var lastErr string
	for _, c := range candidates {
		args := append(append([]string{}, base...), c...)
		out, err := exec.Command("ss", args...).CombinedOutput()
		if err == nil {
			return nil
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		lastErr = msg
	}
	if lastErr == "" {
		lastErr = "unknown error"
	}

	return fmt.Errorf("%s:%d<->%s:%d (%s)", localIP, tuple.LocalPort, remoteIP, tuple.RemotePort, lastErr)
}

// KillByPeerAndPort kills all sockets matching peer IP and port.
func (m *ExecSocketManager) KillByPeerAndPort(peerIP string, port int) error {
	if _, err := exec.LookPath("ss"); err != nil {
		return fmt.Errorf("ss: command not found")
	}

	rawIP := strings.TrimPrefix(strings.TrimSpace(peerIP), "::ffff:")
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return fmt.Errorf("invalid peer IP: %s", peerIP)
	}

	base := []string{}
	if ip.To4() != nil {
		base = append(base, "-4")
	} else {
		base = append(base, "-6")
	}
	base = append(base, "-K")

	portStr := strconv.Itoa(port)
	candidates := [][]string{
		{"dst", peerIP, "sport", "=", ":" + portStr},
		{"src", peerIP, "dport", "=", ":" + portStr},
		{"dst", peerIP, "dport", "=", ":" + portStr},
		{"src", peerIP, "sport", "=", ":" + portStr},
	}
	statePrefixes := [][]string{
		{"state", "established"},
		{},
	}

	var lastErr string
	for _, prefix := range statePrefixes {
		for _, c := range candidates {
			args := append(append(append([]string{}, base...), prefix...), c...)
			out, err := exec.Command("ss", args...).CombinedOutput()
			if err == nil {
				return nil
			}
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			lastErr = msg
		}
	}

	if lastErr == "" {
		lastErr = "unknown error"
	}
	return fmt.Errorf("ss: %s", lastErr)
}

// BroadKill attempts to kill sockets using multiple address format combinations.
func (m *ExecSocketManager) BroadKill(peerIP string, port string) {
	if _, err := exec.LookPath("ss"); err != nil {
		return
	}

	stripped := strings.TrimPrefix(peerIP, "::ffff:")
	mapped := "::ffff:" + stripped

	type combo struct {
		family string
		addr   string
	}
	combos := []combo{
		{"-4", stripped},
		{"-6", mapped},
		{"", stripped},
		{"", mapped},
	}

	for _, c := range combos {
		for _, filter := range [][]string{
			{"dst", c.addr, "sport", "=", ":" + port},
			{"src", c.addr, "dport", "=", ":" + port},
		} {
			args := []string{}
			if c.family != "" {
				args = append(args, c.family)
			}
			args = append(args, "-K")
			args = append(args, filter...)
			exec.Command("ss", args...).Run() //nolint:errcheck
		}
	}
}

// ---------------------------------------------------------------------------
// ss output parsing helpers
// ---------------------------------------------------------------------------

func execParseSSLine(line string) (localAddr, remoteAddr string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return "", "", false
	}
	return fields[2], fields[3], true
}

func execSplitHostPort(addrPort string) (host, port string, ok bool) {
	if strings.HasPrefix(addrPort, "[") {
		idx := strings.LastIndex(addrPort, "]:")
		if idx < 0 {
			return "", "", false
		}
		return addrPort[1:idx], addrPort[idx+2:], true
	}

	idx := strings.LastIndex(addrPort, ":")
	if idx < 0 {
		return "", "", false
	}
	return addrPort[:idx], addrPort[idx+1:], true
}

func execParseSSStateLine(line string) (state, localAddr, remoteAddr string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return "", "", "", false
	}
	return execNormalizeSSState(fields[0]), fields[3], fields[4], true
}

func execNormalizeSSState(raw string) string {
	state := strings.ToUpper(strings.TrimSpace(raw))
	state = strings.ReplaceAll(state, "-", "_")
	switch state {
	case "ESTAB":
		return "ESTABLISHED"
	case "SYNRECV":
		return "SYN_RECV"
	case "TIMEWAIT":
		return "TIME_WAIT"
	default:
		return state
	}
}

func execIsKillActiveState(state string) bool {
	state = strings.TrimSpace(state)
	return state != "" && state != "TIME_WAIT"
}

func execSocketTupleKey(tuple SocketTuple) string {
	return fmt.Sprintf("%s|%d|%s|%d", tuple.LocalIP, tuple.LocalPort, tuple.RemoteIP, tuple.RemotePort)
}

func execNormalizeIPForSS(raw string) (string, bool, error) {
	ipStr := strings.TrimSpace(raw)

	hasMappedPrefix := strings.HasPrefix(ipStr, "::ffff:")

	stripped := strings.TrimPrefix(ipStr, "::ffff:")
	ip := net.ParseIP(stripped)
	if ip == nil {
		ip = net.ParseIP(ipStr)
		if ip == nil {
			return "", false, fmt.Errorf("invalid ip: %s", raw)
		}
	}

	if v4 := ip.To4(); v4 != nil {
		if hasMappedPrefix {
			return "::ffff:" + v4.String(), false, nil
		}
		return v4.String(), true, nil
	}
	return ip.String(), false, nil
}

// ---------------------------------------------------------------------------
// ss -tinHn parsing helpers (for CollectTCPCounters)
// ---------------------------------------------------------------------------

func execParseSSStateLineTuple(line string) (FlowTuple, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return FlowTuple{}, false
	}
	if !execLooksLikeSSStateToken(fields[0]) {
		return FlowTuple{}, false
	}
	local := fields[3]
	peer := fields[4]

	localIP, localPort, ok := execSplitAddrPort(local)
	if !ok {
		return FlowTuple{}, false
	}
	peerIP, peerPort, ok := execSplitAddrPort(peer)
	if !ok {
		return FlowTuple{}, false
	}

	return FlowTuple{
		SrcIP:   localIP,
		SrcPort: localPort,
		DstIP:   peerIP,
		DstPort: peerPort,
	}, true
}

func execLooksLikeSSStateToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if unicode.IsUpper(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func execSplitAddrPort(addr string) (string, int, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0, false
	}

	host := ""
	portStr := ""

	if strings.HasPrefix(addr, "[") {
		idx := strings.LastIndex(addr, "]:")
		if idx < 0 {
			return "", 0, false
		}
		host = addr[1:idx]
		portStr = addr[idx+2:]
	} else {
		idx := strings.LastIndex(addr, ":")
		if idx < 0 {
			return "", 0, false
		}
		host = addr[:idx]
		portStr = addr[idx+1:]
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	return host, port, true
}

func execParseSSMetric(line, key string) (int64, bool) {
	idx := strings.Index(line, key)
	if idx < 0 {
		return 0, false
	}
	start := idx + len(key)
	end := start
	for end < len(line) && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	if end == start {
		return 0, false
	}
	v, err := strconv.ParseInt(line[start:end], 10, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func execNormalizeFlowTuple(t FlowTuple) FlowTuple {
	return FlowTuple{
		SrcIP:   execNormalizeFlowIP(t.SrcIP),
		SrcPort: t.SrcPort,
		DstIP:   execNormalizeFlowIP(t.DstIP),
		DstPort: t.DstPort,
	}
}

func execNormalizeFlowIP(ip string) string {
	return strings.TrimPrefix(strings.TrimSpace(ip), "::ffff:")
}
