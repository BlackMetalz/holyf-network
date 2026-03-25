package collector

import (
	"os"
	"testing"
	"time"
)

func TestCalculateCPUPercent(t *testing.T) {
	t.Parallel()

	prev := &CPUStats{ProcessTicks: 1000, Timestamp: time.Unix(100, 0)}
	curr := CPUStats{ProcessTicks: 1180, Timestamp: time.Unix(103, 0)}

	got, ok := CalculateCPUPercent(curr, prev)
	if !ok {
		t.Fatalf("expected CPU percent to be ready")
	}

	// delta = 180 ticks = 1.8 CPU seconds over 3 wall seconds => 60%
	if got < 59.99 || got > 60.01 {
		t.Fatalf("unexpected CPU percent: got=%v want~60", got)
	}
}

func TestParseCPUStats(t *testing.T) {
	t.Parallel()

	raw := "123 (holyf-network) S 1 2 3 4 5 6 7 8 9 10 120 80 14 15 16 17 18 19 20 21 22 23 24 25"
	ts := time.Unix(123, 0)
	stats, err := parseCPUStats(raw, ts)
	if err != nil {
		t.Fatalf("parseCPUStats error: %v", err)
	}

	if stats.ProcessTicks != 200 {
		t.Fatalf("ProcessTicks: got=%d want=200", stats.ProcessTicks)
	}
	if !stats.Timestamp.Equal(ts) {
		t.Fatalf("Timestamp: got=%v want=%v", stats.Timestamp, ts)
	}
}

func TestParseMemoryStats(t *testing.T) {
	t.Parallel()

	stats, err := parseMemoryStats("1024 256 0 0 0 0 0")
	if err != nil {
		t.Fatalf("parseMemoryStats error: %v", err)
	}

	want := uint64(256 * os.Getpagesize())
	if stats.RSSBytes != want {
		t.Fatalf("RSSBytes: got=%d want=%d", stats.RSSBytes, want)
	}
}
