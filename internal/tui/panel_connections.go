package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
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
// Returns a tview-compatible string with color tags.
func renderConnectionsPanel(data collector.ConnectionData) string {
	sorted := data.SortedStates()

	if len(sorted) == 0 {
		return "  No connections found"
	}

	// Find the maximum count for scaling bars
	maxCount := 0
	for _, s := range sorted {
		if s.Count > maxCount {
			maxCount = s.Count
		}
	}

	var sb strings.Builder

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

		// Format: "  ESTABLISHED  1,247  ████████████████████"
		sb.WriteString(fmt.Sprintf("  [%s]%-12s %6s[white]  %s%s\n",
			color,
			s.Name,
			formatNumber(s.Count),
			bar,
			warning,
		))
	}

	// Total at the bottom
	sb.WriteString(fmt.Sprintf("\n  [bold]Total: %s connections[white]", formatNumber(data.Total)))

	return sb.String()
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
