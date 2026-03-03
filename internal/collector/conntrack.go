package collector

import (
	"fmt"
	"os"
	"os/exec"
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
//   conntrack -S                               — insert/drop counters (requires conntrack-tools)

// ConntrackData holds conntrack table information.
type ConntrackData struct {
	Current      int     // Current number of tracked connections
	Max          int     // Maximum allowed
	UsagePercent float64 // Current/Max * 100

	// Counters (cumulative since boot). -1 means unavailable.
	Inserts int64 // Total new connections tracked
	Drops   int64 // Connections dropped (table full!)

	StatsAvailable bool // Whether insert/drop counters were readable
	Timestamp      time.Time
}

// ConntrackRates holds calculated per-second rates.
type ConntrackRates struct {
	Current      int
	Max          int
	UsagePercent float64

	InsertsPerSec float64 // New connections per second
	DropsPerSec   float64 // Dropped per second (should be 0!)

	// Raw drop count — any drops are bad
	TotalDrops int64

	StatsAvailable bool // Whether rate data is available
	FirstReading   bool
}

// CollectConntrack reads conntrack data from /proc and conntrack command.
func CollectConntrack() (ConntrackData, error) {
	data := ConntrackData{
		Timestamp: time.Now(),
		Inserts:   -1,
		Drops:     -1,
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

	// Read counters from `conntrack -S` (requires conntrack-tools package)
	inserts, drops, ok := readConntrackCommand()
	if ok {
		data.Inserts = inserts
		data.Drops = drops
		data.StatsAvailable = true
	}

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

// readConntrackCommand runs `conntrack -S` to get insert/drop counters.
//
// Output format (one line per CPU):
//
//	cpu=0   found=0 invalid=20 insert=0 insert_failed=3 drop=204829 early_drop=13 ...
//	cpu=1   found=0 invalid=0  insert=0 insert_failed=0 drop=0      early_drop=0  ...
//
// We sum insert and drop across all CPUs.
// Returns (inserts, drops, ok). ok=false if conntrack command not found.
func readConntrackCommand() (inserts, drops int64, ok bool) {
	out, err := exec.Command("conntrack", "-S").Output()
	if err != nil {
		return 0, 0, false
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse key=value pairs
		fields := strings.Fields(line)
		for _, field := range fields {
			parts := strings.SplitN(field, "=", 2)
			if len(parts) != 2 {
				continue
			}

			val, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				continue
			}

			switch parts[0] {
			case "insert":
				inserts += val
			case "drop":
				drops += val
			}
		}
	}

	return inserts, drops, true
}

// CalculateConntrackRates computes per-second rates from two snapshots.
func CalculateConntrackRates(current ConntrackData, previous *ConntrackData) ConntrackRates {
	rates := ConntrackRates{
		Current:        current.Current,
		Max:            current.Max,
		UsagePercent:   current.UsagePercent,
		TotalDrops:     current.Drops,
		StatsAvailable: current.StatsAvailable,
	}

	if previous == nil || !current.StatsAvailable {
		rates.FirstReading = true
		return rates
	}

	elapsed := current.Timestamp.Sub(previous.Timestamp).Seconds()
	if elapsed <= 0 {
		rates.FirstReading = true
		return rates
	}

	rates.InsertsPerSec = float64(current.Inserts-previous.Inserts) / elapsed
	rates.DropsPerSec = float64(current.Drops-previous.Drops) / elapsed

	return rates
}
