package collector

import "testing"

func TestParseSSStateLineTuple(t *testing.T) {
	t.Parallel()

	line := "ESTAB 0 0 172.25.110.116:34616 90.130.70.73:80"
	tuple, ok := parseSSStateLineTuple(line)
	if !ok {
		t.Fatalf("expected tuple parse success")
	}
	if tuple.SrcIP != "172.25.110.116" || tuple.DstIP != "90.130.70.73" {
		t.Fatalf("tuple ip mismatch: %+v", tuple)
	}
	if tuple.SrcPort != 34616 || tuple.DstPort != 80 {
		t.Fatalf("tuple port mismatch: %+v", tuple)
	}
}

func TestParseSSStateLineTupleIPv6(t *testing.T) {
	t.Parallel()

	line := "ESTAB 0 0 [2001:db8::1]:12345 [2001:db8::2]:443"
	tuple, ok := parseSSStateLineTuple(line)
	if !ok {
		t.Fatalf("expected tuple parse success for ipv6")
	}
	if tuple.SrcIP != "2001:db8::1" || tuple.DstIP != "2001:db8::2" {
		t.Fatalf("ipv6 tuple mismatch: %+v", tuple)
	}
}

func TestParseSSMetric(t *testing.T) {
	t.Parallel()

	line := "cubic wscale:7,7 rto:204 bytes_acked:12345 bytes_received:67890 segs_out:123"
	acked, ok := parseSSMetric(line, "bytes_acked:")
	if !ok || acked != 12345 {
		t.Fatalf("bytes_acked parse mismatch: ok=%v value=%d", ok, acked)
	}
	recv, ok := parseSSMetric(line, "bytes_received:")
	if !ok || recv != 67890 {
		t.Fatalf("bytes_received parse mismatch: ok=%v value=%d", ok, recv)
	}
}
