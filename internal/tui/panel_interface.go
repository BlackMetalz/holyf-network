package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// panel_interface.go — Renders the Interface Stats panel content.
// Combines Stories 4.1 (RX/TX bytes/sec), 4.2 (packet rate), 4.3 (errors/drops).

type interfaceSystemSnapshot struct {
	Usage      collector.SystemUsage
	Ready      bool
	Err        string
	RefreshSec int
}

// renderInterfacePanel formats interface stats for the TUI panel.
func renderInterfacePanel(rates collector.InterfaceRates, spike interfaceSpikeAssessment, sys interfaceSystemSnapshot) string {
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

		spikeState := "warming baseline"
		spikeColor := "dim"
		switch spike.Level {
		case healthCrit:
			spikeState = "SPIKE CRIT"
			spikeColor = "red"
		case healthWarn:
			spikeState = "SPIKE WARN"
			spikeColor = "yellow"
		case healthOK:
			if spike.Ready {
				spikeState = "stable"
				spikeColor = "green"
			}
		}
		if spike.Ready && spike.Level == healthUnknown {
			spikeState = "stable"
			spikeColor = "green"
		}

		if spike.Ratio <= 0 {
			spike.Ratio = 1.0
		}
		linkInfo := "speed n/a"
		if spike.LinkSpeedKnown && spike.LinkSpeedBps > 0 {
			linkInfo = fmt.Sprintf("link %s, util %.1f%%", formatLinkSpeed(spike.LinkSpeedBps), spike.LinkUtilPercent)
		}
		sb.WriteString(fmt.Sprintf(
			"  [bold]Traffic:[white] [%s]%s[white]  [dim](%s profile | peak %s, base %s, x%.2f | %s)[white]\n\n",
			spikeColor,
			spikeState,
			spike.ProfileLabel,
			formatBytesRate(spike.PeakBytesPerSec),
			formatBytesRate(spike.BaselineBytesPerSec),
			spike.Ratio,
			linkInfo,
		))
	}

	sb.WriteString(renderSystemUsageLine(sys))
	sb.WriteString("\n\n")

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

func renderSystemUsageLine(sys interfaceSystemSnapshot) string {
	refreshSec := sys.RefreshSec
	if refreshSec <= 0 {
		refreshSec = 1
	}
	errText := strings.TrimSpace(sys.Err)

	if !sys.Ready {
		if errText != "" {
			return fmt.Sprintf("  [bold]App:[white] [yellow]unavailable[white] [dim](%s)[white]", errText)
		}
		return fmt.Sprintf("  [bold]App:[white] [dim]CPU warming[white] [dim](global %ds sample)[white]", refreshSec)
	}

	cpuText := "warming"
	if sys.Usage.CPUReady {
		cpuText = fmt.Sprintf("%.1f%%", sys.Usage.CPUPercent)
	}

	memText := "n/a"
	if sys.Usage.Memory.RSSBytes > 0 {
		memText = formatMemoryBytes(sys.Usage.Memory.RSSBytes) + " RSS"
	}

	line := fmt.Sprintf(
		"  [bold]App:[white] CPU %s | Mem %s [dim](global %ds sample)[white]",
		cpuText,
		memText,
		refreshSec,
	)
	if errText != "" {
		line += fmt.Sprintf(" [yellow]stale (%s)[white]", errText)
	}
	return line
}

// formatBytesRate converts bytes/sec to human-readable format.
// Auto-scales: B/s -> KB/s -> MB/s -> GB/s
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

func formatLinkSpeed(bytesPerSec float64) string {
	bitsPerSec := bytesPerSec * 8.0
	if bitsPerSec >= 1_000_000_000 {
		return fmt.Sprintf("%.1f Gb/s", bitsPerSec/1_000_000_000)
	}
	return fmt.Sprintf("%.0f Mb/s", bitsPerSec/1_000_000)
}

func formatMemoryBytes(bytes uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(bytes)
	idx := 0
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", bytes, units[idx])
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}
