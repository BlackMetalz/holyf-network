package panels

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

func RenderConntrackPanel(rates collector.ConntrackRates, threshold config.ThresholdBand) string {
	var sb strings.Builder

	if rates.Max > 0 {
		sb.WriteString(fmt.Sprintf("  [bold]Used:[white] %s / %s (%s)\n\n",
			tuishared.FormatNumber(rates.Current),
			tuishared.FormatNumber(rates.Max),
			tuishared.FormatConntrackPercentDetailed(rates.UsagePercent),
		))

		bar := renderUsageBar(rates.UsagePercent)
		sb.WriteString(fmt.Sprintf("  %s\n\n", bar))
	} else {
		sb.WriteString(fmt.Sprintf("  [bold]Used:[white] %s\n\n", tuishared.FormatNumber(rates.Current)))
	}

	warnPct := threshold.Warn
	critPct := threshold.Crit
	if warnPct <= 0 {
		warnPct = 70
	}
	if critPct <= 0 {
		critPct = 85
	}

	if rates.UsagePercent >= critPct {
		sb.WriteString(fmt.Sprintf("  [red][WARN] Conntrack table >= %.0f%%[white]\n", critPct))
		sb.WriteString("  [dim]Consider: sysctl net.netfilter.nf_conntrack_max[white]\n\n")
	} else if rates.UsagePercent >= warnPct {
		sb.WriteString(fmt.Sprintf("  [yellow]Conntrack usage >= %.0f%%[white]\n\n", warnPct))
	}

	if !rates.StatsAvailable {
		sb.WriteString("  [dim]Stats unavailable (install conntrack plz)[white]\n")
	} else if rates.FirstReading {
		sb.WriteString("  [dim]Rates available after next refresh[white]\n")
	}

	if !rates.StatsAvailable {
		sb.WriteString("\n  [dim]Drops: N/A[white]")
	} else if rates.TotalDrops > 0 {
		sb.WriteString(fmt.Sprintf("\n  [red]Drops: %s ⚠ (lost since boot)[white]",
			tuishared.FormatNumber(int(rates.TotalDrops))))
	}

	return sb.String()
}

func renderUsageBar(percent float64) string {
	const barWidth = 20

	filled := int(percent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

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
		tuishared.FormatConntrackPercentDetailed(percent),
	)

	return bar
}
