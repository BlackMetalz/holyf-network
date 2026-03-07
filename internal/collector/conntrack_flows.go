package collector

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
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
	State      string
	Orig       FlowTuple
	Reply      FlowTuple
	OrigBytes  int64
	ReplyBytes int64
}

// CollectConntrackFlowsTCP reads TCP flow counters from conntrack.
// It requires conntrack-tools and typically root/sudo privileges.
func CollectConntrackFlowsTCP() ([]ConntrackFlow, error) {
	var (
		candidateLines int
		successfulDump bool
		lastError      error
	)
	merged := make(map[string]ConntrackFlow)
	dumps := [][]string{
		{"-L", "-p", "tcp", "-o", "extended", "-n"},
		{"-L", "-p", "tcp"},
	}
	for _, args := range dumps {
		out, err := exec.Command("conntrack", args...).CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			lastError = fmt.Errorf("conntrack flow dump failed: %s", msg)
			continue
		}

		successfulDump = true
		parsed, candidates := parseConntrackFlowsOutput(string(out))
		candidateLines += candidates
		if len(parsed) == 0 {
			if candidates > 0 {
				// Candidate rows existed but none parsed: keep trying other formats.
				lastError = fmt.Errorf("conntrack flow parse failed: no valid TCP flow rows decoded")
			}
			continue
		}
		mergeConntrackFlowSet(merged, parsed)
	}

	flows := conntrackFlowMapToSortedSlice(merged)
	if len(flows) == 0 {
		if !successfulDump && lastError != nil {
			return nil, lastError
		}
		if candidateLines > 0 {
			if enabled, ok := conntrackAccountingEnabled(); ok && !enabled {
				return nil, fmt.Errorf("conntrack bytes accounting disabled (set net.netfilter.nf_conntrack_acct=1)")
			}
			if lastError != nil {
				return nil, lastError
			}
			return nil, fmt.Errorf("conntrack flow parse failed: no valid TCP flow rows decoded")
		}
		// Empty result can be legitimate: no current TCP conntrack entries.
		return nil, nil
	}

	allZero := true
	for _, flow := range flows {
		if flow.OrigBytes > 0 || flow.ReplyBytes > 0 {
			allZero = false
			break
		}
	}
	if allZero {
		if enabled, ok := conntrackAccountingEnabled(); ok && !enabled {
			return flows, fmt.Errorf("conntrack bytes accounting disabled (set net.netfilter.nf_conntrack_acct=1)")
		}
	}

	return flows, nil
}

func mergeConntrackFlowSet(dst map[string]ConntrackFlow, flows []ConntrackFlow) {
	for _, flow := range flows {
		key := conntrackFlowCanonicalKey(flow)
		existing, exists := dst[key]
		if !exists || shouldPreferConntrackFlow(existing, flow) {
			dst[key] = flow
		}
	}
}

func conntrackFlowMapToSortedSlice(rows map[string]ConntrackFlow) []ConntrackFlow {
	if len(rows) == 0 {
		return nil
	}
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ConntrackFlow, 0, len(keys))
	for _, key := range keys {
		out = append(out, rows[key])
	}
	return out
}

func conntrackFlowCanonicalKey(flow ConntrackFlow) string {
	orig := normalizeFlowTuple(flow.Orig)
	reply := normalizeFlowTuple(flow.Reply)
	left := conntrackFlowTupleKey(orig)
	right := conntrackFlowTupleKey(reply)
	if left <= right {
		return left + "|" + right
	}
	return right + "|" + left
}

func conntrackFlowTupleKey(t FlowTuple) string {
	return fmt.Sprintf("%s:%d>%s:%d", normalizeFlowIP(t.SrcIP), t.SrcPort, normalizeFlowIP(t.DstIP), t.DstPort)
}

func shouldPreferConntrackFlow(current, candidate ConntrackFlow) bool {
	currentNonZero := countNonZeroByteFields(current)
	candidateNonZero := countNonZeroByteFields(candidate)
	if candidateNonZero != currentNonZero {
		return candidateNonZero > currentNonZero
	}

	currentTotal := current.OrigBytes + current.ReplyBytes
	candidateTotal := candidate.OrigBytes + candidate.ReplyBytes
	if candidateTotal != currentTotal {
		return candidateTotal > currentTotal
	}

	currentState := strings.ToUpper(strings.TrimSpace(current.State))
	candidateState := strings.ToUpper(strings.TrimSpace(candidate.State))
	if currentState != "ESTABLISHED" && candidateState == "ESTABLISHED" {
		return true
	}

	return false
}

func countNonZeroByteFields(flow ConntrackFlow) int {
	count := 0
	if flow.OrigBytes > 0 {
		count++
	}
	if flow.ReplyBytes > 0 {
		count++
	}
	return count
}

func parseConntrackFlowsOutput(raw string) ([]ConntrackFlow, int) {
	lines := strings.Split(raw, "\n")
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
	return flows, candidateLines
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
			port, ok := parseConntrackPort(value)
			if !ok {
				return ConntrackFlow{}, false
			}
			sportVals = append(sportVals, port)
		case "dport":
			port, ok := parseConntrackPort(value)
			if !ok {
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

	if len(srcVals) < 2 || len(dstVals) < 2 || len(sportVals) < 2 || len(dportVals) < 2 {
		return ConntrackFlow{}, false
	}

	state := "UNKNOWN"
	if len(fields) > 3 && !strings.Contains(fields[3], "=") {
		state = strings.TrimSpace(fields[3])
	}

	origBytes := int64(0)
	replyBytes := int64(0)
	if len(byteVals) >= 1 {
		origBytes = byteVals[0]
	}
	if len(byteVals) >= 2 {
		replyBytes = byteVals[1]
	}

	return ConntrackFlow{
		State: state,
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
		OrigBytes:  origBytes,
		ReplyBytes: replyBytes,
	}, true
}

func parseConntrackPort(raw string) (int, bool) {
	port, err := strconv.Atoi(raw)
	if err == nil && port >= 1 && port <= 65535 {
		return port, true
	}
	lookup, err := net.LookupPort("tcp", raw)
	if err != nil || lookup < 1 || lookup > 65535 {
		return 0, false
	}
	return lookup, true
}
