package tui

import "strings"

const (
	defaultDiagnosisPanelWidth = 56
	diagnosisMinPanelWidth     = 24
)

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

	var sb strings.Builder
	writeDiagnosisField(&sb, "Issue", diagnosisIssueValue(diagnosis), contentWidth, colorForHealthLevel(diagnosis.Severity))
	writeDiagnosisField(&sb, "Scope", diagnosisScopeValue(diagnosis), contentWidth, "dim")
	writeDiagnosisField(&sb, "Signal", diagnosisSignalValue(diagnosis), contentWidth, "white")
	writeDiagnosisField(&sb, "Likely", diagnosisLikelyValue(diagnosis), contentWidth, "white")
	writeDiagnosisField(&sb, "Check", diagnosisCheckValue(diagnosis), contentWidth, "white")

	return strings.TrimRight(sb.String(), "\n")
}

func writeDiagnosisField(sb *strings.Builder, label, value string, contentWidth int, valueColor string) {
	value = normalizeDiagnosisText(value)
	if value == "" {
		value = "-"
	}

	prefix := label + ": "
	available := max(1, contentWidth-len(prefix))
	wrapped := wrapDiagnosisText(value, available)
	indent := strings.Repeat(" ", len(prefix))

	sb.WriteString("  [dim]")
	sb.WriteString(prefix)
	sb.WriteString("[white]")
	if color := strings.TrimSpace(valueColor); color != "" && color != "white" {
		sb.WriteString("[")
		sb.WriteString(color)
		sb.WriteString("]")
	}
	sb.WriteString(wrapped[0])
	for _, line := range wrapped[1:] {
		sb.WriteString("\n  ")
		sb.WriteString(indent)
		sb.WriteString(line)
	}
	sb.WriteString("[white]\n")
}

func wrapDiagnosisText(text string, width int) []string {
	text = normalizeDiagnosisText(text)
	if text == "" {
		return []string{"-"}
	}
	if width <= 1 || len(text) <= width {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{"-"}
	}

	lines := make([]string, 0, 4)
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func normalizeDiagnosisText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func diagnosisIssueValue(diagnosis *topDiagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Issue); text != "" {
		return text
	}
	if text := strings.TrimSpace(diagnosis.Headline); text != "" {
		return text
	}
	return ""
}

func diagnosisScopeValue(diagnosis *topDiagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Scope); text != "" {
		return text
	}
	return "host-wide"
}

func diagnosisSignalValue(diagnosis *topDiagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Signal); text != "" {
		return text
	}
	if len(diagnosis.Evidence) > 0 {
		return diagnosis.Evidence[0]
	}
	return ""
}

func diagnosisLikelyValue(diagnosis *topDiagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Likely); text != "" {
		return text
	}
	return diagnosis.Reason
}

func diagnosisCheckValue(diagnosis *topDiagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Check); text != "" {
		return text
	}
	if len(diagnosis.NextChecks) > 0 {
		return diagnosis.NextChecks[0]
	}
	return ""
}

func diagnosisContentWidth(panelWidth int) int {
	if panelWidth <= 0 || panelWidth < diagnosisMinPanelWidth {
		panelWidth = defaultDiagnosisPanelWidth
	}
	if panelWidth <= 4 {
		return panelWidth
	}
	return panelWidth - 2
}
