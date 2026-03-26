package panels

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

// panel_connections.go — Renders the Connection States panel content.
// Combines Stories 3.2 (bar graph), 3.3 (color coding), 3.4 (total count).


// maxBarWidth is the maximum width of the bar graph in characters.
const maxBarWidth = 20

// renderConnectionsPanel formats connection state data for the TUI panel.
// retrans can be nil if retransmit data is unavailable.
func RenderConnectionsPanel(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	conntrack *collector.ConntrackRates,
	thresholds config.HealthThresholds,
) string {
	return RenderConnectionsPanelWithStateSort(data, retrans, conntrack, thresholds, true)
}

func RenderConnectionsPanelWithStateSort(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	conntrack *collector.ConntrackRates,
	thresholds config.HealthThresholds,
	sortDesc bool,
) string {
	sorted := data.SortedStates()
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			if sortDesc {
				return sorted[i].Count > sorted[j].Count
			}
			return sorted[i].Count < sorted[j].Count
		}
		return sorted[i].Name < sorted[j].Name
	})

	// Find the maximum count for scaling bars
	maxCount := 0
	for _, s := range sorted {
		if s.Count > maxCount {
			maxCount = s.Count
		}
	}

	var sb strings.Builder
	sb.WriteString(RenderHealthStrip(data, retrans, conntrack, thresholds))
	sb.WriteString("\n\n")

	if len(sorted) == 0 {
		sb.WriteString("  No connections found")
		return sb.String()
	}

	for _, s := range sorted {
		// Determine color based on warning thresholds
		color := "white"
		warning := ""
		if w, ok := tuishared.StateWarnings[s.Name]; ok && s.Count > w.Threshold {
			color = w.Color
			warning = fmt.Sprintf(" [dim](%s)[white]", w.Reason)
		}

		// Build the bar
		bar := tuishared.RenderBar(s.Count, maxCount, maxBarWidth)

		sb.WriteString(fmt.Sprintf("  [%s]%-12s %6s[white]  %s%s\n",
			color,
			s.Name,
			tuishared.FormatNumber(s.Count),
			bar,
			warning,
		))
	}

	// Total
	sb.WriteString(fmt.Sprintf("\n  [bold]Total: %s connections[white]", tuishared.FormatNumber(data.Total)))

	// TCP Retransmits section
	if retrans != nil {
		sb.WriteString("\n\n  [bold]── Retransmits ──[white]\n")
		if retrans.FirstReading {
			sb.WriteString("  [dim]Rates after next refresh[white]")
		} else {
			sample := tuishared.EvaluateRetransSample(data, retrans, thresholds)
			if !sample.Ready {
				sb.WriteString(fmt.Sprintf(
					"  [dim]LOW SAMPLE (est %d/%d, out %.1f/%.1f seg/s)[white]",
					sample.Established,
					sample.MinEstablished,
					sample.OutSegsPerSec,
					sample.MinOutSegsPerSec,
				))
			} else {
				level := tuishared.ClassifyMetric(retrans.RetransPercent, thresholds.RetransPercent)
				color := tuishared.ColorForHealthLevel(level)
				sb.WriteString(fmt.Sprintf("  [%s]%.1f/sec (%.2f%%)[white]",
					color, retrans.RetransPerSec, retrans.RetransPercent))
				if level == tuishared.HealthCrit {
					sb.WriteString(" [red]⚠ high loss![white]")
				}
			}
		}
	}

	return sb.String()
}

func RenderHealthStrip(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	conntrack *collector.ConntrackRates,
	thresholds config.HealthThresholds,
) string {
	overall := tuishared.HealthUnknown

	retransValue := "n/a"
	retransColor := "dim"
	retransLevel := tuishared.HealthUnknown
	if retrans != nil && !retrans.FirstReading {
		sample := tuishared.EvaluateRetransSample(data, retrans, thresholds)
		if sample.Ready {
			retransLevel = tuishared.ClassifyMetric(retrans.RetransPercent, thresholds.RetransPercent)
			retransColor = tuishared.ColorForHealthLevel(retransLevel)
			retransValue = fmt.Sprintf("%.1f%%", retrans.RetransPercent)
			overall = tuishared.MaxHealthLevel(overall, retransLevel)
		} else {
			retransValue = "LOW SAMPLE"
		}
	}

	dropsValue := "n/a"
	dropsColor := "dim"
	dropsLevel := tuishared.HealthUnknown
	if conntrack != nil && conntrack.StatsAvailable && !conntrack.FirstReading {
		drops := conntrack.DropsPerSec
		if drops < 0 {
			drops = 0
		}
		dropsLevel = tuishared.ClassifyMetric(drops, thresholds.DropsPerSec)
		dropsColor = tuishared.ColorForHealthLevel(dropsLevel)
		dropsValue = fmt.Sprintf("%.0f/s", drops)
		overall = tuishared.MaxHealthLevel(overall, dropsLevel)
	}

	headLabel := "HEALTH UNKNOWN"
	headColor := "dim"
	switch overall {
	case tuishared.HealthOK:
		headLabel = "HEALTH OK"
		headColor = "green"
	case tuishared.HealthWarn:
		headLabel = "HEALTH WARN"
		headColor = "yellow"
	case tuishared.HealthCrit:
		headLabel = "HEALTH CRIT"
		headColor = "red"
	}

	return fmt.Sprintf(
		"  [%s]%s[white]  Retrans:[%s]%s[white] | Drops:[%s]%s[white]",
		headColor,
		headLabel,
		retransColor,
		retransValue,
		dropsColor,
		dropsValue,
	)
}
