package collector

import (
	"strings"
	"testing"
)

func TestParseConntrackFlowLineIPv4(t *testing.T) {
	t.Parallel()

	line := "tcp 6 431999 ESTABLISHED src=172.25.110.76 dst=172.25.110.116 sport=53506 dport=22 packets=50 bytes=4096 src=172.25.110.116 dst=172.25.110.76 sport=22 dport=53506 packets=45 bytes=8192 [ASSURED] mark=0 use=1"
	flow, ok := parseConntrackFlowLine(line)
	if !ok {
		t.Fatalf("expected parse success")
	}

	if flow.Orig.SrcIP != "172.25.110.76" || flow.Orig.DstIP != "172.25.110.116" {
		t.Fatalf("orig tuple mismatch: %+v", flow.Orig)
	}
	if flow.Orig.SrcPort != 53506 || flow.Orig.DstPort != 22 {
		t.Fatalf("orig ports mismatch: %+v", flow.Orig)
	}
	if flow.Reply.SrcPort != 22 || flow.Reply.DstPort != 53506 {
		t.Fatalf("reply ports mismatch: %+v", flow.Reply)
	}
	if flow.OrigBytes != 4096 || flow.ReplyBytes != 8192 {
		t.Fatalf("bytes mismatch: orig=%d reply=%d", flow.OrigBytes, flow.ReplyBytes)
	}
	if flow.State != "ESTABLISHED" {
		t.Fatalf("state mismatch: got=%q want=%q", flow.State, "ESTABLISHED")
	}
}

func TestParseConntrackFlowLineIPv4MappedIPv6(t *testing.T) {
	t.Parallel()

	line := "tcp 6 120 ESTABLISHED src=::ffff:10.0.0.2 dst=::ffff:10.0.0.1 sport=443 dport=43000 packets=10 bytes=2048 src=::ffff:10.0.0.1 dst=::ffff:10.0.0.2 sport=43000 dport=443 packets=9 bytes=1024 mark=0 use=1"
	flow, ok := parseConntrackFlowLine(line)
	if !ok {
		t.Fatalf("expected parse success")
	}

	if flow.Orig.SrcIP != "::ffff:10.0.0.2" {
		t.Fatalf("unexpected src ip: %s", flow.Orig.SrcIP)
	}
	if flow.OrigBytes != 2048 || flow.ReplyBytes != 1024 {
		t.Fatalf("bytes mismatch: %+v", flow)
	}
}

func TestParseConntrackFlowLineServiceNames(t *testing.T) {
	t.Parallel()

	line := "tcp 6 120 ESTABLISHED src=10.0.0.2 dst=10.0.0.1 sport=https dport=ssh packets=10 bytes=2048 src=10.0.0.1 dst=10.0.0.2 sport=ssh dport=https packets=9 bytes=1024 mark=0 use=1"
	flow, ok := parseConntrackFlowLine(line)
	if !ok {
		t.Fatalf("expected parse success with service-name ports")
	}
	if flow.Orig.SrcPort != 443 || flow.Orig.DstPort != 22 {
		t.Fatalf("unexpected resolved ports for orig tuple: %+v", flow.Orig)
	}
	if flow.Reply.SrcPort != 22 || flow.Reply.DstPort != 443 {
		t.Fatalf("unexpected resolved ports for reply tuple: %+v", flow.Reply)
	}
}

func TestParseConntrackFlowLineWithoutBytesStillParses(t *testing.T) {
	t.Parallel()

	line := "tcp 6 120 ESTABLISHED src=10.0.0.2 dst=10.0.0.1 sport=443 dport=43000 packets=10 src=10.0.0.1 dst=10.0.0.2 sport=43000 dport=443 packets=9"
	flow, ok := parseConntrackFlowLine(line)
	if !ok {
		t.Fatalf("expected parse success even when bytes are missing")
	}
	if flow.OrigBytes != 0 || flow.ReplyBytes != 0 {
		t.Fatalf("expected missing bytes to default to 0, got orig=%d reply=%d", flow.OrigBytes, flow.ReplyBytes)
	}
}

