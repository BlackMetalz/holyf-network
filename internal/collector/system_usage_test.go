package collector

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCalculateCPUCores(t *testing.T) {
	t.Parallel()

	prev := &CPUStats{ProcessMicros: 1_000_000, Timestamp: time.Unix(100, 0)}
	curr := CPUStats{ProcessMicros: 2_800_000, Timestamp: time.Unix(103, 0)}

	got, ok := CalculateCPUCores(curr, prev)
	if !ok {
		t.Fatalf("expected CPU cores to be ready")
	}

	if got < 0.599 || got > 0.601 {
		t.Fatalf("unexpected CPU cores: got=%v want~0.6", got)
	}
}

func TestRusageCPUTimeMicros(t *testing.T) {
	t.Parallel()

	usage := unix.Rusage{
		Utime: unix.Timeval{Sec: 1, Usec: 250000},
		Stime: unix.Timeval{Sec: 0, Usec: 750000},
	}
	if got := rusageCPUTimeMicros(usage); got != 2_000_000 {
		t.Fatalf("rusageCPUTimeMicros: got=%d want=%d", got, 2_000_000)
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
