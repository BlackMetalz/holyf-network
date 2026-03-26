package trace

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type RenderOptions struct {
	SensitiveIP       bool
	SeverityInfo      string
	FormatPreviewIP   func(string, bool) string
	MaskSensitiveText func(string, bool) string
	ShortStatus       func(string, int) string
	MaskPath          func(string, bool) string
	MetricDisplay     func(int, bool) string
	MetricValue       func(int) string
	SeverityStyled    func(string) string
	ConfidenceStyled  func(string) string
}

func FormatListItem(entry Entry, opts RenderOptions) (main string, secondary string) {
	ts := entry.CapturedAt.Local().Format("2006-01-02 15:04:05")
	peer := applyPreviewIP(opts, entry.PeerIP)
	port := PortLabel(entry.Port)
	color := SeverityColor(entry.Severity, opts.SeverityInfo)
	main = fmt.Sprintf(
		"%s [%s]%s[white] %s | %s:%s",
		ts,
		color,
		blankIfUnknown(entry.Severity, blankIfUnknown(opts.SeverityInfo, "INFO")),
		applyShortStatus(opts, applyMaskText(opts, entry.Issue), 44),
		peer,
		port,
	)
	secondary = fmt.Sprintf(
		"status=%s mode=%s dir=%s cat=%s scope=%s cap=%s drop=%s rst=%d conf=%s",
		blankIfUnknown(entry.Status, "completed"),
		Mode(entry),
		blankIfUnknown(entry.Direction, "ANY"),
		Category(entry),
		blankIfUnknown(entry.Scope, "Peer + Port"),
		applyMetricDisplay(opts, entry.Captured, entry.CapturedEstimated),
		applyMetricValue(opts, entry.DroppedByKernel),
		entry.RstCount,
		blankIfUnknown(entry.Confidence, "LOW"),
	)
	return main, secondary
}

