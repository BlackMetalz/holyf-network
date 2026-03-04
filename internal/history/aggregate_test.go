package history

import (
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

func TestAggregateConnectionsGroupsAndSums(t *testing.T) {
	t.Parallel()

	rows := AggregateConnections([]collector.Connection{
		{
			RemoteIP:  "198.51.100.10",
			LocalPort: 22,
			ProcName:  "sshd",
			State:     "ESTABLISHED",
			TxQueue:   10,
			RxQueue:   2,
			Activity:  12,
		},
		{
			RemoteIP:  "::ffff:198.51.100.10",
			LocalPort: 22,
			ProcName:  "sshd",
			State:     "ESTABLISHED",
			TxQueue:   3,
			RxQueue:   1,
			Activity:  4,
		},
		{
			RemoteIP:  "198.51.100.10",
			LocalPort: 443,
			ProcName:  "",
			State:     "TIME_WAIT",
			TxQueue:   1,
			RxQueue:   0,
			Activity:  1,
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
		{RemoteIP: "198.51.100.2", LocalPort: 22, ProcName: "b", State: "ESTABLISHED", Activity: 2},
		{RemoteIP: "198.51.100.1", LocalPort: 22, ProcName: "a", State: "ESTABLISHED", Activity: 5},
		{RemoteIP: "198.51.100.3", LocalPort: 22, ProcName: "c", State: "ESTABLISHED", Activity: 1},
	}, 2)

	if len(rows) != 2 {
		t.Fatalf("limit mismatch: got=%d want=2", len(rows))
	}
	if rows[0].PeerIP != "198.51.100.1" {
		t.Fatalf("expected highest queue first, got=%s", rows[0].PeerIP)
	}
}

func TestAggregateConnectionsPrefersHigherConnCountForCap(t *testing.T) {
	t.Parallel()

	rows := AggregateConnections([]collector.Connection{
		{RemoteIP: "198.51.100.1", LocalPort: 22, ProcName: "sshd", State: "ESTABLISHED", TxQueue: 500, RxQueue: 0, Activity: 500},
		{RemoteIP: "198.51.100.2", LocalPort: 22, ProcName: "sshd", State: "ESTABLISHED", TxQueue: 10, RxQueue: 0, Activity: 10},
		{RemoteIP: "198.51.100.2", LocalPort: 22, ProcName: "sshd", State: "ESTABLISHED", TxQueue: 10, RxQueue: 0, Activity: 10},
	}, 1)

	if len(rows) != 1 {
		t.Fatalf("limit mismatch: got=%d want=1", len(rows))
	}
	if rows[0].PeerIP != "198.51.100.2" || rows[0].ConnCount != 2 {
		t.Fatalf("expected higher conn_count row to be kept, got=%+v", rows[0])
	}
}
