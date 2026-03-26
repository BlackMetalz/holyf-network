package shared

import (
	"fmt"
	"strings"
)

func FormatNumber(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	if n < 1000 {
		if neg {
			return fmt.Sprintf("-%d", n)
		}
		return fmt.Sprintf("%d", n)
	}
	parts := make([]byte, 0, 16)
	for n >= 1000 {
		chunk := n % 1000
		parts = append([]byte(fmt.Sprintf(",%03d", chunk)), parts...)
		n /= 1000
	}
	parts = append([]byte(fmt.Sprintf("%d", n)), parts...)
	if neg {
		return "-" + string(parts)
	}
	return string(parts)
}

func RenderBar(count, maxCount, width int) string {
	if width <= 0 {
		width = 1
	}
	if maxCount <= 0 || count <= 0 {
		return fmt.Sprintf("[dim]%s[white]", strings.Repeat("░", width))
	}
	filled := int(float64(count) / float64(maxCount) * float64(width))
	if filled <= 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return fmt.Sprintf("[green]%s[dim]%s[white]", strings.Repeat("█", filled), strings.Repeat("░", width-filled))
}
