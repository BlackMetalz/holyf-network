package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// panel_conntrack.go — Renders the Conntrack panel content.
// Focuses on usage pressure, drops, and warning thresholds.

// renderConntrackPanel formats conntrack data for the TUI panel.
func renderConntrackPanel(rates collector.ConntrackRates) string {
	var sb strings.Builder

	// Usage: "Used: 45,231 / 262,144 (17%)"
	if rates.Max > 0 {
		sb.WriteString(fmt.Sprintf("  [bold]Used:[white] %s / %s (%s)\n\n",
			formatNumber(rates.Current),
			formatNumber(rates.Max),
			formatConntrackPercentDetailed(rates.UsagePercent),
		))

		// Visual usage bar (Story 6.2)
		bar := renderUsageBar(rates.UsagePercent)
		sb.WriteString(fmt.Sprintf("  %s\n\n", bar))
	} else {
		sb.WriteString(fmt.Sprintf("  [bold]Used:[white] %s\n\n", formatNumber(rates.Current)))
	}

	// Warning when near limit (Story 6.4)
	if rates.UsagePercent > 80 {
		sb.WriteString("  [red][WARN] Conntrack table > 80% full![white]\n")
		sb.WriteString("  [dim]Consider: sysctl net.netfilter.nf_conntrack_max[white]\n\n")
	} else if rates.UsagePercent > 50 {
		sb.WriteString("  [yellow]Conntrack usage above 50%[white]\n\n")
	}

	// Stats availability
	if !rates.StatsAvailable {
		sb.WriteString("  [dim]Stats unavailable (install conntrack plz)[white]\n")
	} else if rates.FirstReading {
		sb.WriteString("  [dim]Rates available after next refresh[white]\n")
	}

	// Drops — any drops are BAD
	if !rates.StatsAvailable {
		sb.WriteString("\n  [dim]Drops: N/A[white]")
	} else if rates.TotalDrops > 0 {
		sb.WriteString(fmt.Sprintf("\n  [red]Drops: %s ⚠ (lost since boot)[white]",
			formatNumber(int(rates.TotalDrops))))
	}

	return sb.String()
}

// renderUsageBar creates a colored progress bar for conntrack usage.
// Color: green < 50%, yellow 50-80%, red > 80%.
func renderUsageBar(percent float64) string {
	const barWidth = 20

	filled := int(percent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

	// Pick color based on usage
	color := "green"
	if percent > 80 {
		color = "red"
	} else if percent > 50 {
		color = "yellow"
	}

	bar := fmt.Sprintf("[%s]%s[dim]%s[white] %s",
		color,
		strings.Repeat("█", filled),
		strings.Repeat("░", barWidth-filled),
		formatConntrackPercentDetailed(percent),
	)

	return bar
}