func BuildDetailText(entry Entry, opts RenderOptions) string {
	var b strings.Builder

	b.WriteString("  [yellow]Trace Packet History Detail[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf("  CapturedAt: [green]%s[white]\n", entry.CapturedAt.Local().Format("2006-01-02 15:04:05")))
	if !entry.StartedAt.IsZero() || !entry.EndedAt.IsZero() {
		started := "n/a"
		ended := "n/a"
		if !entry.StartedAt.IsZero() {
			started = entry.StartedAt.Local().Format("15:04:05")
		}
		if !entry.EndedAt.IsZero() {
			ended = entry.EndedAt.Local().Format("15:04:05")
		}
		b.WriteString(fmt.Sprintf("  Window: [green]%s[white] -> [green]%s[white]\n", started, ended))
	}

	peer := applyPreviewIP(opts, entry.PeerIP)
	b.WriteString(fmt.Sprintf("  Interface: [green]%s[white]\n", blankIfUnknown(entry.Interface, "n/a")))
	b.WriteString(fmt.Sprintf("  Target: [green]%s:%s[white]\n", peer, PortLabel(entry.Port)))
	b.WriteString(fmt.Sprintf(
		"  Mode: [green]%s[white] | Category: [green]%s[white] | Scope: [green]%s[white] | Direction: [green]%s[white]\n",
		Mode(entry),
		Category(entry),
		blankIfUnknown(entry.Scope, "Peer + Port"),
		blankIfUnknown(entry.Direction, "ANY"),
	))
	b.WriteString(fmt.Sprintf("  Duration: [green]%ds[white] | Packet cap: [green]%d[white]\n", entry.DurationSec, entry.PacketCap))
	b.WriteString(fmt.Sprintf("  Filter: [green]%s[white]\n", applyMaskText(opts, blankIfUnknown(entry.Filter, "n/a"))))
	b.WriteString(fmt.Sprintf("  Status: [green]%s[white]\n", blankIfUnknown(entry.Status, "completed")))

	b.WriteString(fmt.Sprintf(
		"  Captured: [green]%s[white] | ReceivedByFilter: [green]%s[white] | DroppedByKernel: [green]%s[white]\n",
		applyMetricDisplay(opts, entry.Captured, entry.CapturedEstimated),
		applyMetricValue(opts, entry.ReceivedByFilter),
		applyMetricValue(opts, entry.DroppedByKernel),
	))
	b.WriteString(fmt.Sprintf(
		"  Decoded: [green]%d[white] | SYN: [green]%d[white] | SYN-ACK: [green]%d[white] | RST: [green]%d[white]\n",
		entry.DecodedPackets,
		entry.SynCount,
		entry.SynAckCount,
		entry.RstCount,
	))

	b.WriteString("\n  [yellow]Trace Analyzer[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	severity := blankIfUnknown(entry.Severity, blankIfUnknown(opts.SeverityInfo, "INFO"))
	confidence := blankIfUnknown(entry.Confidence, "LOW")
	b.WriteString(fmt.Sprintf(
		"  Severity: %s | Confidence: %s\n",
		applySeverityStyled(opts, severity),
		applyConfidenceStyled(opts, confidence),
	))
	b.WriteString(fmt.Sprintf("  Issue: %s\n", applyMaskText(opts, blankIfUnknown(entry.Issue, "n/a"))))
	b.WriteString(fmt.Sprintf("  Signal: %s\n", applyMaskText(opts, blankIfUnknown(entry.Signal, "n/a"))))
	b.WriteString(fmt.Sprintf("  Likely: %s\n", applyMaskText(opts, blankIfUnknown(entry.Likely, "n/a"))))
	b.WriteString(fmt.Sprintf("  Check next: %s\n", applyMaskText(opts, blankIfUnknown(entry.Check, "n/a"))))

	if entry.Saved && strings.TrimSpace(entry.PCAPPath) != "" {
		b.WriteString(fmt.Sprintf("  PCAP: [aqua]%s[white]\n", applyMaskPath(opts, entry.PCAPPath)))
	} else {
		b.WriteString("  PCAP: [dim]not saved[white]\n")
	}

	if msg := strings.TrimSpace(entry.CaptureErr); msg != "" {
		b.WriteString(fmt.Sprintf("  [red]Capture error:[white] %s\n", applyShortStatus(opts, applyMaskText(opts, msg), 180)))
	}
	if msg := strings.TrimSpace(entry.ReadErr); msg != "" {
		b.WriteString(fmt.Sprintf("  [yellow]Read warning:[white] %s\n", applyShortStatus(opts, applyMaskText(opts, msg), 180)))
	}

	b.WriteString("\n  [yellow]Sample Packets[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	if len(entry.Sample) == 0 {
		b.WriteString("  [dim]No decoded packet lines.[white]\n")
	} else {
		for _, line := range entry.Sample {
			b.WriteString("  ")
			b.WriteString(applyMaskText(opts, line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n  [dim]Enter/Esc to close.[white]")
	return b.String()
}

func BuildCompareText(baseline, incident Entry, opts RenderOptions) string {
	var b strings.Builder

	b.WriteString("  [yellow]Trace Compare (Baseline vs Incident)[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf(
		"  Baseline: [green]%s[white] | [aqua]%s:%s[white] | cat=%s\n",
		baseline.CapturedAt.Local().Format("2006-01-02 15:04:05"),
		applyPreviewIP(opts, baseline.PeerIP),
		PortLabel(baseline.Port),
		Category(baseline),
	))
	b.WriteString(fmt.Sprintf(
		"  Incident: [green]%s[white] | [aqua]%s:%s[white] | cat=%s\n",
		incident.CapturedAt.Local().Format("2006-01-02 15:04:05"),
		applyPreviewIP(opts, incident.PeerIP),
		PortLabel(incident.Port),
		Category(incident),
	))

	baseDrop := dropRatio(baseline)
	incDrop := dropRatio(incident)
	baseRST := rstRatio(baseline)
	incRST := rstRatio(incident)
	baseSynAck := synAckRatio(baseline)
	incSynAck := synAckRatio(incident)

	b.WriteString("\n  [yellow]Ratio Diff[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	b.WriteString(ratioDiffLine("Drop ratio", baseDrop, incDrop))
	b.WriteString(ratioDiffLine("RST ratio", baseRST, incRST))
	b.WriteString(ratioDiffLine("SYN-ACK ratio", baseSynAck, incSynAck))

	b.WriteString("\n  [yellow]Top Flags Changed (incident - baseline)[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	for _, line := range topFlagChangeLines(baseline, incident) {
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n  [dim]Enter/Esc to close.[white]")
	return b.String()
}

func SeverityColor(severity, infoFallback string) string {
	switch strings.ToUpper(strings.TrimSpace(blankIfUnknown(severity, blankIfUnknown(infoFallback, "INFO")))) {
	case "CRIT":
		return "red"
	case "WARN":
		return "yellow"
	default:
		return "green"
	}
}

func Category(entry Entry) string {
	preset := strings.TrimSpace(entry.Preset)
	if preset != "" {
		return preset
	}
	scope := strings.ToLower(strings.TrimSpace(entry.Scope))
	switch {
	case strings.Contains(scope, "5-tuple"):
		return "5-tuple"
	case strings.Contains(scope, "syn/rst"):
		return "SYN/RST only"
	case strings.Contains(scope, "custom"):
		return "Custom"
	case strings.Contains(scope, "peer only"):
		return "Peer only"
	default:
		return "Peer + Port"
	}
}

func Mode(entry Entry) string {
	mode := strings.TrimSpace(entry.Mode)
	if mode != "" {
		return mode
	}
	return "General triage"
}

func PortLabel(port int) string {
	if port <= 0 {
		return "peer-only"
	}
	return strconv.Itoa(port)
}

type ratio struct {
	Value float64
	OK    bool
}

type flagDelta struct {
	Name     string
	Baseline ratio
	Incident ratio
	AbsDelta float64
}

func ratioDiffLine(label string, baseline, incident ratio) string {
	return fmt.Sprintf(
		"  %-15s baseline=%-8s incident=%-8s delta=%s\n",
		label+":",
		percentLabel(baseline),
		percentLabel(incident),
		deltaPPLabel(baseline, incident),
	)
}

func percentLabel(r ratio) string {
	if !r.OK {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", r.Value*100.0)
}

func deltaPPLabel(baseline, incident ratio) string {
	if !baseline.OK || !incident.OK {
		return "n/a"
	}
	return fmt.Sprintf("%+.1fpp", (incident.Value-baseline.Value)*100.0)
}

func dropRatio(entry Entry) ratio {
	den := bestCounter(entry.ReceivedByFilter, entry.Captured, entry.DecodedPackets)
	if entry.DroppedByKernel < 0 || den <= 0 {
		return ratio{OK: false}
	}
	return ratio{Value: float64(entry.DroppedByKernel) / float64(den), OK: true}
}

func rstRatio(entry Entry) ratio {
	den := bestCounter(entry.DecodedPackets, entry.Captured)
	if entry.RstCount < 0 || den <= 0 {
		return ratio{OK: false}
	}
	return ratio{Value: float64(entry.RstCount) / float64(den), OK: true}
}

func synAckRatio(entry Entry) ratio {
	if entry.SynAckCount < 0 || entry.SynCount <= 0 {
		return ratio{OK: false}
	}
	return ratio{Value: float64(entry.SynAckCount) / float64(entry.SynCount), OK: true}
}

func bestCounter(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func topFlagChangeLines(baseline, incident Entry) []string {
	baseRatios := flagRatios(baseline)
	incRatios := flagRatios(incident)

	names := []string{"RST", "SYN-ACK", "SYN", "OTHER"}
	items := make([]flagDelta, 0, len(names))
	for _, name := range names {
		b := baseRatios[name]
		i := incRatios[name]
		delta := -1.0
		if b.OK && i.OK {
			delta = math.Abs(i.Value - b.Value)
		}
		items = append(items, flagDelta{Name: name, Baseline: b, Incident: i, AbsDelta: delta})
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
			percentLabel(item.Baseline),
			percentLabel(item.Incident),
			deltaPPLabel(item.Baseline, item.Incident),
		))
	}
	return lines
}

func flagRatios(entry Entry) map[string]ratio {
	syn := max(0, entry.SynCount)
	synAck := max(0, entry.SynAckCount)
	rst := max(0, entry.RstCount)
	decoded := bestCounter(entry.DecodedPackets, syn+synAck+rst)
	if decoded <= 0 {
		return map[string]ratio{
			"SYN": {OK: false}, "SYN-ACK": {OK: false}, "RST": {OK: false}, "OTHER": {OK: false},
		}
	}
	other := decoded - syn - synAck - rst
	if other < 0 {
		other = 0
	}
	return map[string]ratio{
		"SYN":     {Value: float64(syn) / float64(decoded), OK: true},
		"SYN-ACK": {Value: float64(synAck) / float64(decoded), OK: true},
		"RST":     {Value: float64(rst) / float64(decoded), OK: true},
		"OTHER":   {Value: float64(other) / float64(decoded), OK: true},
	}
}

func blankIfUnknown(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func applyPreviewIP(opts RenderOptions, ip string) string {
	if opts.FormatPreviewIP != nil {
		return opts.FormatPreviewIP(ip, opts.SensitiveIP)
	}
	return ip
}

func applyMaskText(opts RenderOptions, text string) string {
	if opts.MaskSensitiveText != nil {
		return opts.MaskSensitiveText(text, opts.SensitiveIP)
	}
	return text
}

func applyShortStatus(opts RenderOptions, text string, max int) string {
	if opts.ShortStatus != nil {
		return opts.ShortStatus(text, max)
	}
	return text
}

func applyMaskPath(opts RenderOptions, path string) string {
	if opts.MaskPath != nil {
		return opts.MaskPath(path, opts.SensitiveIP)
	}
	return path
}

func applyMetricDisplay(opts RenderOptions, value int, estimated bool) string {
	if opts.MetricDisplay != nil {
		return opts.MetricDisplay(value, estimated)
	}
	if estimated {
		return fmt.Sprintf("n/a~%d", value)
	}
	return fmt.Sprintf("%d", value)
}

func applyMetricValue(opts RenderOptions, value int) string {
	if opts.MetricValue != nil {
		return opts.MetricValue(value)
	}
	return fmt.Sprintf("%d", value)
}

func applySeverityStyled(opts RenderOptions, severity string) string {
	if opts.SeverityStyled != nil {
		return opts.SeverityStyled(severity)
	}
	return severity
}

func applyConfidenceStyled(opts RenderOptions, confidence string) string {
	if opts.ConfidenceStyled != nil {
		return opts.ConfidenceStyled(confidence)
	}
	return confidence
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
