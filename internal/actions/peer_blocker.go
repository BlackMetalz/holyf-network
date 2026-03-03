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

const ruleComment = "holyf-network-peer-block"

// BlockPeer inserts a DROP rule for peer IP -> local TCP port.
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
		"-j", "DROP",
	}

	if err := runCommand(bin, args...); err != nil {
		return fmt.Errorf("cannot block peer %s:%s: %w", peerIP, port, err)
	}

	return nil
}

// UnblockPeer removes a previously inserted DROP rule.
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
		"-j", "DROP",
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

	if _, err := exec.LookPath("conntrack"); err != nil {
		return nil
	}

	port := strconv.Itoa(spec.LocalPort)
	args := []string{
		"-D",
		"-p", "tcp",
		"-s", peerIP,
		"--dport", port,
	}
	if ip.To4() == nil {
		args = append(args, "-f", "ipv6")
	}

	out, err := exec.Command("conntrack", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		// Not fatal: nothing to delete.
		if strings.Contains(msg, "0 flow entries have been deleted") {
			return nil
		}
		return fmt.Errorf("conntrack delete failed: %s", strings.TrimSpace(msg))
	}

	return nil
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
