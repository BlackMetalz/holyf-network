package panels

import (
	"fmt"
	"strings"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

const (
	defaultDiagnosisPanelWidth = 56
	diagnosisMinPanelWidth     = 24
)

type diagField struct {
	label string
	value string
	color string
}

func RenderDiagnosisPanel(diagnosis *tuishared.Diagnosis, panelWidth int) string {
	return RenderDiagnosisPanelWithHeight(diagnosis, panelWidth, 0)
}

func RenderDiagnosisPanelWithHeight(diagnosis *tuishared.Diagnosis, panelWidth, panelHeight int) string {
	if diagnosis == nil {
		return "  [dim]Waiting for live diagnosis data[white]"
	}

	contentWidth := diagnosisContentWidth(panelWidth)

	// Build fields with full content first.
	fields := []diagField{
		{"Issue", DiagnosisIssueValue(diagnosis), tuishared.ColorForHealthLevel(diagnosis.Severity)},
		{"Scope", DiagnosisScopeValue(diagnosis), "dim"},
		{"Signal", DiagnosisSignalValue(diagnosis), "white"},
		{"Likely Cause", DiagnosisLikelyValue(diagnosis), "white"},
		{"Confidence", DiagnosisConfidenceValue(diagnosis), "aqua"},
		{"Why", DiagnosisWhyValue(diagnosis), "white"},
	}
	actions := DiagnosisNextActionsList(diagnosis)

	// If height is known, estimate line usage and condense to fit.
	if panelHeight > 0 {
		fields, actions = fitDiagnosisToHeight(fields, actions, contentWidth, panelHeight)
	}

	var sb strings.Builder
	for _, f := range fields {
		writeDiagnosisField(&sb, f.label, f.value, contentWidth, f.color)
	}
	writeDiagnosisActionsField(&sb, "Next Actions", actions, contentWidth, "white")

	return strings.TrimRight(sb.String(), "\n")
}

// fitDiagnosisToHeight progressively condenses diagnosis content to fit within
// the available panel height. Reduction steps (applied in order until it fits):
//  1. Reduce actions from 3 → 2 → 1
//  2. Remove "Why" field
//  3. Truncate remaining wrapped fields to single lines
func fitDiagnosisToHeight(fields []diagField, actions []string, contentWidth, height int) ([]diagField, []string) {
	// Estimate total lines for a given field set + action list.
	estimate := func(fs []diagField, acts []string) int {
		lines := 0
		for _, f := range fs {
			prefix := f.label + ": "
			avail := max(1, contentWidth-len(prefix))
			lines += len(wrapDiagnosisText(normalizeDiagnosisText(f.value), avail))
		}
		// Actions header line.
		if len(acts) > 0 {
			lines++ // "Next Actions:" label
			for _, a := range acts {
				prefix := "    1) " // all prefixes same length
				avail := max(1, contentWidth-len(prefix))
				lines += len(wrapDiagnosisText(normalizeDiagnosisText(a), avail))
			}
		} else {
			lines++ // "Next Actions: -"
		}
		return lines
	}

	total := estimate(fields, actions)
	if total <= height {
		return fields, actions
	}

	// Step 1: reduce actions count.
	for maxActs := min(len(actions), 2); maxActs >= 1; maxActs-- {
		trimmed := actions[:maxActs]
		if estimate(fields, trimmed) <= height {
			return fields, trimmed
		}
	}
	if len(actions) > 1 {
		actions = actions[:1]
	}

	// Step 2: remove "Why" field.
	if estimate(fields, actions) > height {
		stripped := make([]diagField, 0, len(fields))
		for _, f := range fields {
			if f.label != "Why" {
				stripped = append(stripped, f)
			}
		}
		fields = stripped
	}

	// Step 3: truncate all wrapped fields to single line.
	if estimate(fields, actions) > height {
		for i, f := range fields {
			prefix := f.label + ": "
			avail := max(1, contentWidth-len(prefix))
			fields[i].value = truncateDiagnosisText(f.value, avail)
		}
		if len(actions) > 0 {
			prefix := "    1) "
			avail := max(1, contentWidth-len(prefix))
			actions = []string{truncateDiagnosisText(actions[0], avail)}
		}
	}

	return fields, actions
}

func DiagnosisIssueValue(diagnosis *tuishared.Diagnosis) string {
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

func DiagnosisScopeValue(diagnosis *tuishared.Diagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Scope); text != "" {
		return text
	}
	return "host-wide"
}

func DiagnosisSignalValue(diagnosis *tuishared.Diagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Signal); text != "" {
		return formatDiagnosisSignal(text)
	}
	if len(diagnosis.Evidence) > 0 {
		return diagnosis.Evidence[0]
	}
	return ""
}

