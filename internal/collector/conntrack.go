package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// conntrack.go — Collects connection tracking (conntrack) statistics.
// Conntrack is the kernel's connection tracking system used for NAT, stateful firewall, etc.
//
// Data sources:
//   /proc/sys/net/netfilter/nf_conntrack_count — current entries
//   /proc/sys/net/netfilter/nf_conntrack_max   — maximum entries
//   /proc/net/stat/nf_conntrack                — insert/delete/drop counters

// ConntrackData holds conntrack table information.
type ConntrackData struct {
	Current      int     // Current number of tracked connections
	Max          int     // Maximum allowed
	UsagePercent float64 // Current/Max * 100

	// Counters from /proc/net/stat/nf_conntrack (cumulative since boot)
	Inserts int64 // Total new connections tracked
	Deletes int64 // Total connections removed
	Drops   int64 // Connections dropped (table full!)

	Timestamp time.Time
}

// ConntrackRates holds calculated per-second rates.
type ConntrackRates struct {
	Current      int
	Max          int
	UsagePercent float64

	InsertsPerSec float64 // New connections per second
	DeletesPerSec float64 // Destroyed connections per second
	DropsPerSec   float64 // Dropped per second (should be 0!)

	// Raw drop count — any drops are bad
	TotalDrops int64

	FirstReading bool
}

// CollectConntrack reads conntrack data from /proc.
func CollectConntrack() (ConntrackData, error) {
	data := ConntrackData{
		Timestamp: time.Now(),
	}

	// Read current count
	current, err := readSingleInt("/proc/sys/net/netfilter/nf_conntrack_count")
	if err != nil {
		return data, fmt.Errorf("conntrack not available: %w", err)
	}
	data.Current = current

	// Read max
	max, err := readSingleInt("/proc/sys/net/netfilter/nf_conntrack_max")
	if err != nil {
		// Not fatal — we can still show current count
		data.Max = 0
	} else {
		data.Max = max
	}

	// Calculate percentage
	if data.Max > 0 {
		data.UsagePercent = float64(data.Current) / float64(data.Max) * 100
	}

	// Read counters from /proc/net/stat/nf_conntrack
	data.Inserts, data.Deletes, data.Drops = readConntrackStats()

	return data, nil
}

// readSingleInt reads a file that contains a single integer.
func readSingleInt(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	value, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return 0, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	return value, nil
}

// readConntrackStats parses /proc/net/stat/nf_conntrack for insert/delete/drop counts.
//
// File format:
//
//	entries  searched found new    invalid ignore delete delete_list insert insert_failed drop ...
//	45231    12345678 1234  5678   0       1234   5600   0           5678   0             0
//
// We sum across all CPU lines (there's one line per CPU).
func readConntrackStats() (inserts, deletes, drops int64) {
	content, err := os.ReadFile("/proc/net/stat/nf_conntrack")
	if err != nil {
		return 0, 0, 0
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // Skip header
		}

		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		// Field indices (0-based, hex values):
		// [8] = insert, [6] = delete, [10] = drop
		ins, _ := strconv.ParseInt(fields[8], 16, 64)
		del, _ := strconv.ParseInt(fields[6], 16, 64)
		drp, _ := strconv.ParseInt(fields[10], 16, 64)

		inserts += ins
		deletes += del
		drops += drp
	}

	return inserts, deletes, drops
}

// CalculateConntrackRates computes per-second rates from two snapshots.
func CalculateConntrackRates(current ConntrackData, previous *ConntrackData) ConntrackRates {
	rates := ConntrackRates{
		Current:      current.Current,
		Max:          current.Max,
		UsagePercent: current.UsagePercent,
		TotalDrops:   current.Drops,
	}

	if previous == nil {
		rates.FirstReading = true
		return rates
	}

	elapsed := current.Timestamp.Sub(previous.Timestamp).Seconds()
	if elapsed <= 0 {
		rates.FirstReading = true
		return rates
	}

	rates.InsertsPerSec = float64(current.Inserts-previous.Inserts) / elapsed
	rates.DeletesPerSec = float64(current.Deletes-previous.Deletes) / elapsed
	rates.DropsPerSec = float64(current.Drops-previous.Drops) / elapsed

	return rates
}
