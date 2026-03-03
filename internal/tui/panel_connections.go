package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

// panel_connections.go — Renders the Connection States panel content.
// Combines Stories 3.2 (bar graph), 3.3 (color coding), 3.4 (total count).

// Color thresholds for warning states.
// When a state's count exceeds these values, it gets colored.
var stateWarnings = map[string]struct {
	threshold int
	color     string
	reason    string
}{
	"TIME_WAIT":  {threshold: 1000, color: "yellow", reason: "port exhaustion risk"},
	"CLOSE_WAIT": {threshold: 100, color: "red", reason: "app not closing sockets"},
	"SYN_RECV":   {threshold: 100, color: "red", reason: "possible SYN flood"},
	"FIN_WAIT1":  {threshold: 100, color: "yellow", reason: "connection cleanup issues"},
}

// maxBarWidth is the maximum width of the bar graph in characters.
const maxBarWidth = 20

// renderConnectionsPanel formats connection state data for the TUI panel.
// retrans can be nil if retransmit data is unavailable.
func renderConnectionsPanel(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	conntrack *collector.ConntrackRates,
	thresholds config.HealthThresholds,
) string {
	sorted := data.SortedStates()

	// Find the maximum count for scaling bars
	maxCount := 0
	for _, s := range sorted {
		if s.Count > maxCount {
			maxCount = s.Count
		}
	}

	var sb strings.Builder
	sb.WriteString(renderHealthStrip(retrans, conntrack, thresholds))
	sb.WriteString("\n\n")

	if len(sorted) == 0 {
		sb.WriteString("  No connections found")
		return sb.String()
	}

	for _, s := range sorted {
		// Determine color based on warning thresholds
		color := "white"
		warning := ""
		if w, ok := stateWarnings[s.Name]; ok && s.Count > w.threshold {
			color = w.color
			warning = fmt.Sprintf(" [dim](%s)[white]", w.reason)
		}

		// Build the bar
		bar := renderBar(s.Count, maxCount, maxBarWidth)

		sb.WriteString(fmt.Sprintf("  [%s]%-12s %6s[white]  %s%s\n",
			color,
			s.Name,
			formatNumber(s.Count),
			bar,
			warning,
		))
	}

	// Total
	sb.WriteString(fmt.Sprintf("\n  [bold]Total: %s connections[white]", formatNumber(data.Total)))

	// TCP Retransmits section
	if retrans != nil {
		sb.WriteString("\n\n  [bold]── Retransmits ──[white]\n")
		if retrans.FirstReading {
			sb.WriteString("  [dim]Rates after next refresh[white]")
		} else {
			// Color by severity: < 1% green, 1-5% yellow, > 5% red
			color := "green"
			if retrans.RetransPercent > 5 {
				color = "red"
			} else if retrans.RetransPercent > 1 {
				color = "yellow"
			}

			sb.WriteString(fmt.Sprintf("  [%s]%.0f/sec (%.2f%%)[white]",
				color, retrans.RetransPerSec, retrans.RetransPercent))

			if retrans.RetransPercent > 5 {
				sb.WriteString(" [red]⚠ high loss![white]")
			}
		}
	}

	return sb.String()
}

type healthLevel int

const (
	healthUnknown healthLevel = iota
	healthOK
	healthWarn
	healthCrit
)

func renderHealthStrip(
	retrans *collector.RetransmitRates,
	conntrack *collector.ConntrackRates,
	thresholds config.HealthThresholds,
) string {
	overall := healthUnknown

	retransValue := "n/a"
	retransColor := "dim"
	retransLevel := healthUnknown
	if retrans != nil && !retrans.FirstReading {
		retransLevel = classifyMetric(retrans.RetransPercent, thresholds.RetransPercent)
		retransColor = colorForHealthLevel(retransLevel)
		retransValue = fmt.Sprintf("%.1f%%", retrans.RetransPercent)
		overall = maxHealthLevel(overall, retransLevel)
	}

	dropsValue := "n/a"
	dropsColor := "dim"
	dropsLevel := healthUnknown
	if conntrack != nil && conntrack.StatsAvailable && !conntrack.FirstReading {
		drops := conntrack.DropsPerSec
		if drops < 0 {
			drops = 0
		}
		dropsLevel = classifyMetric(drops, thresholds.DropsPerSec)
		dropsColor = colorForHealthLevel(dropsLevel)
		dropsValue = fmt.Sprintf("%.0f/s", drops)
		overall = maxHealthLevel(overall, dropsLevel)
	}

	conntrackValue := "n/a"
	conntrackColor := "dim"
	conntrackLevel := healthUnknown
	if conntrack != nil && conntrack.Max > 0 {
		conntrackLevel = classifyMetric(conntrack.UsagePercent, thresholds.ConntrackPercent)
		conntrackColor = colorForHealthLevel(conntrackLevel)
		conntrackValue = fmt.Sprintf("%.0f%%", conntrack.UsagePercent)
		overall = maxHealthLevel(overall, conntrackLevel)
	}

	headLabel := "HEALTH UNKNOWN"
	headColor := "dim"
	switch overall {
	case healthOK:
		headLabel = "HEALTH OK"
		headColor = "green"
	case healthWarn:
		headLabel = "HEALTH WARN"
		headColor = "yellow"
	case healthCrit:
		headLabel = "HEALTH CRIT"
		headColor = "red"
	}

	return fmt.Sprintf(
		"  [%s]%s[white]  Retrans:[%s]%s[white] | Drops:[%s]%s[white] | Conntrack:[%s]%s[white]",
		headColor,
		headLabel,
		retransColor,
		retransValue,
		dropsColor,
		dropsValue,
		conntrackColor,
		conntrackValue,
	)
}

func classifyMetric(value float64, threshold config.ThresholdBand) healthLevel {
	if value >= threshold.Crit {
		return healthCrit
	}
	if value >= threshold.Warn {
		return healthWarn
	}
	return healthOK
}

func colorForHealthLevel(level healthLevel) string {
	switch level {
	case healthCrit:
		return "red"
	case healthWarn:
		return "yellow"
	case healthOK:
		return "green"
	default:
		return "dim"
	}
}

func maxHealthLevel(a, b healthLevel) healthLevel {
	if a > b {
		return a
	}
	return b
}

// renderBar creates a visual bar graph using block characters.
// value/maxValue determines the bar length, scaled to maxWidth.
func renderBar(value, maxValue, maxWidth int) string {
	if maxValue == 0 {
		return ""
	}

	width := (value * maxWidth) / maxValue
	if width == 0 && value > 0 {
		width = 1 // At least 1 char for non-zero values
	}

	filled := strings.Repeat("█", width)
	empty := strings.Repeat("░", maxWidth-width)

	return "[green]" + filled + "[dim]" + empty + "[white]"
}

// formatNumber adds comma separators to a number for readability.
// Example: 1234567 → "1,234,567"
func formatNumber(n int) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	// Insert commas from right to left
	var result strings.Builder
	for i, ch := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(ch)
	}
	return result.String()
}
