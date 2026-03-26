package replay

import (
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
)

func TestBuildTraceOnlyRefsSortsAndSkipsZeroTime(t *testing.T) {
	base := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	refs, entries := BuildTraceOnlyRefs([]tuitrace.Entry{
		{PeerIP: "skip"},
		{PeerIP: "b", CapturedAt: base.Add(2 * time.Minute)},
		{PeerIP: "a", CapturedAt: base},
	})

	if len(refs) != 2 || len(entries) != 2 {
		t.Fatalf("expected 2 refs and entries, got %d refs %d entries", len(refs), len(entries))
	}
	if entries[0].PeerIP != "a" || entries[1].PeerIP != "b" {
		t.Fatalf("unexpected order: %+v", entries)
	}
	if refs[0].Offset != 0 || refs[1].Offset != 1 {
		t.Fatalf("unexpected offsets: %+v", refs)
	}
}

func TestTraceTimelineAssociationWindowClamps(t *testing.T) {
	base := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	window := TraceTimelineAssociationWindow([]history.SnapshotRef{
		{CapturedAt: base},
		{CapturedAt: base.Add(2 * time.Second)},
		{CapturedAt: base.Add(4 * time.Second)},
	})
	if window != 15*time.Second {
		t.Fatalf("expected clamp to 15s, got %s", window)
	}
}

func TestBuildTraceTimelineAssociatesNearestSnapshotAndSortsNewestFirst(t *testing.T) {
	base := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	refs := []history.SnapshotRef{
		{CapturedAt: base},
		{CapturedAt: base.Add(30 * time.Second)},
		{CapturedAt: base.Add(60 * time.Second)},
	}
	result := BuildTraceTimeline(refs, []tuitrace.Entry{
		{PeerIP: "late", CapturedAt: base.Add(32 * time.Second)},
		{PeerIP: "early", CapturedAt: base.Add(28 * time.Second)},
		{PeerIP: "far", CapturedAt: base.Add(30 * time.Minute)},
	})

	if result.Total != 3 {
		t.Fatalf("expected total 3, got %d", result.Total)
	}
	if result.Associated != 2 {
		t.Fatalf("expected associated 2, got %d", result.Associated)
	}
	entries := result.BySnapshot[1]
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries near snapshot 1, got %d", len(entries))
	}
	if entries[0].PeerIP != "late" || entries[1].PeerIP != "early" {
		t.Fatalf("unexpected order: %+v", entries)
	}
}

func TestFilterSnapshotRefsByRange(t *testing.T) {
	base := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	begin := base.Add(10 * time.Second)
	end := base.Add(20 * time.Second)
	refs := FilterSnapshotRefsByRange([]history.SnapshotRef{
		{CapturedAt: base},
		{CapturedAt: base.Add(15 * time.Second)},
		{CapturedAt: base.Add(30 * time.Second)},
	}, &begin, &end)
	if len(refs) != 1 || !refs[0].CapturedAt.Equal(base.Add(15*time.Second)) {
		t.Fatalf("unexpected filtered refs: %+v", refs)
	}
}
