package collector

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"unicode"

	"github.com/BlackMetalz/holyf-network/internal/kernelapi"
)

// SocketCounter holds monotonic TCP counters from ss/tcp_info for one tuple.
type SocketCounter struct {
	Tuple         FlowTuple
	BytesAcked    int64 // Local bytes sent and acked by peer.
	BytesReceived int64 // Local bytes received.
}

// CollectSocketTCPCounters reads TCP per-socket counters.
// When a SocketManager is available it uses the kernel API; otherwise it
// falls back to exec.Command("ss", ...).
func CollectSocketTCPCounters() ([]SocketCounter, error) {
	if socketMgr != nil {
		return collectSocketTCPCountersKernel()
	}
	return collectSocketTCPCountersExec()
}

// collectSocketTCPCountersKernel uses the kernelapi SocketManager.
func collectSocketTCPCountersKernel() ([]SocketCounter, error) {
	kaCounters, err := socketMgr.CollectTCPCounters()
	if err != nil {
		return nil, err
	}
	counters := make([]SocketCounter, len(kaCounters))
	for i, c := range kaCounters {
		counters[i] = convertKernelSocketCounter(c)
	}
	return counters, nil
}

// convertKernelSocketCounter converts a kernelapi.SocketCounter to a
// collector.SocketCounter.
func convertKernelSocketCounter(c kernelapi.SocketCounter) SocketCounter {
	return SocketCounter{
		Tuple: FlowTuple{
			SrcIP:   c.Tuple.SrcIP,
			SrcPort: c.Tuple.SrcPort,
			DstIP:   c.Tuple.DstIP,
			DstPort: c.Tuple.DstPort,
		},
		BytesAcked:    c.BytesAcked,
		BytesReceived: c.BytesReceived,
	}
}

// collectSocketTCPCountersExec reads TCP per-socket counters from ss.
func collectSocketTCPCountersExec() ([]SocketCounter, error) {
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

		if tuple, ok := parseSSStateLineTuple(line); ok {
			current = tuple
			haveTuple = true
			continue
		}

		if !haveTuple {
			continue
		}
		acked, hasAcked := parseSSMetric(line, "bytes_acked:")
		if !hasAcked {
			acked, hasAcked = parseSSMetric(line, "bytes_sent:")
		}
		recv, hasRecv := parseSSMetric(line, "bytes_received:")
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
			Tuple:         normalizeFlowTuple(current),
			BytesAcked:    acked,
			BytesReceived: recv,
		})
		// One counter row per socket tuple.
		haveTuple = false
	}

	return counters, nil
}

func parseSSStateLineTuple(line string) (FlowTuple, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return FlowTuple{}, false
	}
	if !looksLikeSSStateToken(fields[0]) {
		return FlowTuple{}, false
	}
	local := fields[3]
	peer := fields[4]

	localIP, localPort, ok := splitAddrPort(local)
	if !ok {
		return FlowTuple{}, false
	}
	peerIP, peerPort, ok := splitAddrPort(peer)
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

func looksLikeSSStateToken(token string) bool {
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

func splitAddrPort(addr string) (string, int, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0, false
	}

	host := ""
	portStr := ""

	if strings.HasPrefix(addr, "[") {
		// [ipv6]:port
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

func parseSSMetric(line, key string) (int64, bool) {
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
