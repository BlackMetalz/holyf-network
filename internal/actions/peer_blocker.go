package actions

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
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

const ruleComment = "holyf-network-peer-block"

// BlockPeer inserts INPUT+OUTPUT DROP rules for peer IP on the target local port.
func BlockPeer(spec PeerBlockSpec) error {
	ip, peerIP, err := validateSpec(spec)
	if err != nil {
		return err
	}

	bin, err := firewallBinary(ip)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)
	comment := ruleCommentForPort(port)
	inputArgs := []string{
		"-I", "INPUT",
		"-s", peerIP,
		"-p", "tcp",
		"--dport", port,
		"-m", "comment",
		"--comment", comment,
		"-j", "DROP",
	}
	outputArgs := []string{
		"-I", "OUTPUT",
		"-d", peerIP,
		"-p", "tcp",
		"--sport", port,
		"-m", "comment",
		"--comment", comment,
		"-j", "DROP",
	}

	if err := runCommand(bin, inputArgs...); err != nil {
		return fmt.Errorf("cannot block peer %s:%s: %w", peerIP, port, err)
	}
	if err := runCommand(bin, outputArgs...); err != nil {
		_ = runCommand(bin,
			"-D", "INPUT",
			"-s", peerIP,
			"-p", "tcp",
			"--dport", port,
			"-m", "comment",
			"--comment", comment,
			"-j", "DROP",
		)
		return fmt.Errorf("cannot block peer %s:%s output path: %w", peerIP, port, err)
	}

	return nil
}

// UnblockPeer removes a previously inserted block rule.
func UnblockPeer(spec PeerBlockSpec) error {
	ip, peerIP, err := validateSpec(spec)
	if err != nil {
		return err
	}

	bin, err := firewallBinary(ip)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)
	comment := ruleCommentForPort(port)
	inputArgs := []string{
		"-D", "INPUT",
		"-s", peerIP,
		"-p", "tcp",
		"--dport", port,
		"-m", "comment",
		"--comment", comment,
		"-j", "DROP",
	}
	outputArgs := []string{
		"-D", "OUTPUT",
		"-d", peerIP,
		"-p", "tcp",
		"--sport", port,
		"-m", "comment",
		"--comment", comment,
		"-j", "DROP",
	}

	var errs []string
	if err := runCommand(bin, inputArgs...); err != nil && !isRuleMissingError(err) {
		errs = append(errs, err.Error())
	}
	if err := runCommand(bin, outputArgs...); err != nil && !isRuleMissingError(err) {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("cannot unblock peer %s:%s: %s", peerIP, port, strings.Join(errs, "; "))
	}

	return nil
}

// DropPeerConnections removes matching conntrack entries (best effort).
func DropPeerConnections(spec PeerBlockSpec) error {
	ip, peerIP, err := validateSpec(spec)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)
	ssErr := killSocketByPeerAndPort(ip, peerIP, port)
	ctErr := deleteConntrackFlows(ip, peerIP, port)

	// At least one mechanism succeeded.
	if ssErr == nil || ctErr == nil {
		return nil
	}

	return fmt.Errorf("%s; %s", ssErr.Error(), ctErr.Error())
}

// QueryAndKillPeerSockets queries ss in real-time for established connections
// matching the peer IP and local port, then kills each one with ss -K.
// This is more reliable than using a cached snapshot because it captures
// connections that were established after the last TUI refresh.
func QueryAndKillPeerSockets(spec PeerBlockSpec) error {
	ip, peerIP, err := validateSpec(spec)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)

	// Query established connections in real-time.
	tuples, queryErr := queryEstablishedSockets(ip, peerIP, port)
	if queryErr != nil {
		// Fallback: try the broad kill approach.
		return killSocketByPeerAndPort(ip, peerIP, port)
	}

	if len(tuples) == 0 {
		// No matching sockets found — nothing to kill.
		return nil
	}

	// Kill each discovered socket.
	return KillSockets(tuples)
}

// queryEstablishedSockets runs `ss -tnp state established` and parses the
// output to find connections matching peerIP and localPort.
//
// ss output format (header + data lines):
//
//	Recv-Q  Send-Q  Local Address:Port  Peer Address:Port  Process
//	0       0       10.0.0.1:8080       10.0.0.2:54321     users:(("nginx",pid=1234,fd=5))
func queryEstablishedSockets(ip net.IP, peerIP, localPort string) ([]SocketTuple, error) {
	if _, err := exec.LookPath("ss"); err != nil {
		return nil, fmt.Errorf("ss: command not found")
	}

	family := "-4"
	if ip.To4() == nil {
		family = "-6"
	}

	out, err := exec.Command("ss", family, "-tnp", "state", "established").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ss query: %w", err)
	}

	normalizedPeer := strings.TrimPrefix(peerIP, "::ffff:")
	lines := strings.Split(string(out), "\n")
	seen := make(map[string]struct{})
	var tuples []SocketTuple

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Recv-Q") {
			continue
		}

		localAddr, remoteAddr, ok := parseSSLine(line)
		if !ok {
			continue
		}

		localIP, localP, ok := splitHostPort(localAddr)
		if !ok || localP != localPort {
			continue
		}

		remoteIP, remoteP, ok := splitHostPort(remoteAddr)
		if !ok {
			continue
		}

		// Normalize and compare peer IP.
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

