package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadIndexOrdersSnapshotsAcrossFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	linesA := []string{
		`{"captured_at":"2026-03-04T10:00:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
	}
	linesB := []string{
		`{"captured_at":"2026-03-04T11:00:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
	}
	writeSegmentLines(t, dir, "connections-20260304-10.jsonl", linesA)
	writeSegmentLines(t, dir, "connections-20260304-11.jsonl", linesB)

	refs, stats, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if stats.Files != 2 {
		t.Fatalf("files count mismatch: got=%d want=2", stats.Files)
	}
	if len(refs) != 2 {
		t.Fatalf("ref count mismatch: got=%d want=2", len(refs))
	}
	if !refs[0].CapturedAt.Before(refs[1].CapturedAt) {
		t.Fatalf("refs should be sorted oldest -> latest")
	}
}

func TestReadSnapshotByOffset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	name := "connections-20260304-10.jsonl"
	lines := []string{
		`{"captured_at":"2026-03-04T10:00:00Z","interface":"eth0","top_limit":100,"connections":[{"local_ip":"10.0.0.1","local_port":22,"remote_ip":"198.51.100.1","remote_port":12345,"state":"ESTABLISHED","tx_queue":0,"rx_queue":0,"activity":10,"inode":"","pid":1,"proc_name":"sshd"}],"version":"v1"}`,
		`{"captured_at":"2026-03-04T10:01:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
	}
	writeSegmentLines(t, dir, name, lines)

	refs, _, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}

	record, err := ReadSnapshot(refs[1])
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if got := record.CapturedAt.UTC().Format(time.RFC3339); got != "2026-03-04T10:01:00Z" {
		t.Fatalf("captured_at mismatch: got=%s", got)
	}
	if len(record.Connections) != 0 {
		t.Fatalf("expected empty connection list for second snapshot")
	}
}

func TestLoadIndexSkipsCorruptLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSegmentLines(t, dir, "connections-20260304-10.jsonl", []string{
		`{"captured_at":"2026-03-04T10:00:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
		`{not-json}`,
		`{"captured_at":"2026-03-04T10:02:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
	})

	refs, stats, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if stats.Corrupt != 1 {
		t.Fatalf("corrupt count mismatch: got=%d want=1", stats.Corrupt)
	}
	if len(refs) != 2 {
		t.Fatalf("valid ref count mismatch: got=%d want=2", len(refs))
	}
}

func TestLoadIndexFromFileReadsOnlyTargetSegment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSegmentLines(t, dir, "connections-20260304-10.jsonl", []string{
		`{"captured_at":"2026-03-04T10:00:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
	})
	writeSegmentLines(t, dir, "connections-20260304-11.jsonl", []string{
		`{"captured_at":"2026-03-04T11:00:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
		`{"captured_at":"2026-03-04T11:01:00Z","interface":"eth0","top_limit":100,"connections":[],"version":"v1"}`,
	})

	refs, stats, err := LoadIndexFromFile(dir, "connections-20260304-11.jsonl")
	if err != nil {
		t.Fatalf("load index from file: %v", err)
	}
	if stats.Files != 1 {
		t.Fatalf("file count mismatch: got=%d want=1", stats.Files)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs from selected file, got=%d", len(refs))
	}
	if got := refs[0].CapturedAt.UTC().Format(time.RFC3339); got != "2026-03-04T11:00:00Z" {
		t.Fatalf("first captured_at mismatch: got=%s", got)
	}
}

func TestLoadIndexFromFileNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, _, err := LoadIndexFromFile(dir, "connections-20990101-00.jsonl"); err == nil {
		t.Fatalf("expected error for missing segment file")
	}
}

func writeSegmentLines(t *testing.T, dir, name string, lines []string) {
	t.Helper()
	path := filepath.Join(dir, name)
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write segment %s: %v", name, err)
	}
}

func TestParseSegmentTimeFromName(t *testing.T) {
	t.Parallel()

	timestamp, ok := parseSegmentTime("connections-20260304-10.jsonl")
	if !ok {
		t.Fatalf("expected valid segment name")
	}
	if got := timestamp.In(time.Local).Format("20060102-15"); got != "20260304-10" {
		t.Fatalf("timestamp mismatch: %s", got)
	}

	if _, ok := parseSegmentTime("bad-name"); ok {
		t.Fatalf("invalid filename should not parse")
	}
}

func TestExpandPathHome(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got := ExpandPath("~/tmp-abc")
	want := filepath.Join(home, "tmp-abc")
	if got != want {
		t.Fatalf("expand path mismatch: got=%s want=%s", got, want)
	}
}
