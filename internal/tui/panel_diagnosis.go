package tui

import (
	"strings"
)

const (
	defaultDiagnosisPanelWidth = 56
	diagnosisCompactThreshold  = 62
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
	if contentWidth <= diagnosisCompactThreshold {
		return renderDiagnosisCompactPanel(diagnosis, contentWidth)
	}
	return renderDiagnosisDetailedPanel(diagnosis, contentWidth)
}

func renderDiagnosisCompactPanel(diagnosis *topDiagnosis, contentWidth int) string {
	var sb strings.Builder

	issue, scope := splitDiagnosisHeadline(strings.TrimSpace(diagnosis.Headline))
	writeDiagnosisField(&sb, "Issue", issue, contentWidth, colorForHealthLevel(diagnosis.Severity))
	if scope != "" {
		writeDiagnosisField(&sb, "Scope", scope, contentWidth, "dim")
	}

	signal := compactDiagnosisSignal(diagnosis)
	if signal != "" {
		writeDiagnosisField(&sb, "Signal", signal, contentWidth, "white")
	}

	likely := compactDiagnosisLikely(diagnosis.Reason)
	if likely != "" {
		writeDiagnosisField(&sb, "Likely", likely, contentWidth, "white")
	}

	checks := compactDiagnosisChecks(diagnosis.NextChecks)
	if checks != "" {
		writeDiagnosisField(&sb, "Check", checks, contentWidth, "white")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func renderDiagnosisDetailedPanel(diagnosis *topDiagnosis, contentWidth int) string {
	var sb strings.Builder

	writeDiagnosisField(&sb, "Summary", strings.TrimSpace(diagnosis.Headline), contentWidth, colorForHealthLevel(diagnosis.Severity))
	writeDiagnosisField(&sb, "Why", strings.TrimSpace(diagnosis.Reason), contentWidth, "white")

	sb.WriteString("  [bold]Evidence[white]\n")
	writeDiagnosisBullets(&sb, diagnosis.Evidence, contentWidth, 2)
	sb.WriteString("  [bold]Next Checks[white]\n")
	writeDiagnosisBullets(&sb, diagnosis.NextChecks, contentWidth, 2)

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

func writeDiagnosisBullets(sb *strings.Builder, lines []string, contentWidth int, limit int) {
	items := diagnosisSectionLines(lines, limit)
	bulletWidth := max(1, contentWidth-2)
	for _, line := range items {
		wrapped := wrapDiagnosisText(line, bulletWidth)
		sb.WriteString("  [dim]-[white] ")
		sb.WriteString(wrapped[0])
		for _, next := range wrapped[1:] {
			sb.WriteString("\n    ")
			sb.WriteString(next)
		}
		sb.WriteString("\n")
	}
}

func diagnosisSectionLines(lines []string, limit int) []string {
	result := make([]string, 0, limit)
	for _, line := range lines {
		line = normalizeDiagnosisText(line)
		if line == "" {
			continue
		}
		result = append(result, line)
		if len(result) >= limit {
			return result
		}
	}
	if len(result) == 0 {
		result = append(result, "No additional detail available.")
	}
	return result
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

func splitDiagnosisHeadline(headline string) (issue string, scope string) {
	headline = normalizeDiagnosisText(headline)
	if headline == "" {
		return "", ""
	}
	if idx := strings.Index(headline, " on "); idx > 0 {
		return strings.TrimSpace(headline[:idx]), strings.TrimSpace(headline[idx+4:])
	}
	return headline, ""
}

func compactDiagnosisSignal(diagnosis *topDiagnosis) string {
	parts := make([]string, 0, 2)
	for _, line := range diagnosisSectionLines(diagnosis.Evidence, 2) {
		line = strings.TrimSuffix(line, ".")
		line = strings.TrimPrefix(line, "State count: ")
		line = strings.TrimPrefix(line, "Culprit: ")
		line = strings.TrimPrefix(line, "Usage: ")
		line = strings.TrimPrefix(line, "Retrans: ")
		line = strings.TrimPrefix(line, "Conntrack: ")
		line = strings.TrimPrefix(line, "Drops: ")
		line = strings.TrimPrefix(line, "Sample ready: ")
		if idx := strings.Index(line, " via "); idx >= 0 {
			line = strings.TrimSpace(line[idx+5:])
		}
		line = strings.ReplaceAll(line, "connection sample", "sample")
		line = strings.ReplaceAll(line, "tracked entries", "entries")
		if line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " | ")
}

func compactDiagnosisLikely(reason string) string {
	reason = normalizeDiagnosisText(reason)
	if reason == "" {
		return ""
	}
	if idx := strings.Index(reason, ";"); idx >= 0 && idx+1 < len(reason) {
		reason = strings.TrimSpace(reason[idx+1:])
	}
	reason = strings.TrimSuffix(reason, ".")
	replacer := strings.NewReplacer(
		"are dominating more than a current path-quality issue", "look more like connection churn than a current path issue",
		"the local app has not closed these sockets yet", "the local app has not closed these sockets yet",
		"half-open handshakes are piling up", "half-open handshakes are piling up",
		"close handshakes are stalling", "close handshakes are stalling",
	)
	return replacer.Replace(reason)
}

func compactDiagnosisChecks(nextChecks []string) string {
	parts := make([]string, 0, 2)
	for _, line := range diagnosisSectionLines(nextChecks, 2) {
		line = strings.TrimSuffix(line, ".")
		line = strings.TrimPrefix(line, "Check whether one service is creating ")
		line = strings.TrimPrefix(line, "Check whether ")
		line = strings.TrimPrefix(line, "Check ")
		line = strings.TrimPrefix(line, "Review keepalive, ")
		line = strings.TrimPrefix(line, "Review ")
		line = strings.TrimPrefix(line, "Validate ")
		line = strings.TrimPrefix(line, "Inspect ")
		line = strings.TrimPrefix(line, "Run ")
		line = strings.TrimPrefix(line, "Keep watching ")
		line = strings.TrimPrefix(line, "Collect ")
		line = strings.ReplaceAll(line, "faster than expected", "")
		line = strings.ReplaceAll(line, "connection reuse", "conn reuse")
		line = strings.ReplaceAll(line, "short-lived connections", "short-lived conns")
		line = strings.ReplaceAll(line, "Connection States", "States")
		line = strings.ReplaceAll(line, "Top Connections", "Top")
		line = strings.ReplaceAll(line, "client retry behavior", "client retries")
		line = strings.ReplaceAll(line, "before blaming packet loss", "")
		line = strings.ReplaceAll(line, "or ", "")
		line = strings.ReplaceAll(line, "  ", " ")
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " | ")
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
