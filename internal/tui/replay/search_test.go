package replay

import (
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
)

func TestParsePortFilter(t *testing.T) {
	t.Parallel()
	if got, err := ParsePortFilter("443"); err != nil || got != "443" {
		t.Fatalf("ParsePortFilter valid: got=%q err=%v", got, err)
	}
	if _, err := ParsePortFilter("70000"); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestFindNearestNonEmptyIndex(t *testing.T) {
	t.Parallel()
	refs := []history.SnapshotRef{{Offset: 0}, {Offset: 1}, {Offset: 2}, {Offset: 3}}
	counts := []int{1, 0, 0, 2}
	idx, ok := FindNearestNonEmptyIndex(refs, 2, func(ref history.SnapshotRef) int {
		return counts[int(ref.Offset)]
	})
	if !ok || idx != 3 {
		t.Fatalf("FindNearestNonEmptyIndex got idx=%d ok=%v", idx, ok)
	}
}

func TestClosestSnapshotIndex(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	refs := []history.SnapshotRef{
		{CapturedAt: base},
		{CapturedAt: base.Add(10 * time.Minute)},
		{CapturedAt: base.Add(20 * time.Minute)},
	}
	if got := ClosestSnapshotIndex(refs, base.Add(7*time.Minute)); got != 1 {
		t.Fatalf("ClosestSnapshotIndex got=%d want=1", got)
	}
}
