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
//	(0,0) (1,0)     bits: 0x01 0x08
//	(0,1) (1,1)           0x02 0x10
//	(0,2) (1,2)           0x04 0x20
//	(0,3) (1,3)           0x40 0x80
//
// Unicode Braille: U+2800 + dot_pattern
const brailleBase = 0x2800

var brailleDots = [4][2]rune{
	{0x01, 0x08}, // row 0 (top)
	{0x02, 0x10}, // row 1
	{0x04, 0x20}, // row 2
	{0x40, 0x80}, // row 3 (bottom)
}

// RenderTimeSeriesChart renders a time-series chart with Y-axis labels,
// X-axis time labels, and Braille dot plotting.
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
	yLabelWidth := 8 // "  10 KB│" = 8 chars
	xLabelHeight := 1
	chartWidth := width - yLabelWidth - 2  // right margin
	chartHeight := height - 4 - xLabelHeight // title(2) + x-axis(1) + margin(1)
	if chartWidth < 10 {
		chartWidth = 10
	}
	if chartHeight < 4 {
		chartHeight = 4
	}

	// Braille resolution: each char = 2 dots wide, 4 dots tall
	dotsWide := chartWidth * 2
	dotsTall := chartHeight * 4

	// Resample values to fit chart width
	resampled := resampleValues(values, dotsWide)

	// Find max for Y-axis scaling
	maxVal := 0.0
	for _, v := range resampled {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	// Round up to nice number for Y-axis
	maxVal = niceMax(maxVal)

	// Build Braille canvas: [row][col] where each cell is a Braille char
	canvas := make([][]rune, chartHeight)
	for r := range canvas {
		canvas[r] = make([]rune, chartWidth)
	}

	// Convert values to dot Y positions
	dotYs := make([]int, len(resampled))
	for i, v := range resampled {
		if v <= 0 {
			dotYs[i] = 0
		} else {
			dotYs[i] = int(v / maxVal * float64(dotsTall-1))
			if dotYs[i] >= dotsTall {
				dotYs[i] = dotsTall - 1
			}
		}
	}

	// setDot plots a single dot on the Braille canvas
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

	// Draw connected lines between consecutive data points using Bresenham's algorithm
	for i := 0; i < len(resampled); i++ {
		setDot(i, dotYs[i])
		if i > 0 {
			// Draw line from (i-1, dotYs[i-1]) to (i, dotYs[i])
			x0, y0 := i-1, dotYs[i-1]
			x1, y1 := i, dotYs[i]
			bresenhamLine(x0, y0, x1, y1, setDot)
		}
	}

	// Render chart rows with Y-axis labels
	for row := 0; row < chartHeight; row++ {
		// Y-axis label
		if row == 0 {
			sb.WriteString(fmt.Sprintf("  %6s│", formatAxisValue(maxVal)))
		} else if row == chartHeight-1 {
			sb.WriteString(fmt.Sprintf("  %6s│", "0"))
		} else if row == chartHeight/2 {
			sb.WriteString(fmt.Sprintf("  %6s│", formatAxisValue(maxVal/2)))
		} else {
			sb.WriteString("        │")
		}

		// Braille chars for this row
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

	// X-axis time labels (left=oldest, right=now)
	sb.WriteString("        ")
	totalSecs := len(values)
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

// resampleValues resamples input values to target count using nearest-neighbor.
func resampleValues(values []float64, targetCount int) []float64 {
	if len(values) == 0 || targetCount <= 0 {
		return nil
	}
	if len(values) <= targetCount {
		// Pad left with zeros
		result := make([]float64, targetCount)
		offset := targetCount - len(values)
		copy(result[offset:], values)
		return result
	}
	// Downsample
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

// bresenhamLine draws a line between two points using Bresenham's algorithm,
// calling setDot for each point along the line.
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
