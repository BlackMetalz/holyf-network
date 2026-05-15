package panels

import (
	"fmt"
	"strings"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

// sparkChars maps values 0-7 to Unicode block elements for sparkline rendering.
var sparkChars = []rune("▁▂▃▄▅▆▇█")

// RenderSparkline converts a slice of values into a Unicode sparkline string.
// Width determines how many characters to use. If len(values) > width, only the
// most recent values are used. If len(values) < width, the sparkline is right-aligned
// with spaces on the left.
func RenderSparkline(values []float64, width int) string {
	if len(values) == 0 || width < 1 {
		return strings.Repeat(" ", width)
	}

	// Use only the most recent `width` values.
	if len(values) > width {
		values = values[len(values)-width:]
	}

	// Find max for auto-scaling.
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}

	// Build sparkline characters.
	var sb strings.Builder
	// Pad left if fewer values than width.
	if len(values) < width {
		sb.WriteString(strings.Repeat(" ", width-len(values)))
	}
	for _, v := range values {
		idx := 0
		if maxVal > 0 && v > 0 {
			idx = int(v / maxVal * float64(len(sparkChars)-1))
			if idx >= len(sparkChars) {
				idx = len(sparkChars) - 1
			}
		}
		sb.WriteRune(sparkChars[idx])
	}
	return sb.String()
}

// RenderBandwidthChart renders a compact bandwidth sparkline panel with RX and TX lines.
func RenderBandwidthChart(rxBuffer, txBuffer *tuishared.RingBuffer, panelWidth int) string {
	var sb strings.Builder

	rxValues := rxBuffer.Values()
	txValues := txBuffer.Values()
	rxLast, rxOk := rxBuffer.Last()
	txLast, txOk := txBuffer.Last()

	// Reserve space: "  RX " (5) + sparkline + "  " (2) + rate (10) = 17 fixed chars
	sparkWidth := panelWidth - 19
	if sparkWidth < 10 {
		sparkWidth = 10
	}

	// Window label
	window := rxBuffer.Count()
	if txBuffer.Count() > window {
		window = txBuffer.Count()
	}
	sb.WriteString(fmt.Sprintf("  [dim]── Bandwidth (%ds) ──[white]\n", window))

	// RX line
	rxRate := "warming"
	if rxOk {
		rxRate = formatBandwidthRate(rxLast)
	}
	rxSparkline := RenderSparkline(rxValues, sparkWidth)
	sb.WriteString(fmt.Sprintf("  [green]RX[white] %s  [green]%s[white]\n", rxSparkline, rxRate))

	// TX line
	txRate := "warming"
	if txOk {
		txRate = formatBandwidthRate(txLast)
	}
	txSparkline := RenderSparkline(txValues, sparkWidth)
	sb.WriteString(fmt.Sprintf("  [aqua]TX[white] %s  [aqua]%s[white]\n", txSparkline, txRate))

	// Peak info
	rxPeak := peakValue(rxValues)
	txPeak := peakValue(txValues)
	if rxPeak > 0 || txPeak > 0 {
		sb.WriteString(fmt.Sprintf("  [dim]peak: RX %s  TX %s[white]\n", formatBandwidthRate(rxPeak), formatBandwidthRate(txPeak)))
	}

	return sb.String()
}

func peakValue(values []float64) float64 {
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func formatBandwidthRate(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1_000_000_000:
		return fmt.Sprintf("%.1f GB/s", bytesPerSec/1_000_000_000)
	case bytesPerSec >= 1_000_000:
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/1_000_000)
	case bytesPerSec >= 1_000:
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1_000)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}
