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

// BlockPeer inserts a TCP reset REJECT rule for peer IP -> local TCP port.
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
	args := []string{
		"-I", "INPUT",
		"-s", peerIP,
		"-p", "tcp",
		"--dport", port,
		"-m", "comment",
		"--comment", ruleComment,
		"-j", "REJECT",
		"--reject-with", "tcp-reset",
	}

	if err := runCommand(bin, args...); err != nil {
		return fmt.Errorf("cannot block peer %s:%s: %w", peerIP, port, err)
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
	args := []string{
		"-D", "INPUT",
		"-s", peerIP,
		"-p", "tcp",
		"--dport", port,
		"-m", "comment",
		"--comment", ruleComment,
		"-j", "REJECT",
		"--reject-with", "tcp-reset",
	}

	if err := runCommand(bin, args...); err != nil {
		return fmt.Errorf("cannot unblock peer %s:%s: %w", peerIP, port, err)
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

	common := []string{"-D", "-p", "tcp"}
	if ip.To4() == nil {
		common = append(common, "-f", "ipv6")
	}
	commands := [][]string{
		append(append([]string{}, common...), "-s", peerIP, "--dport", port),
		append(append([]string{}, common...), "-d", peerIP, "--sport", port),
		append(append([]string{}, common...), "-d", peerIP, "--dport", port),
		append(append([]string{}, common...), "-s", peerIP, "--sport", port),
	}

	var errs []string
	for _, args := range commands {
		out, err := exec.Command("conntrack", args...).CombinedOutput()
		if err == nil {
			return nil
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
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("conntrack: %s", strings.Join(errs, " | "))
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
