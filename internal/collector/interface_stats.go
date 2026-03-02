package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// interface_stats.go — Collects network interface statistics from /sys/class/net/<iface>/statistics/
// Combines Stories 4.1 (RX/TX bytes), 4.2 (packet rate), 4.3 (errors/drops).

// InterfaceStats holds raw counters for a network interface.
// These are cumulative values since boot — to get rates, compare with previous snapshot.
type InterfaceStats struct {
	// Byte counters
	RxBytes int64
	TxBytes int64

	// Packet counters
	RxPackets int64
	TxPackets int64

	// Error/drop counters
	RxErrors  int64
	TxErrors  int64
	RxDropped int64
	TxDropped int64

	// When this snapshot was taken
	Timestamp time.Time
}

// InterfaceRates holds calculated per-second rates.
type InterfaceRates struct {
	RxBytesPerSec float64
	TxBytesPerSec float64
	RxPktsPerSec  float64
	TxPktsPerSec  float64

	// Cumulative error/drop counts (not rates — these are rare events)
	RxErrors  int64
	TxErrors  int64
	RxDropped int64
	TxDropped int64

	// Whether this is the first reading (no rate available yet)
	FirstReading bool
}

// CollectInterfaceStats reads counters from /sys/class/net/<iface>/statistics/.
// Each counter is a single number in its own file.
func CollectInterfaceStats(ifaceName string) (InterfaceStats, error) {
	basePath := fmt.Sprintf("/sys/class/net/%s/statistics", ifaceName)

	stats := InterfaceStats{
		Timestamp: time.Now(),
	}

	// Check if the interface directory exists
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return stats, fmt.Errorf("interface '%s' not found in /sys/class/net/ (requires Linux)", ifaceName)
	}

	var err error

	// Read each counter file. These files contain a single integer.
	stats.RxBytes, err = readStatFile(basePath, "rx_bytes")
	if err != nil {
		return stats, err
	}
	stats.TxBytes, _ = readStatFile(basePath, "tx_bytes")
	stats.RxPackets, _ = readStatFile(basePath, "rx_packets")
	stats.TxPackets, _ = readStatFile(basePath, "tx_packets")
	stats.RxErrors, _ = readStatFile(basePath, "rx_errors")
	stats.TxErrors, _ = readStatFile(basePath, "tx_errors")
	stats.RxDropped, _ = readStatFile(basePath, "rx_dropped")
	stats.TxDropped, _ = readStatFile(basePath, "tx_dropped")

	return stats, nil
}

// readStatFile reads a single integer from a /sys statistics file.
func readStatFile(basePath, fileName string) (int64, error) {
	data, err := os.ReadFile(fmt.Sprintf("%s/%s", basePath, fileName))
	if err != nil {
		return 0, fmt.Errorf("cannot read %s: %w", fileName, err)
	}

	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse %s: %w", fileName, err)
	}

	return value, nil
}

// CalculateRates computes per-second rates by comparing current and previous snapshots.
// If previous is nil (first reading), returns rates with FirstReading=true.
func CalculateRates(current InterfaceStats, previous *InterfaceStats) InterfaceRates {
	rates := InterfaceRates{
		RxErrors:  current.RxErrors,
		TxErrors:  current.TxErrors,
		RxDropped: current.RxDropped,
		TxDropped: current.TxDropped,
	}

	if previous == nil {
		rates.FirstReading = true
		return rates
	}

	// Calculate elapsed time in seconds
	elapsed := current.Timestamp.Sub(previous.Timestamp).Seconds()
	if elapsed <= 0 {
		rates.FirstReading = true
		return rates
	}

	// Calculate bytes/sec and packets/sec
	rates.RxBytesPerSec = float64(current.RxBytes-previous.RxBytes) / elapsed
	rates.TxBytesPerSec = float64(current.TxBytes-previous.TxBytes) / elapsed
	rates.RxPktsPerSec = float64(current.RxPackets-previous.RxPackets) / elapsed
	rates.TxPktsPerSec = float64(current.TxPackets-previous.TxPackets) / elapsed

	return rates
}
