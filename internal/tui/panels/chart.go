package panels

import (
	"fmt"
	"math"
	"strings"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

// Braille dot patterns for plotting. Each Braille character is a 2x4 dot matrix.
// Dot positions:
//
//	col 0   col 1     bits:
//	row 0   row 0     0x01  0x08
//	row 1   row 1     0x02  0x10
//	row 2   row 2     0x04  0x20
//	row 3   row 3     0x40  0x80
//
// Unicode Braille: U+2800 + dot_pattern
const brailleBase = 0x2800

var brailleDots = [4][2]rune{
	{0x01, 0x08}, // row 0 (top)
	{0x02, 0x10}, // row 1
	{0x04, 0x20}, // row 2
	{0x40, 0x80}, // row 3 (bottom)
}

// RenderTimeSeriesChart renders a time-series chart with Y-axis, X-axis,
// and continuous Braille line plotting.
func RenderTimeSeriesChart(
	buffer *tuishared.RingBuffer,
	title string,
	width, height int,
	color string,
) string {
	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}

	values := buffer.Values()
	current, hasValue := buffer.Last()

	var sb strings.Builder

	// Title line with current value
	currentLabel := "warming..."
	if hasValue {
		currentLabel = formatBandwidthRate(current)
	}
	sb.WriteString(fmt.Sprintf("  [%s]%s[white]  [dim]current:[white] [%s]%s[white]\n\n", color, title, color, currentLabel))

	if len(values) == 0 {
		sb.WriteString("  [dim]Collecting samples...[white]\n")
		return sb.String()
	}

	// Chart area dimensions
	yLabelWidth := 8
	chartWidth := width - yLabelWidth - 2
	chartHeight := height - 5 // title(2) + x-axis(2) + margin(1)
	if chartWidth < 10 {
		chartWidth = 10
	}
	if chartHeight < 3 {
		chartHeight = 3
	}

	// Braille resolution
	dotsWide := chartWidth * 2
	dotsTall := chartHeight * 4

	// Find max for Y-axis scaling
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	maxVal = niceMax(maxVal)

	// Build Braille canvas
	canvas := make([][]rune, chartHeight)
	for r := range canvas {
		canvas[r] = make([]rune, chartWidth)
	}

	setDot := func(dotX, dotY int) {
		if dotX < 0 || dotX >= dotsWide || dotY < 0 || dotY >= dotsTall {
			return
		}
		charCol := dotX / 2
		subCol := dotX % 2
		invertedDotY := (dotsTall - 1) - dotY
		charRow := invertedDotY / 4
		subRow := invertedDotY % 4
		if charRow >= 0 && charRow < chartHeight && charCol >= 0 && charCol < chartWidth {
			canvas[charRow][charCol] |= brailleDots[subRow][subCol]
		}
	}

	// Map each real data point to a dot-X position, then draw lines between them.
	// This ensures the line is continuous regardless of how many samples we have.
	n := len(values)
	for i := 0; i < n; i++ {
		// Map sample index to dot X position (right-aligned: last sample = rightmost)
		dotX := 0
		if n > 1 {
			dotX = i * (dotsWide - 1) / (n - 1)
		} else {
			dotX = dotsWide - 1
		}

		// Map value to dot Y position (0=bottom, dotsTall-1=top)
		dotY := int(values[i] / maxVal * float64(dotsTall-1))
		if dotY < 0 {
			dotY = 0
		}
		if dotY >= dotsTall {
			dotY = dotsTall - 1
		}

		setDot(dotX, dotY)

		// Draw line from previous point to this point
		if i > 0 {
			prevDotX := 0
			if n > 1 {
				prevDotX = (i - 1) * (dotsWide - 1) / (n - 1)
			}
			prevDotY := int(values[i-1] / maxVal * float64(dotsTall-1))
			if prevDotY < 0 {
				prevDotY = 0
			}
			if prevDotY >= dotsTall {
				prevDotY = dotsTall - 1
			}
			bresenhamLine(prevDotX, prevDotY, dotX, dotY, setDot)
		}
	}

	// Render chart rows with Y-axis labels
	for row := 0; row < chartHeight; row++ {
		if row == 0 {
			sb.WriteString(fmt.Sprintf("  %6s│", formatAxisValue(maxVal)))
		} else if row == chartHeight-1 {
			sb.WriteString(fmt.Sprintf("  %6s│", "0"))
		} else if row == chartHeight/2 {
			sb.WriteString(fmt.Sprintf("  %6s│", formatAxisValue(maxVal/2)))
		} else {
			sb.WriteString("        │")
		}

		for col := 0; col < chartWidth; col++ {
			ch := canvas[row][col]
			if ch == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteString(fmt.Sprintf("[%s]%c[white]", color, brailleBase+ch))
			}
		}
		sb.WriteString("\n")
	}

	// X-axis line
	sb.WriteString("        └")
	sb.WriteString(strings.Repeat("─", chartWidth))
	sb.WriteString("\n")

	// X-axis time labels
	sb.WriteString("        ")
	totalSecs := n
	if totalSecs < 1 {
		totalSecs = 1
	}
	labelCount := 4
	if chartWidth < 30 {
		labelCount = 2
	}
	xLabels := make([]string, chartWidth+1)
	for i := 0; i <= labelCount; i++ {
		pos := i * chartWidth / labelCount
		if pos >= len(xLabels) {
			pos = len(xLabels) - 1
		}
		secAgo := totalSecs - (i * totalSecs / labelCount)
		if secAgo <= 0 {
			xLabels[pos] = "now"
		} else {
			xLabels[pos] = fmt.Sprintf("-%ds", secAgo)
		}
	}
	col := 0
	for col <= chartWidth && col < len(xLabels) {
		if xLabels[col] != "" {
			sb.WriteString(xLabels[col])
			col += len(xLabels[col])
		} else {
			sb.WriteString(" ")
			col++
		}
	}
	sb.WriteString("\n")

	return sb.String()
}

// bresenhamLine draws a line between two points, calling setDot for each pixel.
func bresenhamLine(x0, y0, x1, y1 int, setDot func(x, y int)) {
	dx := x1 - x0
	dy := y1 - y0
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}

	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}

	err := dx - dy
	for {
		setDot(x0, y0)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// niceMax rounds up to a "nice" number for Y-axis labels.
func niceMax(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(v))
	base := math.Pow(10, exp)
	niceSteps := []float64{1, 2, 5, 10}
	for _, s := range niceSteps {
		nice := s * base
		if nice >= v {
			return nice
		}
	}
	return v
}

// formatAxisValue formats a byte rate for Y-axis label (max 6 chars).
func formatAxisValue(v float64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.0fGB", v/1_000_000_000)
	case v >= 1_000_000:
		return fmt.Sprintf("%.0fMB", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.0fKB", v/1_000)
	default:
		return fmt.Sprintf("%.0fB", v)
	}
}

// resampleValues resamples input values to target count using nearest-neighbor.
// Kept for sparkline use (panels/sparkline.go).
func resampleValues(values []float64, targetCount int) []float64 {
	if len(values) == 0 || targetCount <= 0 {
		return nil
	}
	if len(values) <= targetCount {
		result := make([]float64, targetCount)
		offset := targetCount - len(values)
		copy(result[offset:], values)
		return result
	}
	result := make([]float64, targetCount)
	for i := 0; i < targetCount; i++ {
		srcIdx := i * len(values) / targetCount
		if srcIdx >= len(values) {
			srcIdx = len(values) - 1
		}
		result[i] = values[srcIdx]
	}
	return result
}
