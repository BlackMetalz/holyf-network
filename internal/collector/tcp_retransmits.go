package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// tcp_retransmits.go — Parses /proc/net/snmp for TCP retransmit counters.
//
// What are retransmits?
// When a TCP packet is lost, the sender retransmits it.
// High retransmit rate = packet loss, congestion, or slow server.
//
// Data source: /proc/net/snmp
// Format: header line, then values line:
//   Tcp: RtoAlgorithm RtoMin RtoMax MaxConn ... InSegs OutSegs RetransSegs InErrs OutRsts ...
//   Tcp: 1            200    120000  -1     ... 12345  67890   42          0      0

// RetransmitData holds TCP retransmit counters.
type RetransmitData struct {
	RetransSegs int64 // Cumulative retransmitted segments since boot
	OutSegs     int64 // Cumulative outgoing segments since boot
	InSegs      int64 // Cumulative incoming segments since boot
	Timestamp   time.Time
}

// RetransmitRates holds calculated per-second rates.
type RetransmitRates struct {
	RetransPerSec float64 // Retransmits per second
	OutSegsPerSec float64 // Outgoing segments per second (for ratio)

	// Retransmit ratio: RetransSegs / OutSegs * 100
	// < 1% = normal, 1-5% = warning, > 5% = bad
	RetransPercent float64

	TotalRetrans int64 // Cumulative count
	FirstReading bool
}

// CollectRetransmits reads TCP retransmit data from /proc/net/snmp.
func CollectRetransmits() (RetransmitData, error) {
	data := RetransmitData{
		Timestamp: time.Now(),
	}

	content, err := os.ReadFile("/proc/net/snmp")
	if err != nil {
		return data, fmt.Errorf("cannot read /proc/net/snmp: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Find the Tcp header and values lines
	var headerFields, valueFields []string
	for i, line := range lines {
		if strings.HasPrefix(line, "Tcp:") {
			if headerFields == nil {
				headerFields = strings.Fields(line)
			} else {
				valueFields = strings.Fields(lines[i])
				break
			}
			// The next "Tcp:" line is the values
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "Tcp:") {
				valueFields = strings.Fields(lines[i+1])
				break
			}
		}
	}

	if headerFields == nil || valueFields == nil {
		return data, fmt.Errorf("TCP stats not found in /proc/net/snmp")
	}

	// Find column indices by matching header names
	for i, header := range headerFields {
		if i >= len(valueFields) {
			break
		}
		switch header {
		case "RetransSegs":
			data.RetransSegs, _ = strconv.ParseInt(valueFields[i], 10, 64)
		case "OutSegs":
			data.OutSegs, _ = strconv.ParseInt(valueFields[i], 10, 64)
		case "InSegs":
			data.InSegs, _ = strconv.ParseInt(valueFields[i], 10, 64)
		}
	}

	return data, nil
}

// CalculateRetransmitRates computes retransmit rate from two snapshots.
func CalculateRetransmitRates(current RetransmitData, previous *RetransmitData) RetransmitRates {
	rates := RetransmitRates{
		TotalRetrans: current.RetransSegs,
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

	rates.RetransPerSec = float64(current.RetransSegs-previous.RetransSegs) / elapsed
	rates.OutSegsPerSec = float64(current.OutSegs-previous.OutSegs) / elapsed

	// Calculate retransmit ratio
	deltaOut := current.OutSegs - previous.OutSegs
	deltaRetrans := current.RetransSegs - previous.RetransSegs
	if deltaOut > 0 {
		rates.RetransPercent = float64(deltaRetrans) / float64(deltaOut) * 100
	}

	return rates
}
