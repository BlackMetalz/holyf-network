package collector

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// FlowTuple identifies one directional TCP flow tuple.
type FlowTuple struct {
	SrcIP   string
	SrcPort int
	DstIP   string
	DstPort int
}

// ConntrackFlow represents one bidirectional conntrack TCP flow with counters.
type ConntrackFlow struct {
	Orig       FlowTuple
	Reply      FlowTuple
	OrigBytes  int64
	ReplyBytes int64
}

// CollectConntrackFlowsTCP reads TCP flow counters from conntrack.
// It requires conntrack-tools and typically root/sudo privileges.
func CollectConntrackFlowsTCP() ([]ConntrackFlow, error) {
	out, err := exec.Command("conntrack", "-L", "-p", "tcp", "-o", "extended", "-n").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("conntrack flow dump failed: %s", msg)
	}

	lines := strings.Split(string(out), "\n")
	flows := make([]ConntrackFlow, 0, len(lines))
	candidateLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Ignore non-flow informational lines from conntrack output.
		if !looksLikeConntrackFlowLine(line) {
			continue
		}
		candidateLines++
		flow, ok := parseConntrackFlowLine(line)
		if !ok {
			continue
		}
		flows = append(flows, flow)
	}
	if candidateLines > 0 && len(flows) == 0 {
		if enabled, ok := conntrackAccountingEnabled(); ok && !enabled {
			return nil, fmt.Errorf("conntrack bytes accounting disabled (set net.netfilter.nf_conntrack_acct=1)")
		}
		return nil, fmt.Errorf("conntrack flow parse failed: no valid TCP flow rows decoded")
	}
	if len(flows) > 0 {
		allZero := true
		for _, flow := range flows {
			if flow.OrigBytes > 0 || flow.ReplyBytes > 0 {
				allZero = false
				break
			}
		}
		if allZero {
			if enabled, ok := conntrackAccountingEnabled(); ok && !enabled {
				return nil, fmt.Errorf("conntrack bytes accounting disabled (set net.netfilter.nf_conntrack_acct=1)")
			}
		}
	}

	// Empty result can be legitimate: no current TCP conntrack entries.
	return flows, nil
}

func looksLikeConntrackFlowLine(line string) bool {
	return strings.Contains(line, "src=") &&
		strings.Contains(line, "dst=") &&
		strings.Contains(line, "sport=") &&
		strings.Contains(line, "dport=")
}

func conntrackAccountingEnabled() (enabled bool, known bool) {
	data, err := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct")
	if err != nil {
		return false, false
	}
	raw := strings.TrimSpace(string(data))
	switch raw {
	case "1":
		return true, true
	case "0":
		return false, true
	default:
		return false, false
	}
}

func parseConntrackFlowLine(line string) (ConntrackFlow, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ConntrackFlow{}, false
	}

	srcVals := make([]string, 0, 2)
	dstVals := make([]string, 0, 2)
	sportVals := make([]int, 0, 2)
	dportVals := make([]int, 0, 2)
	byteVals := make([]int64, 0, 2)

	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]
		switch key {
		case "src":
			srcVals = append(srcVals, value)
		case "dst":
			dstVals = append(dstVals, value)
		case "sport":
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				return ConntrackFlow{}, false
			}
			sportVals = append(sportVals, port)
		case "dport":
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				return ConntrackFlow{}, false
			}
			dportVals = append(dportVals, port)
		case "bytes":
			bytes, err := strconv.ParseInt(value, 10, 64)
			if err != nil || bytes < 0 {
				return ConntrackFlow{}, false
			}
			byteVals = append(byteVals, bytes)
		}
	}

	if len(srcVals) < 2 || len(dstVals) < 2 || len(sportVals) < 2 || len(dportVals) < 2 || len(byteVals) < 2 {
		return ConntrackFlow{}, false
	}

	return ConntrackFlow{
		Orig: FlowTuple{
			SrcIP:   srcVals[0],
			SrcPort: sportVals[0],
			DstIP:   dstVals[0],
			DstPort: dportVals[0],
		},
		Reply: FlowTuple{
			SrcIP:   srcVals[1],
			SrcPort: sportVals[1],
			DstIP:   dstVals[1],
			DstPort: dportVals[1],
		},
		OrigBytes:  byteVals[0],
		ReplyBytes: byteVals[1],
	}, true
}
