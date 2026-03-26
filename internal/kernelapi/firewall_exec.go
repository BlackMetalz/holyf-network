package kernelapi

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const fwRuleComment = "holyf-network-peer-block"

// ExecFirewall implements Firewall by shelling out to iptables/ip6tables.
func (e *ExecFirewall) BackendName() string { return "exec(iptables)" }

type ExecFirewall struct{}

// NewExecFirewall returns a new ExecFirewall.
func NewExecFirewall() *ExecFirewall {
	return &ExecFirewall{}
}

// ListBlockedPeers returns all peer blocks managed by holyf-network.
func (f *ExecFirewall) ListBlockedPeers() ([]PeerBlockSpec, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("peer blocking is only supported on Linux")
	}

	seen := make(map[string]PeerBlockSpec)
	var errs []string

	for _, bin := range []string{"iptables", "ip6tables"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}

		out, err := exec.Command(bin, "-S", "INPUT").CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			errs = append(errs, fmt.Sprintf("%s: %s", bin, msg))
			continue
		}

		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if !strings.Contains(line, fwRuleComment+":") {
				continue
			}

			fields := strings.Fields(line)
			peerRaw := fwArgValue(fields, "-s")
			portRaw := fwArgValue(fields, "--dport")
			if peerRaw == "" || portRaw == "" {
				continue
			}

			port, err := strconv.Atoi(portRaw)
			if err != nil || port < 1 || port > 65535 {
				continue
			}

			peerIP := fwNormalizeRuleIP(peerRaw)
			spec := PeerBlockSpec{PeerIP: peerIP, LocalPort: port}
			seen[fmt.Sprintf("%s|%d", spec.PeerIP, spec.LocalPort)] = spec
		}
	}

	if len(seen) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	blocks := make([]PeerBlockSpec, 0, len(seen))
	for _, spec := range seen {
		blocks = append(blocks, spec)
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].PeerIP != blocks[j].PeerIP {
			return blocks[i].PeerIP < blocks[j].PeerIP
		}
		return blocks[i].LocalPort < blocks[j].LocalPort
	})

	return blocks, nil
}

// BlockPeer inserts DROP rules for peer IP on the target local port.
func (f *ExecFirewall) BlockPeer(spec PeerBlockSpec) error {
	ip, peerIP, err := fwValidateSpec(spec)
	if err != nil {
		return err
	}

	bin, err := fwFirewallBinary(ip)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)
	comment := fwRuleCommentForPort(port)
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

	if err := fwRunCommand(bin, inputArgs...); err != nil {
		return fmt.Errorf("cannot block peer %s:%s: %w", peerIP, port, err)
	}
	if err := fwRunCommand(bin, outputArgs...); err != nil {
		_ = fwRunCommand(bin,
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

// UnblockPeer removes previously inserted block rules.
func (f *ExecFirewall) UnblockPeer(spec PeerBlockSpec) error {
	ip, peerIP, err := fwValidateSpec(spec)
	if err != nil {
		return err
	}

	bin, err := fwFirewallBinary(ip)
	if err != nil {
		return err
	}

	port := strconv.Itoa(spec.LocalPort)
	comment := fwRuleCommentForPort(port)
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
	if err := fwRunCommand(bin, inputArgs...); err != nil && !fwIsRuleMissingError(err) {
		errs = append(errs, err.Error())
	}
	if err := fwRunCommand(bin, outputArgs...); err != nil && !fwIsRuleMissingError(err) {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("cannot unblock peer %s:%s: %s", peerIP, port, strings.Join(errs, "; "))
	}

	return nil
}

// ---------------------------------------------------------------------------
// firewall helpers
// ---------------------------------------------------------------------------

func fwValidateSpec(spec PeerBlockSpec) (ip net.IP, normalized string, err error) {
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

func fwFirewallBinary(ip net.IP) (string, error) {
	bin := "iptables"
	if ip.To4() == nil {
		bin = "ip6tables"
	}

	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf("%s not found", bin)
	}

	return bin, nil
}

func fwRunCommand(name string, args ...string) error {
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

func fwIsRuleMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Bad rule") ||
		strings.Contains(msg, "No chain/target/match by that name")
}

func fwRuleCommentForPort(port string) string {
	return fwRuleComment + ":" + port
}

func fwArgValue(fields []string, key string) string {
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] == key {
			return strings.Trim(fields[i+1], "\"")
		}
	}
	return ""
}

func fwNormalizeRuleIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if ip, _, err := net.ParseCIDR(raw); err == nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ip.String()
	}

	ip := net.ParseIP(strings.TrimPrefix(raw, "::ffff:"))
	if ip == nil {
		return raw
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}