func TestParseConntrackFlowsOutput(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		"conntrack v1.4.8 (conntrack-tools): 1 flow entries have been shown.",
		"tcp      6 431970 ESTABLISHED src=172.25.110.116 dst=172.25.110.76 sport=35678 dport=3306 src=172.20.0.2 dst=172.25.110.116 sport=3306 dport=35678 [ASSURED] mark=0 use=1",
		"",
	}, "\n")
	flows, candidates := parseConntrackFlowsOutput(raw)
	if candidates != 1 {
		t.Fatalf("candidate count mismatch: got=%d want=1", candidates)
	}
	if len(flows) != 1 {
		t.Fatalf("flow count mismatch: got=%d want=1", len(flows))
	}
}

func TestLooksLikeConntrackFlowLine(t *testing.T) {
	t.Parallel()

	if !looksLikeConntrackFlowLine("tcp 6 431999 ESTABLISHED src=1.1.1.1 dst=2.2.2.2 sport=123 dport=80") {
		t.Fatalf("expected flow-like line to be detected")
	}
	if looksLikeConntrackFlowLine("conntrack v1.4.7 (conntrack-tools)") {
		t.Fatalf("expected non-flow line to be ignored")
	}
}

func TestConntrackFlowCanonicalKeyStableAcrossDirections(t *testing.T) {
	t.Parallel()

	a := ConntrackFlow{
		Orig:  FlowTuple{SrcIP: "172.25.110.116", SrcPort: 43286, DstIP: "172.25.110.76", DstPort: 3306},
		Reply: FlowTuple{SrcIP: "172.20.0.2", SrcPort: 3306, DstIP: "172.25.110.116", DstPort: 43286},
	}
	b := ConntrackFlow{
		Orig:  a.Reply,
		Reply: a.Orig,
	}

	if conntrackFlowCanonicalKey(a) != conntrackFlowCanonicalKey(b) {
		t.Fatalf("canonical key should match across direction swaps")
	}
}

func TestMergeConntrackFlowSetMergesDistinctFlows(t *testing.T) {
	t.Parallel()

	flowsA := []ConntrackFlow{
		{
			Orig:  FlowTuple{SrcIP: "172.25.110.116", SrcPort: 50000, DstIP: "172.25.110.76", DstPort: 22},
			Reply: FlowTuple{SrcIP: "172.25.110.76", SrcPort: 22, DstIP: "172.25.110.116", DstPort: 50000},
		},
	}
	flowsB := []ConntrackFlow{
		{
			Orig:  FlowTuple{SrcIP: "172.25.110.116", SrcPort: 43286, DstIP: "172.25.110.76", DstPort: 3306},
			Reply: FlowTuple{SrcIP: "172.20.0.2", SrcPort: 3306, DstIP: "172.25.110.116", DstPort: 43286},
		},
	}

	merged := map[string]ConntrackFlow{}
	mergeConntrackFlowSet(merged, flowsA)
	mergeConntrackFlowSet(merged, flowsB)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged flows, got=%d", len(merged))
	}
}

func TestMergeConntrackFlowSetPrefersFlowWithRicherBytes(t *testing.T) {
	t.Parallel()

	low := ConntrackFlow{
		State:      "ESTABLISHED",
		Orig:       FlowTuple{SrcIP: "172.25.110.116", SrcPort: 43286, DstIP: "172.25.110.76", DstPort: 3306},
		Reply:      FlowTuple{SrcIP: "172.20.0.2", SrcPort: 3306, DstIP: "172.25.110.116", DstPort: 43286},
		OrigBytes:  0,
		ReplyBytes: 0,
	}
	high := low
	high.OrigBytes = 4096
	high.ReplyBytes = 8192

	merged := map[string]ConntrackFlow{}
	mergeConntrackFlowSet(merged, []ConntrackFlow{low})
	mergeConntrackFlowSet(merged, []ConntrackFlow{high})

	if len(merged) != 1 {
		t.Fatalf("expected 1 merged flow, got=%d", len(merged))
	}

	got := merged[conntrackFlowCanonicalKey(low)]
	if got.OrigBytes != 4096 || got.ReplyBytes != 8192 {
		t.Fatalf("expected richer bytes flow to win, got orig=%d reply=%d", got.OrigBytes, got.ReplyBytes)
	}
}
