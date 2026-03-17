package tui

import (
	"fmt"
	"strings"
)

const defaultDiagnosisPanelWidth = 56

func (a *App) renderDiagnosisPanel() {
	if len(a.panels) <= 4 || a.panels[4] == nil {
		return
	}

	_, _, width, _ := a.panels[4].GetInnerRect()
	a.panels[4].SetText(renderDiagnosisPanel(a.topDiagnosis, width))
}

func renderDiagnosisPanel(diagnosis *topDiagnosis, panelWidth int) string {
	if diagnosis == nil {
		return "  [dim]Waiting for live diagnosis data[white]"
	}

	contentWidth := diagnosisContentWidth(panelWidth)
	color := colorForHealthLevel(diagnosis.Severity)

	var sb strings.Builder
	sb.WriteString("  [")
	sb.WriteString(color)
	sb.WriteString("]Summary: ")
	sb.WriteString(truncateRight(strings.TrimSpace(diagnosis.Headline), max(0, contentWidth-len("Summary: "))))
	sb.WriteString("[white]\n")

	sb.WriteString("  [dim]Why: ")
	sb.WriteString(truncateRight(strings.TrimSpace(diagnosis.Reason), max(0, contentWidth-len("Why: "))))
	sb.WriteString("[white]\n")

	sb.WriteString("  [bold]Evidence[white]\n")
	for _, line := range diagnosisLines(diagnosis.Evidence, 2, contentWidth) {
		sb.WriteString("  [dim]-[white] ")
		sb.WriteString(truncateRight(line, max(0, contentWidth-2)))
		sb.WriteString("\n")
	}

	sb.WriteString("  [bold]Next Checks[white]\n")
	lines := diagnosisLines(diagnosis.NextChecks, 2, contentWidth)
	for i, line := range lines {
		sb.WriteString("  [dim]-[white] ")
		sb.WriteString(truncateRight(line, max(0, contentWidth-2)))
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func diagnosisLines(lines []string, limit int, contentWidth int) []string {
	result := make([]string, 0, limit)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, truncateRight(line, max(0, contentWidth-2)))
		if len(result) >= limit {
			return result
		}
	}
	if len(result) == 0 {
		result = append(result, truncateRight("No additional detail available.", max(0, contentWidth-2)))
	}
	return result
}

func diagnosisContentWidth(panelWidth int) int {
	if panelWidth <= 0 {
		panelWidth = defaultDiagnosisPanelWidth
	}
	if panelWidth <= 4 {
		return panelWidth
	}
	return panelWidth - 2
}

func formatDiagnosisSummaryLine(diagnosis *topDiagnosis, panelWidth int) string {
	if diagnosis == nil {
		return ""
	}
	contentWidth := diagnosisContentWidth(panelWidth)
	headline := truncateRight(strings.TrimSpace(diagnosis.Headline), max(0, contentWidth-len("Diagnosis: ")))
	return fmt.Sprintf("Diagnosis: %s", headline)
}
