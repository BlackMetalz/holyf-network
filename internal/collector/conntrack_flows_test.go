package collector

import "testing"

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

func TestParseConntrackFlowLineInvalid(t *testing.T) {
	t.Parallel()

	line := "tcp 6 120 ESTABLISHED src=10.0.0.2 dst=10.0.0.1 sport=443 dport=43000 packets=10 src=10.0.0.1 dst=10.0.0.2 sport=43000 dport=443 packets=9"
	if _, ok := parseConntrackFlowLine(line); ok {
		t.Fatalf("expected parse to fail for missing bytes fields")
	}
}
