package tui

import (
	"strings"
	"testing"
)

func TestBuildTracePacketFilter(t *testing.T) {
	t.Parallel()

	req := tracePacketRequest{
		PeerIP: "203.0.113.10",
		Port:   443,
		Scope:  traceScopePeerPort,
	}
	if got := buildTracePacketFilter(req); got != "tcp and host 203.0.113.10 and port 443" {
		t.Fatalf("unexpected peer+port filter: %q", got)
	}

	req.Scope = traceScopePeerOnly
	if got := buildTracePacketFilter(req); got != "tcp and host 203.0.113.10" {
		t.Fatalf("unexpected peer-only filter: %q", got)
	}
}

func TestParseTracePacketCounters(t *testing.T) {
	t.Parallel()

	raw := `tcpdump: listening on eth0, link-type EN10MB (Ethernet), snapshot length 262144 bytes
12 packets captured
17 packets received by filter
1 packets dropped by kernel`

	captured, receivedByFilter, droppedByKernel := parseTracePacketCounters(raw)
	if captured != 12 || receivedByFilter != 17 || droppedByKernel != 1 {
		t.Fatalf("unexpected counters: captured=%d recv=%d drop=%d", captured, receivedByFilter, droppedByKernel)
	}
}

func TestParseTracePacketCountersMissingRowsStayNA(t *testing.T) {
	t.Parallel()

	captured, receivedByFilter, droppedByKernel := parseTracePacketCounters("tcpdump: no summary lines")
	if captured != -1 || receivedByFilter != -1 || droppedByKernel != -1 {
		t.Fatalf("expected -1 defaults, got captured=%d recv=%d drop=%d", captured, receivedByFilter, droppedByKernel)
	}
}

func TestParseTracePacketIntRange(t *testing.T) {
	t.Parallel()

	v, err := parseTracePacketIntRange(" 10 ", 1, 60, "Duration")
	if err != nil || v != 10 {
		t.Fatalf("expected valid parse, got v=%d err=%v", v, err)
	}
	if _, err := parseTracePacketIntRange("0", 1, 60, "Duration"); err == nil {
		t.Fatalf("expected error for out-of-range value")
	}
	if _, err := parseTracePacketIntRange("abc", 1, 60, "Duration"); err == nil {
		t.Fatalf("expected error for invalid integer")
	}
}

func TestBuildTracePacketActionSummaryIncludesKeyFields(t *testing.T) {
	t.Parallel()

	result := tracePacketResult{
		Request: tracePacketRequest{
			PeerIP:    "203.0.113.10",
			Port:      443,
			Scope:     traceScopePeerPort,
			Direction: traceDirectionIn,
		},
		Captured:        12,
		DroppedByKernel: 1,
		RstCount:        2,
		Saved:           true,
		PCAPPath:        "/tmp/holyf-network/captures/trace-test.pcap",
	}

	summary := buildTracePacketActionSummary(result)
	for _, want := range []string{
		"Trace ok 203.0.113.10:443",
		"dir=IN scope=Peer + Port",
		"captured=12 drop=1 rst=2",
		"saved=/tmp/holyf-network/captures/trace-test.pcap",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected summary to contain %q, got: %q", want, summary)
		}
	}
}
