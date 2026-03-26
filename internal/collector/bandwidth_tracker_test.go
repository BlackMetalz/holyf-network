package collector

import (
	"testing"
	"time"
)

func TestBandwidthTrackerFirstSampleIsBaseline(t *testing.T) {
	t.Parallel()

	tracker := NewBandwidthTracker()
	now := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	flows := []ConntrackFlow{
		{
			Orig:       FlowTuple{SrcIP: "172.25.110.76", SrcPort: 53506, DstIP: "172.25.110.116", DstPort: 22},
			Reply:      FlowTuple{SrcIP: "172.25.110.116", SrcPort: 22, DstIP: "172.25.110.76", DstPort: 53506},
			OrigBytes:  1000,
			ReplyBytes: 2000,
		},
	}

	snapshot := tracker.BuildSnapshot(flows, now)
	if snapshot.Available {
		t.Fatalf("first sample must not be available")
	}
	if snapshot.SampleSeconds != 0 {
		t.Fatalf("first sample seconds mismatch: got=%v want=0", snapshot.SampleSeconds)
	}
}

func TestBandwidthTrackerComputesDeltaAndRate(t *testing.T) {
	t.Parallel()

	tracker := NewBandwidthTracker()
	t1 := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Second)

	base := ConntrackFlow{
		Orig:       FlowTuple{SrcIP: "172.25.110.76", SrcPort: 53506, DstIP: "172.25.110.116", DstPort: 22},
		Reply:      FlowTuple{SrcIP: "172.25.110.116", SrcPort: 22, DstIP: "172.25.110.76", DstPort: 53506},
		OrigBytes:  1000,
		ReplyBytes: 2000,
	}
	tracker.BuildSnapshot([]ConntrackFlow{base}, t1)

	current := base
	current.OrigBytes = 1600
	current.ReplyBytes = 2600
	snapshot := tracker.BuildSnapshot([]ConntrackFlow{current}, t2)
	if !snapshot.Available {
		t.Fatalf("second sample should be available")
	}
	if snapshot.SampleSeconds != 5 {
		t.Fatalf("sample seconds mismatch: got=%.2f want=5", snapshot.SampleSeconds)
	}

	localTuple := FlowTuple{
		SrcIP:   "172.25.110.116",
		SrcPort: 22,
		DstIP:   "172.25.110.76",
		DstPort: 53506,
	}
	bw := snapshot.ByTuple[localTuple]
	if bw.TxBytesDelta != 600 || bw.RxBytesDelta != 600 || bw.TotalBytesDelta != 1200 {
		t.Fatalf("delta mismatch: %+v", bw)
	}
	if bw.TotalBytesPerSec != 240 {
		t.Fatalf("rate mismatch: got=%.2f want=240", bw.TotalBytesPerSec)
	}
}

func TestBandwidthTrackerClampsCounterReset(t *testing.T) {
	t.Parallel()

	tracker := NewBandwidthTracker()
	t1 := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Second)

	flow := ConntrackFlow{
		Orig:       FlowTuple{SrcIP: "10.0.0.2", SrcPort: 50000, DstIP: "10.0.0.1", DstPort: 22},
		Reply:      FlowTuple{SrcIP: "10.0.0.1", SrcPort: 22, DstIP: "10.0.0.2", DstPort: 50000},
		OrigBytes:  3000,
		ReplyBytes: 7000,
	}
	tracker.BuildSnapshot([]ConntrackFlow{flow}, t1)

	flow.OrigBytes = 100
	flow.ReplyBytes = 200
	snapshot := tracker.BuildSnapshot([]ConntrackFlow{flow}, t2)

	localTuple := FlowTuple{
		SrcIP:   "10.0.0.1",
		SrcPort: 22,
		DstIP:   "10.0.0.2",
		DstPort: 50000,
	}
	bw := snapshot.ByTuple[localTuple]
	if bw.TxBytesDelta != 0 || bw.RxBytesDelta != 0 || bw.TotalBytesDelta != 0 {
		t.Fatalf("expected zero delta after reset, got %+v", bw)
	}
}