func DiagnosisLikelyValue(diagnosis *tuishared.Diagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Likely); text != "" {
		return text
	}
	return diagnosis.Reason
}

func DiagnosisConfidenceValue(diagnosis *tuishared.Diagnosis) string {
	if diagnosis == nil {
		return ""
	}
	if text := strings.TrimSpace(diagnosis.Confidence); text != "" {
		return text
	}
	switch diagnosis.Severity {
	case tuishared.HealthCrit:
		return "HIGH"
	case tuishared.HealthWarn:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func DiagnosisWhyValue(diagnosis *tuishared.Diagnosis) string {
	if diagnosis == nil {
		return ""
	}
	evidence := compactDiagnosisList(diagnosis.Evidence, 2)
	if len(evidence) > 0 {
		return strings.Join(evidence, "; ")
	}
	if text := strings.TrimSpace(diagnosis.Reason); text != "" {
		return text
	}
	return DiagnosisSignalValue(diagnosis)
}

func DiagnosisCheckValue(diagnosis *tuishared.Diagnosis) string {
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

func DiagnosisNextActionsList(diagnosis *tuishared.Diagnosis) []string {
	if diagnosis == nil {
		return nil
	}
	steps := compactDiagnosisList(diagnosis.NextChecks, 3)
	if len(steps) > 0 {
		return steps
	}
	if text := strings.TrimSpace(diagnosis.Check); text != "" {
		return []string{normalizeDiagnosisText(text)}
	}
	return nil
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

func writeDiagnosisActionsField(sb *strings.Builder, label string, actions []string, contentWidth int, valueColor string) {
	if len(actions) == 0 {
		writeDiagnosisField(sb, label, "-", contentWidth, valueColor)
		return
	}

	sb.WriteString("  [dim]")
	sb.WriteString(label)
	sb.WriteString(":[white]\n")
	if color := strings.TrimSpace(valueColor); color != "" && color != "white" {
		sb.WriteString("[")
		sb.WriteString(color)
		sb.WriteString("]")
	}

	for i, action := range actions {
		prefix := fmt.Sprintf("    %d) ", i+1)
		available := max(1, contentWidth-len(prefix))
		wrapped := wrapDiagnosisText(normalizeDiagnosisText(action), available)
		indent := strings.Repeat(" ", len(prefix))
		sb.WriteString(prefix)
		sb.WriteString(wrapped[0])
		sb.WriteString("\n")
		for _, line := range wrapped[1:] {
			sb.WriteString(indent)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
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

func formatDiagnosisSignal(raw string) string {
	segments := strings.Split(normalizeDiagnosisText(raw), "|")
	if len(segments) == 0 {
		return normalizeDiagnosisText(raw)
	}
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		seg := normalizeDiagnosisText(segment)
		if seg == "" {
			continue
		}
		out = append(out, formatDiagnosisSignalSegment(seg))
	}
	if len(out) == 0 {
		return normalizeDiagnosisText(raw)
	}
	return strings.Join(out, " | ")
}

func formatDiagnosisSignalSegment(segment string) string {
	switch {
	case strings.EqualFold(segment, "States stable"):
		return "States: stable"
	}

	parts := strings.Fields(segment)
	if len(parts) < 2 {
		return segment
	}
	head := strings.ToUpper(parts[0])
	rest := strings.Join(parts[1:], " ")

	switch head {
	case "RETR":
		return "Retrans: " + rest
	case "CT":
		return "Conntrack: " + rest
	case "OUT":
		return "Out seg/s: " + rest
	case "EST":
		return "ESTABLISHED: " + rest
	case "TW":
		return "TIME_WAIT: " + rest
	case "CW":
		return "CLOSE_WAIT: " + rest
	case "SR":
		return "SYN_RECV: " + rest
	case "FW1":
		return "FIN_WAIT1: " + rest
	case "SYN_RECV", "CLOSE_WAIT", "TIME_WAIT", "FIN_WAIT1", "ESTABLISHED":
		return parts[0] + ": " + rest
	default:
		return segment
	}
}

func truncateDiagnosisText(text string, width int) string {
	text = normalizeDiagnosisText(text)
	if text == "" || width <= 0 {
		return "-"
	}
	if len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func compactDiagnosisList(items []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, item := range items {
		normalized := normalizeDiagnosisText(item)
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
		if len(out) >= limit {
			break
		}
	}
	return out
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
