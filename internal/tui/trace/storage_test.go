package trace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneDataDirByAge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.Local)
	old := SegmentFileName(now.AddDate(0, 0, -10))
	cur := SegmentFileName(now)
	if err := os.WriteFile(filepath.Join(dir, old), []byte("{\"captured_at\":\"2026-03-20T10:00:00Z\"}\n"), 0o600); err != nil {
		t.Fatalf("write old segment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cur), []byte("{\"captured_at\":\"2026-03-30T10:00:00Z\"}\n"), 0o600); err != nil {
		t.Fatalf("write current segment: %v", err)
	}

	if err := PruneDataDirByAge(dir, 24*7, now); err != nil {
		t.Fatalf("prune trace history: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, old)); err == nil {
		t.Fatalf("old segment should be pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, cur)); err != nil {
		t.Fatalf("current segment should be kept: %v", err)
	}
}
