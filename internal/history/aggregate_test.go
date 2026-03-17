package history

import (
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

func TestAggregateConnectionsGroupsAndSums(t *testing.T) {
	t.Parallel()

	rows := AggregateConnections([]collector.Connection{
		{
			RemoteIP:         "198.51.100.10",
			LocalPort:        22,
			ProcName:         "sshd",
			State:            "ESTABLISHED",
			TxQueue:          10,
			RxQueue:          2,
			Activity:         12,
			TxBytesDelta:     1000,
			RxBytesDelta:     500,
			TotalBytesDelta:  1500,
			TxBytesPerSec:    100,
			RxBytesPerSec:    50,
			TotalBytesPerSec: 150,
		},
		{
			RemoteIP:         "::ffff:198.51.100.10",
			LocalPort:        22,
			ProcName:         "sshd",
			State:            "ESTABLISHED",
			TxQueue:          3,
			RxQueue:          1,
			Activity:         4,
			TxBytesDelta:     700,
			RxBytesDelta:     300,
			TotalBytesDelta:  1000,
			TxBytesPerSec:    70,
			RxBytesPerSec:    30,
			TotalBytesPerSec: 100,
		},
		{
			RemoteIP:         "198.51.100.10",
			LocalPort:        443,
			ProcName:         "",
			State:            "TIME_WAIT",
			TxQueue:          1,
			RxQueue:          0,
			Activity:         1,
			TxBytesDelta:     40,
			RxBytesDelta:     10,
			TotalBytesDelta:  50,
			TxBytesPerSec:    4,
			RxBytesPerSec:    1,
			TotalBytesPerSec: 5,
		},
	}, 0)

	if len(rows) != 2 {
		t.Fatalf("aggregate row count mismatch: got=%d want=2", len(rows))
	}

	first := rows[0]
	if first.PeerIP != "198.51.100.10" || first.LocalPort != 22 || first.ProcName != "sshd" {
		t.Fatalf("first aggregate key mismatch: %+v", first)
	}
	if first.ConnCount != 2 || first.TxQueue != 13 || first.RxQueue != 3 || first.TotalQueue != 16 {
		t.Fatalf("first aggregate sums mismatch: %+v", first)
	}
	if first.TotalBytesDelta != 2500 || first.TxBytesDelta != 1700 || first.RxBytesDelta != 800 {
		t.Fatalf("first aggregate bandwidth sums mismatch: %+v", first)
	}
	if got := first.States["ESTABLISHED"]; got != 2 {
		t.Fatalf("state count mismatch for ESTABLISHED: got=%d want=2", got)
	}

	second := rows[1]
	if second.ProcName != "unknown" {
		t.Fatalf("expected unknown proc fallback, got=%q", second.ProcName)
	}
}

func TestAggregateConnectionsLimit(t *testing.T) {
	t.Parallel()

	rows := AggregateConnections([]collector.Connection{
		{RemoteIP: "198.51.100.2", LocalPort: 22, ProcName: "b", State: "ESTABLISHED", Activity: 2, TotalBytesDelta: 20},
		{RemoteIP: "198.51.100.1", LocalPort: 22, ProcName: "a", State: "ESTABLISHED", Activity: 5, TotalBytesDelta: 50},
		{RemoteIP: "198.51.100.3", LocalPort: 22, ProcName: "c", State: "ESTABLISHED", Activity: 1, TotalBytesDelta: 10},
	}, 2)

	if len(rows) != 2 {
		t.Fatalf("limit mismatch: got=%d want=2", len(rows))
	}
	if rows[0].PeerIP != "198.51.100.1" {
		t.Fatalf("expected highest bandwidth first, got=%s", rows[0].PeerIP)
	}
}

func TestAggregateConnectionsPrefersHigherBandwidthForCap(t *testing.T) {
	t.Parallel()

	rows := AggregateConnections([]collector.Connection{
		{RemoteIP: "198.51.100.1", LocalPort: 22, ProcName: "sshd", State: "ESTABLISHED", TxQueue: 10, RxQueue: 0, Activity: 10, TotalBytesDelta: 5000},
		{RemoteIP: "198.51.100.2", LocalPort: 22, ProcName: "sshd", State: "ESTABLISHED", TxQueue: 200, RxQueue: 0, Activity: 200, TotalBytesDelta: 100},
		{RemoteIP: "198.51.100.2", LocalPort: 22, ProcName: "sshd", State: "ESTABLISHED", TxQueue: 200, RxQueue: 0, Activity: 200, TotalBytesDelta: 100},
	}, 1)

	if len(rows) != 1 {
		t.Fatalf("limit mismatch: got=%d want=1", len(rows))
	}
	if rows[0].PeerIP != "198.51.100.1" {
		t.Fatalf("expected higher bandwidth row to be kept, got=%+v", rows[0])
	}
}

func TestAggregateConnectionsByDirectionOutgoingUsesRemotePort(t *testing.T) {
	t.Parallel()

	rows := AggregateConnectionsByDirection([]collector.Connection{
		{RemoteIP: "198.51.100.10", LocalPort: 52001, RemotePort: 443, ProcName: "curl", State: "ESTABLISHED", TotalBytesDelta: 100},
		{RemoteIP: "198.51.100.10", LocalPort: 52002, RemotePort: 443, ProcName: "curl", State: "ESTABLISHED", TotalBytesDelta: 50},
		{RemoteIP: "198.51.100.10", LocalPort: 52003, RemotePort: 8443, ProcName: "curl", State: "ESTABLISHED", TotalBytesDelta: 30},
	}, AggregateOutgoing, 0)

	if len(rows) != 2 {
		t.Fatalf("expected 2 outgoing groups, got=%d", len(rows))
	}
	if rows[0].Port != 443 || rows[0].ConnCount != 2 {
		t.Fatalf("expected remote port 443 group with 2 conns, got=%+v", rows[0])
	}
	if rows[1].Port != 8443 || rows[1].ConnCount != 1 {
		t.Fatalf("expected remote port 8443 group with 1 conn, got=%+v", rows[1])
	}
}

func TestSplitConnectionsByDirectionFiltersSelfAndUsesListenerPorts(t *testing.T) {
	t.Parallel()

	listenPorts := map[int]struct{}{
		18080: {},
	}
	conns := []collector.Connection{
		{PID: 2001, LocalPort: 18080, RemotePort: 52001},
		{PID: 2002, LocalPort: 52246, RemotePort: 443},
		{PID: 4242, LocalPort: 52247, RemotePort: 8443},
	}

	incoming, outgoing := SplitConnectionsByDirection(conns, listenPorts, 4242)
	if len(incoming) != 1 || incoming[0].LocalPort != 18080 {
		t.Fatalf("expected one incoming listener-backed connection, got=%+v", incoming)
	}
	if len(outgoing) != 1 || outgoing[0].RemotePort != 443 {
		t.Fatalf("expected one outgoing non-self connection, got=%+v", outgoing)
	}
}
