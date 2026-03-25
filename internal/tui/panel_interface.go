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
		sb.WriteString(fmt.Sprintf("  [bold]RX:[white] %-12s  [bold]TX:[white] %s\n", formatBytesRate(rates.RxBytesPerSec), formatBytesRate(rates.TxBytesPerSec)))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  [bold]Packet rate:[white] %s RX, %s TX\n", formatPacketRate(rates.RxPktsPerSec), formatPacketRate(rates.TxPktsPerSec)))
		sb.WriteString("\n")
		sb.WriteString(renderSpeedLine(spike))
		trafficLine := renderTrafficLine(spike)
		if trafficLine != "" {
			sb.WriteString("\n")
			sb.WriteString(trafficLine)
		}
		sb.WriteString("\n\n")
	}

	sb.WriteString(renderSystemUsageLine(sys))
	sb.WriteString("\n\n")

	errColor := "green"
	if rates.RxErrors+rates.TxErrors > 0 {
		errColor = "red"
	}
	dropColor := "green"
	if rates.RxDropped+rates.TxDropped > 0 {
		dropColor = "red"
	}

	sb.WriteString(fmt.Sprintf("  [%s]Errors: %d RX, %d TX[white]", errColor, rates.RxErrors, rates.TxErrors))
	if rates.RxErrors+rates.TxErrors > 0 {
		sb.WriteString(" ⚠")
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("  [%s]Drops:  %d RX, %d TX[white]", dropColor, rates.RxDropped, rates.TxDropped))
	if rates.RxDropped+rates.TxDropped > 0 {
		sb.WriteString(" ⚠")
	}
	sb.WriteString("\n")

	return sb.String()
}

func renderSpeedLine(spike interfaceSpikeAssessment) string {
	if spike.LinkSpeedKnown && spike.LinkSpeedBps > 0 {
		return fmt.Sprintf("  [bold]Speed:[white] %s  [dim](util %.1f%%)[white]", formatLinkSpeed(spike.LinkSpeedBps), spike.LinkUtilPercent)
	}
	return "  [bold]Speed:[white] [dim]unknown[white]"
}

func renderTrafficLine(spike interfaceSpikeAssessment) string {
	switch spike.DisplayLevel {
	case healthCrit:
		return "  [bold]Traffic:[white] [red]SPIKE CRIT[white]"
	case healthWarn:
		return "  [bold]Traffic:[white] [yellow]SPIKE WARN[white]"
	}
	return ""
}

func renderSystemUsageLine(sys interfaceSystemSnapshot) string {
	errText := strings.TrimSpace(sys.Err)
	if !sys.Ready {
		if errText != "" {
			return fmt.Sprintf("  [bold]App:[white] [yellow]unavailable[white] [dim](%s)[white]", errText)
		}
		return "  [bold]App:[white] [dim]CPU warming[white]"
	}

	cpuText := "warming"
	if sys.Usage.CPUReady {
		cpuText = formatCPUCores(sys.Usage.CPUCores)
	}

	memText := "n/a"
	if sys.Usage.Memory.RSSBytes > 0 {
		memText = formatMemoryBytes(sys.Usage.Memory.RSSBytes) + " RSS"
	}

	line := fmt.Sprintf("  [bold]App:[white] CPU %s | Mem %s", cpuText, memText)
	if errText != "" {
		line += fmt.Sprintf(" [yellow]stale (%s)[white]", errText)
	}
	return line
}

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

func formatCPUCores(cores float64) string {
	unit := "cores"
	if cores >= 0.995 && cores <= 1.005 {
		unit = "core"
	}
	return fmt.Sprintf("%.2f %s", cores, unit)
}
