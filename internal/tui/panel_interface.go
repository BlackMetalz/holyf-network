package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// panel_interface.go — Renders the Interface Stats panel content.
// Combines Stories 4.1 (RX/TX bytes/sec), 4.2 (packet rate), 4.3 (errors/drops).

// renderInterfacePanel formats interface stats for the TUI panel.
func renderInterfacePanel(rates collector.InterfaceRates) string {
	var sb strings.Builder

	if rates.FirstReading {
		sb.WriteString("  [yellow]Collecting baseline...[white]\n")
		sb.WriteString("  [dim]Rates available after next refresh (press r)[white]\n\n")
	} else {
		// RX/TX throughput
		sb.WriteString(fmt.Sprintf("  [bold]RX:[white] %-12s  [bold]TX:[white] %s\n",
			formatBytesRate(rates.RxBytesPerSec),
			formatBytesRate(rates.TxBytesPerSec),
		))
		sb.WriteString("\n")

		// Packet rates
		sb.WriteString(fmt.Sprintf("  [bold]Packets:[white] %s RX, %s TX\n",
			formatPacketRate(rates.RxPktsPerSec),
			formatPacketRate(rates.TxPktsPerSec),
		))
		sb.WriteString("\n")
	}

	// Errors and drops (cumulative, always shown)
	errColor := "green"
	if rates.RxErrors+rates.TxErrors > 0 {
		errColor = "red"
	}
	dropColor := "green"
	if rates.RxDropped+rates.TxDropped > 0 {
		dropColor = "red"
	}

	sb.WriteString(fmt.Sprintf("  [%s]Errors: %d RX, %d TX[white]",
		errColor, rates.RxErrors, rates.TxErrors,
	))
	if rates.RxErrors+rates.TxErrors > 0 {
		sb.WriteString(" ⚠")
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("  [%s]Drops:  %d RX, %d TX[white]",
		dropColor, rates.RxDropped, rates.TxDropped,
	))
	if rates.RxDropped+rates.TxDropped > 0 {
		sb.WriteString(" ⚠")
	}
	sb.WriteString("\n")

	return sb.String()
}

// formatBytesRate converts bytes/sec to human-readable format.
// Auto-scales: B/s → KB/s → MB/s → GB/s
func formatBytesRate(bytesPerSec float64) string {
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	idx := 0

	value := bytesPerSec
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}

	if idx == 0 {
		return fmt.Sprintf("%.0f %s", value, units[idx])
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}

// formatPacketRate converts packets/sec to human-readable format.
// Uses k/M suffixes for large numbers.
func formatPacketRate(pktsPerSec float64) string {
	if pktsPerSec >= 1_000_000 {
		return fmt.Sprintf("%.1fM/s", pktsPerSec/1_000_000)
	}
	if pktsPerSec >= 1_000 {
		return fmt.Sprintf("%.1fk/s", pktsPerSec/1_000)
	}
	return fmt.Sprintf("%.0f/s", pktsPerSec)
}
