package shared

import "fmt"

const TinyConntrackPercent = 0.1

func FormatConntrackPercentShort(percent float64) string {
	if percent <= 0 {
		return "0%"
	}
	if percent < TinyConntrackPercent {
		return fmt.Sprintf("<%.1f%%", TinyConntrackPercent)
	}
	if percent < 1 {
		return fmt.Sprintf("%.1f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

func FormatConntrackPercentDetailed(percent float64) string {
	if percent <= 0 {
		return "0.0%"
	}
	if percent < TinyConntrackPercent {
		return fmt.Sprintf("<%.1f%%", TinyConntrackPercent)
	}
	return fmt.Sprintf("%.1f%%", percent)
}
