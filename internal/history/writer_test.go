package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

func TestSnapshotWriterAppendWritesNDJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := NewSnapshotWriter(WriterConfig{
		DataDir:             dir,
		RetentionHours:      24,
		MaxFiles:            72,
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
		Connections: []collector.Connection{
			{LocalIP: "10.0.0.1", LocalPort: 22, RemoteIP: "198.51.100.1", RemotePort: 12345, State: "ESTABLISHED", Activity: 10},
		},
		Version: "v1",
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
	if got.Interface != "eth0" || len(got.Connections) != 1 {
		t.Fatalf("unexpected record content: %#v", got)
	}
}

func TestSnapshotWriterRotatesByDay(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 24, MaxFiles: 72, PruneEverySnapshots: 10})
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
	writer, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 1, MaxFiles: 72, PruneEverySnapshots: 1})
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

func TestSnapshotWriterPrunesByMaxFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 240, MaxFiles: 2, PruneEverySnapshots: 1})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := writer.Append(SnapshotRecord{CapturedAt: base.AddDate(0, 0, i)}); err != nil {
			t.Fatalf("append #%d: %v", i, err)
		}
	}

	files, err := listSegmentFiles(dir)
	if err != nil {
		t.Fatalf("list segments: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 retained segment files, got %d", len(files))
	}
	if files[0].Name != segmentFileName(base.AddDate(0, 0, 1)) || files[1].Name != segmentFileName(base.AddDate(0, 0, 2)) {
		t.Fatalf("expected to keep latest two segments, got: %s, %s", files[0].Name, files[1].Name)
	}
}

func TestSnapshotWriterAcquiresExclusiveLock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 24, MaxFiles: 72, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new first writer: %v", err)
	}
	defer first.Close()

	second, err := NewSnapshotWriter(WriterConfig{DataDir: dir, RetentionHours: 24, MaxFiles: 72, PruneEverySnapshots: 10})
	if err == nil {
		_ = second.Close()
		t.Fatalf("expected second writer lock acquisition to fail")
	}
}
