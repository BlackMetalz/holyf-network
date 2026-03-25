package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// CPUStats holds process CPU counters from getrusage(RUSAGE_SELF).
type CPUStats struct {
	ProcessMicros uint64
	Timestamp     time.Time
}

// MemoryStats holds process memory footprint.
type MemoryStats struct {
	RSSBytes uint64
}

// SystemUsage is the operator-facing CPU/memory snapshot.
type SystemUsage struct {
	CPUCores float64
	CPUReady bool
	Memory   MemoryStats
}

// CollectSystemUsage collects CPU and memory stats for the current process.
// CPU percentage requires a previous CPUStats snapshot for delta math.
func CollectSystemUsage(previousCPU *CPUStats) (SystemUsage, *CPUStats, error) {
	usage := SystemUsage{}

	cpu, err := CollectCPUStats()
	if err != nil {
		return usage, nil, err
	}
	mem, err := CollectMemoryStats()
	if err != nil {
		return usage, nil, err
	}

	usage.Memory = mem
	usage.CPUCores, usage.CPUReady = CalculateCPUCores(cpu, previousCPU)

	return usage, &cpu, nil
}

// CollectCPUStats reads process user+system CPU time for the current process.
func CollectCPUStats() (CPUStats, error) {
	var usage unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &usage); err != nil {
		return CPUStats{}, fmt.Errorf("cannot read process rusage: %w", err)
	}
	return CPUStats{
		ProcessMicros: rusageCPUTimeMicros(usage),
		Timestamp:     time.Now(),
	}, nil
}

// CollectMemoryStats reads /proc/self/statm and returns resident set size.
func CollectMemoryStats() (MemoryStats, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return MemoryStats{}, fmt.Errorf("cannot read /proc/self/statm: %w", err)
	}
	return parseMemoryStats(string(data))
}

// CalculateCPUCores calculates process CPU usage as logical CPU cores used between two snapshots.
// The result is process-local utilization, so multithreaded workloads may exceed 1.0 cores.
func CalculateCPUCores(current CPUStats, previous *CPUStats) (float64, bool) {
	if previous == nil {
		return 0, false
	}
	if current.ProcessMicros < previous.ProcessMicros {
		return 0, false
	}

	elapsedSeconds := current.Timestamp.Sub(previous.Timestamp).Seconds()
	if elapsedSeconds <= 0 {
		return 0, false
	}

	microsDelta := current.ProcessMicros - previous.ProcessMicros
	cpuSeconds := float64(microsDelta) / 1_000_000.0
	cores := cpuSeconds / elapsedSeconds
	if cores < 0 {
		cores = 0
	}
	return cores, true
}

func rusageCPUTimeMicros(usage unix.Rusage) uint64 {
	return timevalToMicros(usage.Utime) + timevalToMicros(usage.Stime)
}

func timevalToMicros(tv unix.Timeval) uint64 {
	sec := uint64(tv.Sec)
	usec := uint64(tv.Usec)
	return sec*1_000_000 + usec
}

func parseMemoryStats(raw string) (MemoryStats, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) < 2 {
		return MemoryStats{}, fmt.Errorf("short /proc/self/statm data")
	}

	rssPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return MemoryStats{}, fmt.Errorf("invalid rss in /proc/self/statm: %w", err)
	}

	return MemoryStats{
		RSSBytes: rssPages * uint64(os.Getpagesize()),
	}, nil
}
