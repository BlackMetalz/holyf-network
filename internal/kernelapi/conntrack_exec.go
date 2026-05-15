package kernelapi

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// ExecConntrackManager implements ConntrackManager by shelling out to conntrack.
type ExecConntrackManager struct{}

func (e *ExecConntrackManager) BackendName() string { return "exec(conntrack)" }

// NewExecConntrackManager returns a new ExecConntrackManager.
func NewExecConntrackManager() *ExecConntrackManager {
	return &ExecConntrackManager{}
}

// ReadStats returns cumulative insert and drop counters from `conntrack -S`.
func (m *ExecConntrackManager) ReadStats() (inserts, drops int64, ok bool) {
	out, err := exec.Command("conntrack", "-S").Output()
	if err != nil {
		return 0, 0, false
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		for _, field := range fields {
			parts := strings.SplitN(field, "=", 2)
			if len(parts) != 2 {
				continue
			}

			val, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				continue
			}

			switch parts[0] {
			case "insert":
				inserts += val
			case "drop":
				drops += val
			}
		}
	}

	return inserts, drops, true
}

// CollectFlowsTCP returns all current TCP conntrack flows with byte counters.
func (m *ExecConntrackManager) CollectFlowsTCP() ([]ConntrackFlow, error) {
	if _, err := exec.LookPath("conntrack"); err != nil {
		return nil, nil // conntrack tool not installed — graceful empty
	}

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
		parsed, candidates := ctParseConntrackFlowsOutput(string(out))
		candidateLines += candidates
		if len(parsed) == 0 {
			if candidates > 0 {
				lastError = fmt.Errorf("conntrack flow parse failed: no valid TCP flow rows decoded")
			}
			continue
		}
		ctMergeConntrackFlowSet(merged, parsed)
	}

	flows := ctConntrackFlowMapToSortedSlice(merged)
	if len(flows) == 0 {
		if !successfulDump && lastError != nil {
			return nil, lastError
		}
		if candidateLines > 0 {
			if enabled, ok := ctConntrackAccountingEnabled(); ok && !enabled {
				return nil, fmt.Errorf("conntrack bytes accounting disabled (set net.netfilter.nf_conntrack_acct=1)")
			}
			if lastError != nil {
				return nil, lastError
			}
			return nil, fmt.Errorf("conntrack flow parse failed: no valid TCP flow rows decoded")
		}
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
		if enabled, ok := ctConntrackAccountingEnabled(); ok && !enabled {
			return flows, fmt.Errorf("conntrack bytes accounting disabled (set net.netfilter.nf_conntrack_acct=1)")
		}
	}

	return flows, nil
}

// DeleteFlows removes conntrack entries matching peer IP and port.
func (m *ExecConntrackManager) DeleteFlows(peerIP string, port int) error {
	if _, err := exec.LookPath("conntrack"); err != nil {
		return fmt.Errorf("conntrack: command not found")
	}

	rawIP := strings.TrimPrefix(strings.TrimSpace(peerIP), "::ffff:")
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return fmt.Errorf("invalid peer IP: %s", peerIP)
	}

	portStr := strconv.Itoa(port)
	commonTCP := []string{"-D", "-p", "tcp"}
	if ip.To4() == nil {
		commonTCP = append(commonTCP, "-f", "ipv6")
	}
	commands := [][]string{
		append(append([]string{}, commonTCP...), "-s", peerIP, "--dport", portStr),
		append(append([]string{}, commonTCP...), "-d", peerIP, "--sport", portStr),
		append(append([]string{}, commonTCP...), "-d", peerIP, "--dport", portStr),
		append(append([]string{}, commonTCP...), "-s", peerIP, "--sport", portStr),
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

// ---------------------------------------------------------------------------
// conntrack flow parsing helpers
// ---------------------------------------------------------------------------

func ctParseConntrackFlowsOutput(raw string) ([]ConntrackFlow, int) {
	lines := strings.Split(raw, "\n")
	flows := make([]ConntrackFlow, 0, len(lines))
	candidateLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !ctLooksLikeConntrackFlowLine(line) {
			continue
		}
		candidateLines++
		flow, ok := ctParseConntrackFlowLine(line)
		if !ok {
			continue
		}
		flows = append(flows, flow)
	}
	return flows, candidateLines
}

func ctLooksLikeConntrackFlowLine(line string) bool {
	return strings.Contains(line, "src=") &&
		strings.Contains(line, "dst=") &&
		strings.Contains(line, "sport=") &&
		strings.Contains(line, "dport=")
}

func ctParseConntrackFlowLine(line string) (ConntrackFlow, bool) {
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
			port, ok := ctParseConntrackPort(value)
			if !ok {
				return ConntrackFlow{}, false
			}
			sportVals = append(sportVals, port)
		case "dport":
			port, ok := ctParseConntrackPort(value)
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

func ctParseConntrackPort(raw string) (int, bool) {
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

func ctMergeConntrackFlowSet(dst map[string]ConntrackFlow, flows []ConntrackFlow) {
	for _, flow := range flows {
		key := ctConntrackFlowCanonicalKey(flow)
		existing, exists := dst[key]
		if !exists || ctShouldPreferConntrackFlow(existing, flow) {
			dst[key] = flow
		}
	}
}

func ctConntrackFlowMapToSortedSlice(rows map[string]ConntrackFlow) []ConntrackFlow {
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

func ctConntrackFlowCanonicalKey(flow ConntrackFlow) string {
	orig := ctNormalizeFlowTuple(flow.Orig)
	reply := ctNormalizeFlowTuple(flow.Reply)
	left := ctConntrackFlowTupleKey(orig)
	right := ctConntrackFlowTupleKey(reply)
	if left <= right {
		return left + "|" + right
	}
	return right + "|" + left
}

func ctConntrackFlowTupleKey(t FlowTuple) string {
	return fmt.Sprintf("%s:%d>%s:%d", ctNormalizeFlowIP(t.SrcIP), t.SrcPort, ctNormalizeFlowIP(t.DstIP), t.DstPort)
}

func ctShouldPreferConntrackFlow(current, candidate ConntrackFlow) bool {
	currentNonZero := ctCountNonZeroByteFields(current)
	candidateNonZero := ctCountNonZeroByteFields(candidate)
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

func ctCountNonZeroByteFields(flow ConntrackFlow) int {
	count := 0
	if flow.OrigBytes > 0 {
		count++
	}
	if flow.ReplyBytes > 0 {
		count++
	}
	return count
}

func ctNormalizeFlowTuple(t FlowTuple) FlowTuple {
	return FlowTuple{
		SrcIP:   ctNormalizeFlowIP(t.SrcIP),
		SrcPort: t.SrcPort,
		DstIP:   ctNormalizeFlowIP(t.DstIP),
		DstPort: t.DstPort,
	}
}

func ctNormalizeFlowIP(ip string) string {
	return strings.TrimPrefix(strings.TrimSpace(ip), "::ffff:")
}

func ctConntrackAccountingEnabled() (enabled bool, known bool) {
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