func TestBandwidthTrackerFirstSeenFlowAfterBaselineSkipsAccumulated(t *testing.T) {
	t.Parallel()

	tracker := NewBandwidthTracker()
	t1 := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Second)
	t3 := t2.Add(5 * time.Second)

	// First call sets global baseline with one unrelated flow.
	baseline := ConntrackFlow{
		Orig:       FlowTuple{SrcIP: "10.0.0.2", SrcPort: 40000, DstIP: "10.0.0.1", DstPort: 22},
		Reply:      FlowTuple{SrcIP: "10.0.0.1", SrcPort: 22, DstIP: "10.0.0.2", DstPort: 40000},
		OrigBytes:  100,
		ReplyBytes: 200,
	}
	tracker.BuildSnapshot([]ConntrackFlow{baseline}, t1)

	// New flow appears after baseline with accumulated bytes — first-seen
	// should report zero delta (accumulated bytes are historical, not this interval).
	newFlow := ConntrackFlow{
		Orig:       FlowTuple{SrcIP: "10.0.0.3", SrcPort: 50000, DstIP: "10.0.0.1", DstPort: 8080},
		Reply:      FlowTuple{SrcIP: "10.0.0.1", SrcPort: 8080, DstIP: "10.0.0.3", DstPort: 50000},
		OrigBytes:  4096,
		ReplyBytes: 8192,
	}
	snapshot := tracker.BuildSnapshot([]ConntrackFlow{newFlow}, t2)
	if !snapshot.Available {
		t.Fatalf("snapshot should be available after baseline")
	}

	localTuple := FlowTuple{
		SrcIP:   "10.0.0.1",
		SrcPort: 8080,
		DstIP:   "10.0.0.3",
		DstPort: 50000,
	}
	bw := snapshot.ByTuple[localTuple]
	if bw.TxBytesDelta != 0 || bw.RxBytesDelta != 0 {
		t.Fatalf("first-seen flow should skip accumulated bytes, got %+v", bw)
	}

	// On the third sample, the flow gets a real delta.
	newFlow.OrigBytes = 5096
	newFlow.ReplyBytes = 9192
	snapshot3 := tracker.BuildSnapshot([]ConntrackFlow{newFlow}, t3)
	bw3 := snapshot3.ByTuple[localTuple]
	if bw3.RxBytesDelta != 1000 || bw3.TxBytesDelta != 1000 {
		t.Fatalf("second sample should show real delta, got %+v", bw3)
	}
}

func TestEnrichConnectionsWithBandwidth(t *testing.T) {
	t.Parallel()

	conns := []Connection{
		{
			LocalIP:    "::ffff:10.0.0.1",
			LocalPort:  22,
			RemoteIP:   "::ffff:10.0.0.2",
			RemotePort: 50000,
		},
	}
	snapshot := BandwidthSnapshot{
		ByTuple: map[FlowTuple]TupleBandwidth{
			{
				SrcIP:   "10.0.0.1",
				SrcPort: 22,
				DstIP:   "10.0.0.2",
				DstPort: 50000,
			}: {
				TxBytesDelta:     100,
				RxBytesDelta:     200,
				TotalBytesDelta:  300,
				TxBytesPerSec:    10,
				RxBytesPerSec:    20,
				TotalBytesPerSec: 30,
			},
		},
		Available: true,
	}

	got := EnrichConnectionsWithBandwidth(conns, snapshot)
	if got[0].TotalBytesDelta != 300 || got[0].TotalBytesPerSec != 30 {
		t.Fatalf("enrich mismatch: %+v", got[0])
	}
}

func TestEnrichConnectionsWithBandwidthFallbackIgnoresLocalIP(t *testing.T) {
	t.Parallel()

	conns := []Connection{
		{
			LocalIP:    "172.25.110.116",
			LocalPort:  59856,
			RemoteIP:   "90.130.70.73",
			RemotePort: 80,
		},
	}
	// Snapshot tuple has different local IP (NAT/translation scenario),
	// but same peer + ports.
	snapshot := BandwidthSnapshot{
		ByTuple: map[FlowTuple]TupleBandwidth{
			{
				SrcIP:   "10.1.2.3",
				SrcPort: 59856,
				DstIP:   "90.130.70.73",
				DstPort: 80,
			}: {
				TxBytesDelta:     120,
				RxBytesDelta:     4096,
				TotalBytesDelta:  4216,
				TxBytesPerSec:    12,
				RxBytesPerSec:    409.6,
				TotalBytesPerSec: 421.6,
			},
		},
		Available: true,
	}

	got := EnrichConnectionsWithBandwidth(conns, snapshot)
	if got[0].TotalBytesDelta != 4216 || got[0].RxBytesDelta != 4096 {
		t.Fatalf("fallback enrich mismatch: %+v", got[0])
	}
}
