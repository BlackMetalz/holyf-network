package collector

import "testing"

func TestParseListenPortsCollectsOnlyListenSockets(t *testing.T) {
	t.Parallel()

	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000   100        0 1 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000   100        0 2 1 0000000000000000 100 0 0 10 0
   2: 0100007F:1F91 0200007F:C001 01 00000000:00000000 00:00000000 00000000   100        0 3 1 0000000000000000 100 0 0 10 0
`

	ports := parseListenPorts(content, false)
	if len(ports) != 2 {
		t.Fatalf("listen port count mismatch: got=%d want=%d", len(ports), 2)
	}
	if _, ok := ports[80]; !ok {
		t.Fatalf("expected port 80 in parsed listen ports: %+v", ports)
	}
	if _, ok := ports[8080]; !ok {
		t.Fatalf("expected port 8080 in parsed listen ports: %+v", ports)
	}
	if _, ok := ports[8081]; ok {
		t.Fatalf("did not expect established socket port in listen ports: %+v", ports)
	}
}
