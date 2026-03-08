package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotWriterAppendWritesNDJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := NewSnapshotWriter(WriterConfig{
		DataDir:             dir,
		RetentionHours:      24,
		PruneEverySnapshots: 10,
	})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	ts := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	_, err = writer.Append(SnapshotRecord{
		CapturedAt: ts,
		Interface:  "eth0",
		TopLimit:   100,
		Groups: []SnapshotGroup{
			{
				PeerIP:     "198.51.100.1",
				LocalPort:  22,
				ProcName:   "sshd",
				ConnCount:  1,
				TxQueue:    10,
				RxQueue:    0,
				TotalQueue: 10,
				States:     map[string]int{"ESTABLISHED": 1},
			},
		},
		Version: "test",
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	segment := filepath.Join(dir, segmentFileName(ts))
	data, err := os.ReadFile(segment)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("segment should contain NDJSON line ending with newline")
	}

	var got SnapshotRecord
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatalf("decode ndjson line: %v", err)
	}
	if got.Interface != "eth0" || len(got.Groups) != 1 {
		t.Fatalf("unexpected record content: %#v", got)
	}
}

func TestSnapshotWriterRotatesByDay(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	t1 := time.Date(2026, 3, 4, 10, 15, 0, 0, time.UTC)
	t2 := t1.Add(25 * time.Hour)

	if _, err := writer.Append(SnapshotRecord{CapturedAt: t1}); err != nil {
		t.Fatalf("append t1: %v", err)
	}
	if _, err := writer.Append(SnapshotRecord{CapturedAt: t2}); err != nil {
		t.Fatalf("append t2: %v", err)
	}

	files, err := listSegmentFiles(dir)
	if err != nil {
		t.Fatalf("list segments: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 segment files, got %d", len(files))
	}
}

func TestSnapshotWriterPrunesByRetentionHours(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 1, PruneEverySnapshots: 1})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	old := now.Add(-27 * time.Hour)

	if _, err := writer.Append(SnapshotRecord{CapturedAt: old}); err != nil {
		t.Fatalf("append old: %v", err)
	}
	if _, err := writer.Append(SnapshotRecord{CapturedAt: now}); err != nil {
		t.Fatalf("append now: %v", err)
	}

	files, err := listSegmentFiles(dir)
	if err != nil {
		t.Fatalf("list segments: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 retained segment file, got %d", len(files))
	}
	if files[0].Name != segmentFileName(now) {
		t.Fatalf("unexpected retained segment: %s", files[0].Name)
	}
}

func TestSnapshotWriterAcquiresExclusiveLock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new first writer: %v", err)
	}
	defer first.Close()

	second, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err == nil {
		_ = second.Close()
		t.Fatalf("expected second writer lock acquisition to fail")
	}
}

func TestPruneDataDirByAgeHandlesMissingDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "missing")
	result, err := PruneDataDirByAge(dir, 24, time.Now())
	if err != nil {
		t.Fatalf("prune missing dir: %v", err)
	}
	if result.RemovedByAge != 0 {
		t.Fatalf("expected 0 removed files, got %d", result.RemovedByAge)
	}
}

func TestPruneDataDirByAgeRejectsInvalidRetention(t *testing.T) {
	t.Parallel()

	_, err := PruneDataDirByAge(t.TempDir(), 0, time.Now())
	if err == nil {
		t.Fatalf("expected error for retention-hours=0")
	}
}

func TestPruneDataDirByAgeKeepsCurrentSegment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 3, 8, 15, 0, 0, 0, time.Local)
	current := now
	old := now.AddDate(0, 0, -2)

	if err := os.WriteFile(filepath.Join(dir, segmentFileName(old)), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write old segment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, segmentFileName(current)), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write current segment: %v", err)
	}

	result, err := PruneDataDirByAge(dir, 24, now)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if result.RemovedByAge != 1 {
		t.Fatalf("expected 1 removed file, got %d", result.RemovedByAge)
	}
	if _, err := os.Stat(filepath.Join(dir, segmentFileName(current))); err != nil {
		t.Fatalf("current segment must be kept, stat err: %v", err)
	}
}

func TestSnapshotWriterPruneEverySnapshotsZeroDisablesPeriodicPrune(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := NewSnapshotWriter(WriterConfig{
		DataDir:             dir,
		RetentionHours:      1,
		PruneEverySnapshots: 0,
	})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.Local)
	staleDay := now.AddDate(0, 0, -2)
	stalePath := filepath.Join(dir, segmentFileName(staleDay))
	if err := os.WriteFile(stalePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale segment: %v", err)
	}

	if _, err := writer.Append(SnapshotRecord{CapturedAt: now}); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("stale segment should remain after rotate append when prune is disabled, stat err=%v", err)
	}

	// Inject a stale segment after first append. Second append is same-day, so
	// no rotate occurs; with PruneEverySnapshots=0 periodic prune must not run.
	if err := os.WriteFile(stalePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale segment: %v", err)
	}

	if _, err := writer.Append(SnapshotRecord{CapturedAt: now.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("append second: %v", err)
	}
	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("stale segment should remain when periodic prune is disabled, stat err=%v", err)
	}
}
