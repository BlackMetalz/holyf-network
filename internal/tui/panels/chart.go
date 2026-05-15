package panels

import (
	"fmt"
	"math"
	"strings"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

// Braille dot patterns for plotting. Each Braille character is a 2x4 dot matrix.
//
//	col 0   col 1     bits:
//	row 0   row 0     0x01  0x08
//	row 1   row 1     0x02  0x10
//	row 2   row 2     0x04  0x20
//	row 3   row 3     0x40  0x80
const brailleBase = 0x2800

var brailleDots = [4][2]rune{
	{0x01, 0x08}, // row 0 (top)
	{0x02, 0x10}, // row 1
	{0x04, 0x20}, // row 2
	{0x40, 0x80}, // row 3 (bottom)
}

// RenderTimeSeriesChart renders a filled area chart using Braille characters.
// Each column is filled from the bottom (0) up to the data value, creating
// a solid filled shape that reads as a continuous line.
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
	chartHeight := height - 5
	if chartWidth < 10 {
		chartWidth = 10
	}
	if chartHeight < 3 {
		chartHeight = 3
	}

	dotsWide := chartWidth * 2
	dotsTall := chartHeight * 4

	// Find max for Y-axis
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

	// Build canvas
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

	// For each dot column, interpolate the Y value from the data points
	// and fill from bottom (0) up to that Y value.
	n := len(values)
	for dotX := 0; dotX < dotsWide; dotX++ {
		// Map dot column back to fractional data index
		var val float64
		if n == 1 {
			val = values[0]
		} else {
			fIdx := float64(dotX) / float64(dotsWide-1) * float64(n-1)
			idx0 := int(fIdx)
			idx1 := idx0 + 1
			if idx1 >= n {
				idx1 = n - 1
			}
			frac := fIdx - float64(idx0)
			val = values[idx0]*(1-frac) + values[idx1]*frac
		}

		// Map value to dot Y (0=bottom, dotsTall-1=top)
		topDotY := int(val / maxVal * float64(dotsTall-1))
		if topDotY < 0 {
			topDotY = 0
		}
		if topDotY >= dotsTall {
			topDotY = dotsTall - 1
		}

		// Fill column from bottom up to topDotY — creates solid area
		// Only fill the top 2 dots to make it look like a line, not a filled bar
		startY := topDotY - 1
		if startY < 0 {
			startY = 0
		}
		for dotY := startY; dotY <= topDotY; dotY++ {
			setDot(dotX, dotY)
		}
	}

	// Render chart with Y-axis labels
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

	// X-axis
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
// Used by sparkline renderer.
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
