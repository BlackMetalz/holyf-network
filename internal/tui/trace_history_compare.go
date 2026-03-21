package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type traceRatio struct {
	Value float64
	OK    bool
}

func buildTraceHistoryCompareText(baseline, incident traceHistoryEntry, sensitiveIP bool) string {
	var b strings.Builder

	b.WriteString("  [yellow]Trace Compare (Baseline vs Incident)[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf(
		"  Baseline: [green]%s[white] | [aqua]%s:%s[white] | cat=%s\n",
		baseline.CapturedAt.Local().Format("2006-01-02 15:04:05"),
		formatPreviewIP(baseline.PeerIP, sensitiveIP),
		traceHistoryPortLabel(baseline.Port),
		traceHistoryCategory(baseline),
	))
	b.WriteString(fmt.Sprintf(
		"  Incident: [green]%s[white] | [aqua]%s:%s[white] | cat=%s\n",
		incident.CapturedAt.Local().Format("2006-01-02 15:04:05"),
		formatPreviewIP(incident.PeerIP, sensitiveIP),
		traceHistoryPortLabel(incident.Port),
		traceHistoryCategory(incident),
	))

	baseDrop := traceHistoryDropRatio(baseline)
	incDrop := traceHistoryDropRatio(incident)
	baseRST := traceHistoryRSTRatio(baseline)
	incRST := traceHistoryRSTRatio(incident)
	baseSynAck := traceHistorySynAckRatio(baseline)
	incSynAck := traceHistorySynAckRatio(incident)

	b.WriteString("\n  [yellow]Ratio Diff[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	b.WriteString(traceHistoryRatioDiffLine("Drop ratio", baseDrop, incDrop))
	b.WriteString(traceHistoryRatioDiffLine("RST ratio", baseRST, incRST))
	b.WriteString(traceHistoryRatioDiffLine("SYN-ACK ratio", baseSynAck, incSynAck))

	b.WriteString("\n  [yellow]Top Flags Changed (incident - baseline)[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	for _, line := range traceHistoryTopFlagChangeLines(baseline, incident) {
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n  [dim]Enter/Esc to close.[white]")
	return b.String()
}

func traceHistoryRatioDiffLine(label string, baseline, incident traceRatio) string {
	return fmt.Sprintf(
		"  %-15s baseline=%-8s incident=%-8s delta=%s\n",
		label+":",
		traceHistoryPercentLabel(baseline),
		traceHistoryPercentLabel(incident),
		traceHistoryDeltaPPLabel(baseline, incident),
	)
}

func traceHistoryPercentLabel(r traceRatio) string {
	if !r.OK {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", r.Value*100.0)
}

func traceHistoryDeltaPPLabel(baseline, incident traceRatio) string {
	if !baseline.OK || !incident.OK {
		return "n/a"
	}
	return fmt.Sprintf("%+.1fpp", (incident.Value-baseline.Value)*100.0)
}

func traceHistoryDropRatio(entry traceHistoryEntry) traceRatio {
	den := traceHistoryBestCounter(entry.ReceivedByFilter, entry.Captured, entry.DecodedPackets)
	if entry.DroppedByKernel < 0 || den <= 0 {
		return traceRatio{OK: false}
	}
	return traceRatio{Value: float64(entry.DroppedByKernel) / float64(den), OK: true}
}

func traceHistoryRSTRatio(entry traceHistoryEntry) traceRatio {
	den := traceHistoryBestCounter(entry.DecodedPackets, entry.Captured)
	if entry.RstCount < 0 || den <= 0 {
		return traceRatio{OK: false}
	}
	return traceRatio{Value: float64(entry.RstCount) / float64(den), OK: true}
}

func traceHistorySynAckRatio(entry traceHistoryEntry) traceRatio {
	if entry.SynAckCount < 0 || entry.SynCount <= 0 {
		return traceRatio{OK: false}
	}
	return traceRatio{Value: float64(entry.SynAckCount) / float64(entry.SynCount), OK: true}
}

func traceHistoryBestCounter(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

type traceFlagDelta struct {
	Name     string
	Baseline traceRatio
	Incident traceRatio
	AbsDelta float64
}

func traceHistoryTopFlagChangeLines(baseline, incident traceHistoryEntry) []string {
	baseRatios := traceHistoryFlagRatios(baseline)
	incRatios := traceHistoryFlagRatios(incident)

	names := []string{"RST", "SYN-ACK", "SYN", "OTHER"}
	items := make([]traceFlagDelta, 0, len(names))
	for _, name := range names {
		b := baseRatios[name]
		i := incRatios[name]
		delta := -1.0
		if b.OK && i.OK {
			delta = math.Abs(i.Value - b.Value)
		}
		items = append(items, traceFlagDelta{
			Name:     name,
			Baseline: b,
			Incident: i,
			AbsDelta: delta,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].AbsDelta != items[j].AbsDelta {
			return items[i].AbsDelta > items[j].AbsDelta
		}
		return items[i].Name < items[j].Name
	})

	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf(
			"  %-8s baseline=%-8s incident=%-8s delta=%s",
			item.Name+":",
			traceHistoryPercentLabel(item.Baseline),
			traceHistoryPercentLabel(item.Incident),
			traceHistoryDeltaPPLabel(item.Baseline, item.Incident),
		))
	}
	return lines
}

func traceHistoryFlagRatios(entry traceHistoryEntry) map[string]traceRatio {
	syn := max(0, entry.SynCount)
	synAck := max(0, entry.SynAckCount)
	rst := max(0, entry.RstCount)
	decoded := traceHistoryBestCounter(entry.DecodedPackets, syn+synAck+rst)
	if decoded <= 0 {
		return map[string]traceRatio{
			"SYN":     {OK: false},
			"SYN-ACK": {OK: false},
			"RST":     {OK: false},
			"OTHER":   {OK: false},
		}
	}
	other := decoded - syn - synAck - rst
	if other < 0 {
		other = 0
	}
	return map[string]traceRatio{
		"SYN":     {Value: float64(syn) / float64(decoded), OK: true},
		"SYN-ACK": {Value: float64(synAck) / float64(decoded), OK: true},
		"RST":     {Value: float64(rst) / float64(decoded), OK: true},
		"OTHER":   {Value: float64(other) / float64(decoded), OK: true},
	}
}
