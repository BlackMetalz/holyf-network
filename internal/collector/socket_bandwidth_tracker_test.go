package collector

import (
	"testing"
	"time"
)

func TestSocketBandwidthTrackerDelta(t *testing.T) {
	t.Parallel()

	tracker := NewSocketBandwidthTracker()
	t1 := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Second)

	tuple := FlowTuple{
		SrcIP:   "172.25.110.116",
		SrcPort: 34616,
		DstIP:   "90.130.70.73",
		DstPort: 80,
	}

	tracker.BuildSnapshot([]SocketCounter{
		{Tuple: tuple, BytesAcked: 1000, BytesReceived: 2000},
	}, t1)

	snapshot := tracker.BuildSnapshot([]SocketCounter{
		{Tuple: tuple, BytesAcked: 3000, BytesReceived: 5000},
	}, t2)

	if !snapshot.Available {
		t.Fatalf("snapshot should be available")
	}
	bw := snapshot.ByTuple[normalizeFlowTuple(tuple)]
	if bw.TxBytesDelta != 2000 || bw.RxBytesDelta != 3000 || bw.TotalBytesDelta != 5000 {
		t.Fatalf("delta mismatch: %+v", bw)
	}
	if bw.TotalBytesPerSec != 2500 {
		t.Fatalf("rate mismatch: got=%.2f want=2500", bw.TotalBytesPerSec)
	}
}

func TestOverlayMissingBandwidth(t *testing.T) {
	t.Parallel()

	conns := []Connection{
		{
			LocalIP:    "172.25.110.116",
			LocalPort:  34616,
			RemoteIP:   "90.130.70.73",
			RemotePort: 80,
		},
	}
	snapshot := BandwidthSnapshot{
		ByTuple: map[FlowTuple]TupleBandwidth{
			{
				SrcIP:   "172.25.110.116",
				SrcPort: 34616,
				DstIP:   "90.130.70.73",
				DstPort: 80,
			}: {
				TxBytesDelta:     512,
				RxBytesDelta:     1024,
				TotalBytesDelta:  1536,
				TxBytesPerSec:    512,
				RxBytesPerSec:    1024,
				TotalBytesPerSec: 1536,
			},
		},
		Available: true,
	}

	got := OverlayMissingBandwidth(conns, snapshot)
	if got[0].TotalBytesDelta != 1536 {
		t.Fatalf("overlay should fill missing bandwidth, got=%+v", got[0])
	}
}
