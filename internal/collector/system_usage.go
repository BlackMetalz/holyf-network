package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const processClockTicksPerSecond = 100

// CPUStats holds process CPU counters from /proc/self/stat.
type CPUStats struct {
	ProcessTicks uint64
	Timestamp    time.Time
}

// MemoryStats holds process memory footprint.
type MemoryStats struct {
	RSSBytes uint64
}

// SystemUsage is the operator-facing CPU/memory snapshot.
type SystemUsage struct {
	CPUPercent float64
	CPUReady   bool
	Memory     MemoryStats
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
	usage.CPUPercent, usage.CPUReady = CalculateCPUPercent(cpu, previousCPU)

	return usage, &cpu, nil
}

// CollectCPUStats reads /proc/self/stat and parses utime+stime for this process.
func CollectCPUStats() (CPUStats, error) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return CPUStats{}, fmt.Errorf("cannot read /proc/self/stat: %w", err)
	}
	return parseCPUStats(string(data), time.Now())
}

// CollectMemoryStats reads /proc/self/statm and returns resident set size.
func CollectMemoryStats() (MemoryStats, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return MemoryStats{}, fmt.Errorf("cannot read /proc/self/statm: %w", err)
	}
	return parseMemoryStats(string(data))
}

// CalculateCPUPercent calculates process CPU % between two snapshots.
// The result is process-local utilization, so multithreaded workloads may exceed 100%.
func CalculateCPUPercent(current CPUStats, previous *CPUStats) (float64, bool) {
	if previous == nil {
		return 0, false
	}
	if current.ProcessTicks < previous.ProcessTicks {
		return 0, false
	}

	elapsedSeconds := current.Timestamp.Sub(previous.Timestamp).Seconds()
	if elapsedSeconds <= 0 {
		return 0, false
	}

	tickDelta := current.ProcessTicks - previous.ProcessTicks
	cpuSeconds := float64(tickDelta) / processClockTicksPerSecond
	percent := (cpuSeconds / elapsedSeconds) * 100.0
	if percent < 0 {
		percent = 0
	}
	return percent, true
}

func parseCPUStats(raw string, ts time.Time) (CPUStats, error) {
	raw = strings.TrimSpace(raw)
	closeIdx := strings.LastIndex(raw, ")")
	if closeIdx == -1 || closeIdx+2 >= len(raw) {
		return CPUStats{}, fmt.Errorf("invalid /proc/self/stat format")
	}

	fields := strings.Fields(raw[closeIdx+2:])
	if len(fields) <= 12 {
		return CPUStats{}, fmt.Errorf("short /proc/self/stat data")
	}

	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return CPUStats{}, fmt.Errorf("invalid utime in /proc/self/stat: %w", err)
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return CPUStats{}, fmt.Errorf("invalid stime in /proc/self/stat: %w", err)
	}

	return CPUStats{
		ProcessTicks: utime + stime,
		Timestamp:    ts,
	}, nil
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
