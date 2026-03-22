package tui

import "fmt"

const tinyConntrackPercent = 0.1

func formatConntrackPercentShort(percent float64) string {
	if percent <= 0 {
		return "0%"
	}
	if percent < tinyConntrackPercent {
		return fmt.Sprintf("<%.1f%%", tinyConntrackPercent)
	}
	if percent < 1 {
		return fmt.Sprintf("%.1f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

func formatConntrackPercentDetailed(percent float64) string {
	if percent <= 0 {
		return "0.0%"
	}
	if percent < tinyConntrackPercent {
		return fmt.Sprintf("<%.1f%%", tinyConntrackPercent)
	}
	return fmt.Sprintf("%.1f%%", percent)
}