// parseSSLine extracts local and remote address fields from a single ss output line.
// Expected format: "Recv-Q Send-Q Local:Port Peer:Port [Process]"
// Fields are whitespace-separated; we need fields at index 2 and 3
// (0=RecvQ, 1=SendQ, 2=Local, 3=Peer).
func parseSSLine(line string) (localAddr, remoteAddr string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return "", "", false
	}
	return fields[2], fields[3], true
}

// splitHostPort splits "addr:port" handling IPv6 bracket notation.
// Examples: "10.0.0.1:8080" -> ("10.0.0.1", "8080")
//
//	"[::1]:8080"    -> ("::1", "8080")
func splitHostPort(addrPort string) (host, port string, ok bool) {
	// IPv6 bracket notation: [::1]:port
	if strings.HasPrefix(addrPort, "[") {
		idx := strings.LastIndex(addrPort, "]:")
		if idx < 0 {
			return "", "", false
		}
		return addrPort[1:idx], addrPort[idx+2:], true
	}

	// IPv4 or bare IPv6: last colon separates host and port.
	idx := strings.LastIndex(addrPort, ":")
	if idx < 0 {
		return "", "", false
	}
	return addrPort[:idx], addrPort[idx+1:], true
}

// KillSockets tries to close exact established sockets with ss -K.
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
	if _, err := exec.LookPath("ss"); err != nil {
		return fmt.Errorf("ss: command not found")
	}

	localIP, localV4, err := normalizeIPForSS(tuple.LocalIP)
	if err != nil {
		return fmt.Errorf("local ip: %w", err)
	}
	remoteIP, remoteV4, err := normalizeIPForSS(tuple.RemoteIP)
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

func killSocketByPeerAndPort(ip net.IP, peerIP, port string) error {
	if _, err := exec.LookPath("ss"); err != nil {
		return fmt.Errorf("ss: command not found")
	}

	base := []string{}
	if ip.To4() != nil {
		base = append(base, "-4")
	} else {
		base = append(base, "-6")
	}
	base = append(base, "-K")

	// Try multiple tuple directions. iproute2 filter semantics differ across versions.
	candidates := [][]string{
		{"dst", peerIP, "sport", "=", ":" + port},
		{"src", peerIP, "dport", "=", ":" + port},
		{"dst", peerIP, "dport", "=", ":" + port},
		{"src", peerIP, "sport", "=", ":" + port},
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

func deleteConntrackFlows(ip net.IP, peerIP, port string) error {
	if _, err := exec.LookPath("conntrack"); err != nil {
		return fmt.Errorf("conntrack: command not found")
	}

	commonTCP := []string{"-D", "-p", "tcp"}
	if ip.To4() == nil {
		commonTCP = append(commonTCP, "-f", "ipv6")
	}
	commands := [][]string{
		append(append([]string{}, commonTCP...), "-s", peerIP, "--dport", port),
		append(append([]string{}, commonTCP...), "-d", peerIP, "--sport", port),
		append(append([]string{}, commonTCP...), "-d", peerIP, "--dport", port),
		append(append([]string{}, commonTCP...), "-s", peerIP, "--sport", port),
	}

	var errs []string
	deleted := false
	for _, args := range commands {
		out, err := exec.Command("conntrack", args...).CombinedOutput()
		if err == nil {
			deleted = true
			continue
		}
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "0 flow entries have been deleted") {
			continue
		}
		if msg == "" {
			msg = err.Error()
		}
		errs = append(errs, msg)
	}
	if deleted || len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("conntrack: %s", strings.Join(errs, " | "))
}

func isRuleMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Bad rule") ||
		strings.Contains(msg, "No chain/target/match by that name")
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

func firewallBinary(ip net.IP) (string, error) {
	bin := "iptables"
	if ip.To4() == nil {
		bin = "ip6tables"
	}

	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf("%s not found", bin)
	}

	return bin, nil
}

func runCommand(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s %s", name, msg)
	}
	return nil
}

func normalizeIPForSS(raw string) (string, bool, error) {
	ipStr := strings.TrimSpace(raw)
	ipStr = strings.TrimPrefix(ipStr, "::ffff:")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", false, fmt.Errorf("invalid ip: %s", raw)
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), true, nil
	}
	return ip.String(), false, nil
}

func ruleCommentForPort(port string) string {
	return ruleComment + ":" + port
}
