package tui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	traceHistoryPage          = "trace-history"
	traceHistoryDetailPage    = "trace-history-detail"
	traceHistoryComparePage   = "trace-history-compare"
	traceHistorySampleMax     = 8
	traceHistorySegmentPrefix = "trace-history-"
	traceHistorySegmentSuffix = ".jsonl"
	traceHistorySegmentLayout = "20060102"
)

type traceHistoryEntry struct {
	CapturedAt time.Time `json:"captured_at"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	EndedAt    time.Time `json:"ended_at,omitempty"`

	Interface   string `json:"interface"`
	PeerIP      string `json:"peer_ip"`
	Port        int    `json:"port"`
	Mode        string `json:"mode,omitempty"`
	Scope       string `json:"scope"`
	Preset      string `json:"preset,omitempty"`
	Direction   string `json:"direction"`
	DurationSec int    `json:"duration_sec"`
	PacketCap   int    `json:"packet_cap"`
	Filter      string `json:"filter"`

	Status   string `json:"status"`
	Saved    bool   `json:"saved"`
	PCAPPath string `json:"pcap_path,omitempty"`

	Captured          int  `json:"captured"`
	CapturedEstimated bool `json:"captured_estimated,omitempty"`
	ReceivedByFilter  int  `json:"received_by_filter"`
	DroppedByKernel   int  `json:"dropped_by_kernel"`
	DecodedPackets    int  `json:"decoded_packets"`
	SynCount          int  `json:"syn_count"`
	SynAckCount       int  `json:"syn_ack_count"`
	RstCount          int  `json:"rst_count"`

	Severity   string `json:"severity"`
	Confidence string `json:"confidence"`
	Issue      string `json:"issue"`
	Signal     string `json:"signal"`
	Likely     string `json:"likely"`
	Check      string `json:"check_next"`

	CaptureErr string   `json:"capture_err,omitempty"`
	ReadErr    string   `json:"read_err,omitempty"`
	Sample     []string `json:"sample_packets,omitempty"`
}

func (e *traceHistoryEntry) UnmarshalJSON(data []byte) error {
	type traceHistoryEntryAlias traceHistoryEntry
	var raw struct {
		traceHistoryEntryAlias
		LegacyProfile string `json:"profile,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*e = traceHistoryEntry(raw.traceHistoryEntryAlias)
	if strings.TrimSpace(e.Mode) == "" {
		e.Mode = strings.TrimSpace(raw.LegacyProfile)
	}
	return nil
}

func (a *App) appendTraceHistory(result tracePacketResult) {
	entry := newTraceHistoryEntry(result)

	a.traceHistoryMu.Lock()
	defer a.traceHistoryMu.Unlock()

	a.ensureTraceHistoryLoadedLocked()
	a.traceHistory = append(a.traceHistory, entry)
	a.persistTraceHistoryEntryLocked(entry)
	a.pruneTraceHistorySegmentsByAgeLocked(entry.CapturedAt)
}

func newTraceHistoryEntry(result tracePacketResult) traceHistoryEntry {
	diag := analyzeTracePacket(result)
	sample := make([]string, 0, traceHistorySampleMax)
	for _, line := range result.SampleLines {
		if len(sample) >= traceHistorySampleMax {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		sample = append(sample, trimmed)
	}

	capturedAt := result.EndedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	entry := traceHistoryEntry{
		CapturedAt: capturedAt,
		StartedAt:  result.StartedAt,
		EndedAt:    result.EndedAt,

		Interface:   result.Request.Interface,
		PeerIP:      normalizeIP(result.Request.PeerIP),
		Port:        result.Request.Port,
		Mode:        result.Request.Profile.Label(),
		Scope:       tracePacketScopeDisplay(result.Request),
		Preset:      tracePacketHistoryCategory(result.Request),
		Direction:   result.Request.Direction.Label(),
		DurationSec: result.Request.DurationSec,
		PacketCap:   result.Request.PacketCap,
		Filter:      result.Filter,

		Status:   traceHistoryStatusFromResult(result),
		Saved:    result.Saved,
		PCAPPath: result.PCAPPath,

		Captured:          result.Captured,
		CapturedEstimated: result.CapturedEstimated,
		ReceivedByFilter:  result.ReceivedByFilter,
		DroppedByKernel:   result.DroppedByKernel,
		DecodedPackets:    result.DecodedPackets,
		SynCount:          result.SynCount,
		SynAckCount:       result.SynAckCount,
		RstCount:          result.RstCount,

		Severity:   diag.Severity,
		Confidence: diag.Confidence,
		Issue:      diag.Issue,
		Signal:     diag.Signal,
		Likely:     diag.Likely,
		Check:      diag.Check,

		Sample: sample,
	}
	if result.CaptureErr != nil {
		entry.CaptureErr = result.CaptureErr.Error()
	}
	if result.ReadErr != nil {
		entry.ReadErr = result.ReadErr.Error()
	}
	return entry
}

func traceHistoryStatusFromResult(result tracePacketResult) string {
	switch {
	case result.Aborted:
		return "aborted"
	case result.CaptureErr != nil:
		return "failed"
	case result.TimedOut:
		return "completed-timeout"
	default:
		return "completed"
	}
}

func (a *App) promptTraceHistory() {
	entries := a.recentTraceHistory(traceHistoryModalLimit)
	if len(entries) == 0 {
		a.showTraceHistoryEmptyModal()
		return
	}

	closeModal := func() {
		a.pages.RemovePage(traceHistoryComparePage)
		a.pages.RemovePage(traceHistoryDetailPage)
		a.pages.RemovePage(traceHistoryPage)
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	list := tview.NewList().
		ShowSecondaryText(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedBackgroundColor(tcell.ColorDarkCyan)
	list.SetBorder(true)
	list.SetTitle(fmt.Sprintf(" Trace History (latest %d) ", traceHistoryModalLimit))

	for _, entry := range entries {
		mainText, secondary := formatTraceHistoryListItem(entry, a.sensitiveIP)
		list.AddItem(mainText, secondary, 0, nil)
	}

	openDetail := func() {
		idx := list.GetCurrentItem()
		if idx < 0 || idx >= len(entries) {
			return
		}
		a.showTraceHistoryDetailModal(entries[idx], list)
	}

	compareMarked := -1
	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	refreshFooter := func() {
		text := "  [dim]Enter: detail | c: mark baseline/compare | Esc: close | " + a.traceHistoryStorageSummary()
		if compareMarked >= 0 && compareMarked < len(entries) {
			text += fmt.Sprintf(" | baseline=%s", entries[compareMarked].CapturedAt.Local().Format("15:04:05"))
		}
		footer.SetText(text + "[white]")
	}
	refreshFooter()

	handleCompare := func() {
		idx := list.GetCurrentItem()
		if idx < 0 || idx >= len(entries) {
			return
		}
		if compareMarked < 0 {
			compareMarked = idx
			a.setStatusNote("Baseline marked. Move to incident row and press c again.", 5*time.Second)
			refreshFooter()
			return
		}
		if compareMarked == idx {
			compareMarked = -1
			a.setStatusNote("Trace compare baseline cleared", 4*time.Second)
			refreshFooter()
			return
		}
		a.showTraceHistoryCompareModal(entries[compareMarked], entries[idx], list)
		compareMarked = -1
		refreshFooter()
	}

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		case tcell.KeyEnter:
			openDetail()
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'c' || event.Rune() == 'C' {
				handleCompare()
				return nil
			}
		}
		return event
	})

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(footer, 1, 0, false)

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(content, 128, 0, true).
			AddItem(nil, 0, 1, false),
			22, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(traceHistoryComparePage)
	a.pages.RemovePage(traceHistoryDetailPage)
	a.pages.RemovePage(traceHistoryPage)
	a.pages.AddPage(traceHistoryPage, modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(list)
}

func (a *App) showTraceHistoryEmptyModal() {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText("  No trace packet history yet\n\n  [dim]" + a.traceHistoryStorageSummary() + "[white]\n  [dim]Press Enter/Esc to close.[white]")
	view.SetBorder(true)
	view.SetTitle(" Trace History ")

	closeModal := func() {
		a.pages.RemovePage(traceHistoryPage)
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 110, 0, true).
			AddItem(nil, 0, 1, false),
			10, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(traceHistoryPage)
	a.pages.AddPage(traceHistoryPage, modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func (a *App) showTraceHistoryDetailModal(entry traceHistoryEntry, backFocus tview.Primitive) {
	body := buildTraceHistoryDetailText(entry, a.sensitiveIP)

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(body)
	view.SetBorder(true)
	view.SetTitle(" Trace History Detail ")

	closeModal := func() {
		a.pages.RemovePage(traceHistoryDetailPage)
		if backFocus != nil {
			a.app.SetFocus(backFocus)
		}
		a.updateStatusBar()
	}
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 120, 0, true).
			AddItem(nil, 0, 1, false),
			26, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(traceHistoryDetailPage)
	a.pages.AddPage(traceHistoryDetailPage, modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func (a *App) showTraceHistoryCompareModal(baseline, incident traceHistoryEntry, backFocus tview.Primitive) {
	body := buildTraceHistoryCompareText(baseline, incident, a.sensitiveIP)

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(body)
	view.SetBorder(true)
	view.SetTitle(" Trace Compare ")

	closeModal := func() {
		a.pages.RemovePage(traceHistoryComparePage)
		if backFocus != nil {
			a.app.SetFocus(backFocus)
		}
		a.updateStatusBar()
	}
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 132, 0, true).
			AddItem(nil, 0, 1, false),
			24, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(traceHistoryComparePage)
	a.pages.AddPage(traceHistoryComparePage, modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func formatTraceHistoryListItem(entry traceHistoryEntry, sensitiveIP bool) (main string, secondary string) {
	ts := entry.CapturedAt.Local().Format("2006-01-02 15:04:05")
	peer := formatPreviewIP(entry.PeerIP, sensitiveIP)
	port := traceHistoryPortLabel(entry.Port)
	color := tracePacketSeverityColor(entry.Severity)
	main = fmt.Sprintf(
		"%s [%s]%s[white] %s | %s:%s",
		ts,
		color,
		blankIfUnknown(entry.Severity, "INFO"),
		shortStatus(maskSensitiveIPsInText(entry.Issue, sensitiveIP), 44),
		peer,
		port,
	)
	secondary = fmt.Sprintf(
		"status=%s mode=%s dir=%s cat=%s scope=%s cap=%s drop=%s rst=%d conf=%s",
		blankIfUnknown(entry.Status, "completed"),
		traceHistoryMode(entry),
		blankIfUnknown(entry.Direction, "ANY"),
		traceHistoryCategory(entry),
		blankIfUnknown(entry.Scope, "Peer + Port"),
		tracePacketMetricDisplay(entry.Captured, entry.CapturedEstimated),
		tracePacketMetricValue(entry.DroppedByKernel),
		entry.RstCount,
		blankIfUnknown(entry.Confidence, "LOW"),
	)
	return main, secondary
}

func buildTraceHistoryDetailText(entry traceHistoryEntry, sensitiveIP bool) string {
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

	peer := formatPreviewIP(entry.PeerIP, sensitiveIP)
	b.WriteString(fmt.Sprintf("  Interface: [green]%s[white]\n", blankIfUnknown(entry.Interface, "n/a")))
	b.WriteString(fmt.Sprintf("  Target: [green]%s:%s[white]\n", peer, traceHistoryPortLabel(entry.Port)))
	b.WriteString(fmt.Sprintf(
		"  Mode: [green]%s[white] | Category: [green]%s[white] | Scope: [green]%s[white] | Direction: [green]%s[white]\n",
		traceHistoryMode(entry),
		traceHistoryCategory(entry),
		blankIfUnknown(entry.Scope, "Peer + Port"),
		blankIfUnknown(entry.Direction, "ANY"),
	))
	b.WriteString(fmt.Sprintf("  Duration: [green]%ds[white] | Packet cap: [green]%d[white]\n", entry.DurationSec, entry.PacketCap))
	b.WriteString(fmt.Sprintf("  Filter: [green]%s[white]\n", maskSensitiveIPsInText(blankIfUnknown(entry.Filter, "n/a"), sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Status: [green]%s[white]\n", blankIfUnknown(entry.Status, "completed")))

	b.WriteString(fmt.Sprintf(
		"  Captured: [green]%s[white] | ReceivedByFilter: [green]%s[white] | DroppedByKernel: [green]%s[white]\n",
		tracePacketMetricDisplay(entry.Captured, entry.CapturedEstimated),
		tracePacketMetricValue(entry.ReceivedByFilter),
		tracePacketMetricValue(entry.DroppedByKernel),
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
	b.WriteString(fmt.Sprintf(
		"  Severity: %s | Confidence: %s\n",
		tracePacketSeverityStyled(blankIfUnknown(entry.Severity, traceSeverityInfo)),
		tracePacketConfidenceStyled(blankIfUnknown(entry.Confidence, "LOW")),
	))
	b.WriteString(fmt.Sprintf("  Issue: %s\n", maskSensitiveIPsInText(blankIfUnknown(entry.Issue, "n/a"), sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Signal: %s\n", maskSensitiveIPsInText(blankIfUnknown(entry.Signal, "n/a"), sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Likely: %s\n", maskSensitiveIPsInText(blankIfUnknown(entry.Likely, "n/a"), sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Check next: %s\n", maskSensitiveIPsInText(blankIfUnknown(entry.Check, "n/a"), sensitiveIP)))

	if entry.Saved && strings.TrimSpace(entry.PCAPPath) != "" {
		b.WriteString(fmt.Sprintf("  PCAP: [aqua]%s[white]\n", maskTracePacketPath(entry.PCAPPath, sensitiveIP)))
	} else {
		b.WriteString("  PCAP: [dim]not saved[white]\n")
	}

	if msg := strings.TrimSpace(entry.CaptureErr); msg != "" {
		b.WriteString(fmt.Sprintf("  [red]Capture error:[white] %s\n", shortStatus(maskSensitiveIPsInText(msg, sensitiveIP), 180)))
	}
	if msg := strings.TrimSpace(entry.ReadErr); msg != "" {
		b.WriteString(fmt.Sprintf("  [yellow]Read warning:[white] %s\n", shortStatus(maskSensitiveIPsInText(msg, sensitiveIP), 180)))
	}

	b.WriteString("\n  [yellow]Sample Packets[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	if len(entry.Sample) == 0 {
		b.WriteString("  [dim]No decoded packet lines.[white]\n")
	} else {
		for _, line := range entry.Sample {
			b.WriteString("  ")
			b.WriteString(maskSensitiveIPsInText(line, sensitiveIP))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n  [dim]Enter/Esc to close.[white]")
	return b.String()
}

func tracePacketSeverityColor(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case traceSeverityCrit:
		return "red"
	case traceSeverityWarn:
		return "yellow"
	default:
		return "green"
	}
}

func tracePacketHistoryCategory(req tracePacketRequest) string {
	switch req.Preset {
	case traceFilterPresetPeerOnly:
		return "Peer only"
	case traceFilterPresetFiveTuple:
		return "5-tuple"
	case traceFilterPresetSynRstOnly:
		return "SYN/RST only"
	case traceFilterPresetCustom:
		return "Custom"
	default:
		return "Peer + Port"
	}
}

func traceHistoryCategory(entry traceHistoryEntry) string {
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

func traceHistoryMode(entry traceHistoryEntry) string {
	mode := strings.TrimSpace(entry.Mode)
	if mode != "" {
		return mode
	}
	return "General triage"
}

func traceHistoryPortLabel(port int) string {
	if port <= 0 {
		return "peer-only"
	}
	return strconv.Itoa(port)
}

func blankIfUnknown(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func (a *App) recentTraceHistory(limit int) []traceHistoryEntry {
	if limit <= 0 {
		limit = traceHistoryModalLimit
	}

	a.traceHistoryMu.Lock()
	defer a.traceHistoryMu.Unlock()
	a.ensureTraceHistoryLoadedLocked()

	total := len(a.traceHistory)
	if total == 0 {
		return nil
	}
	if limit > total {
		limit = total
	}

	out := make([]traceHistoryEntry, 0, limit)
	for i := total - 1; i >= total-limit; i-- {
		out = append(out, a.traceHistory[i])
	}
	return out
}

func defaultTraceHistoryDataDir() string {
	return strings.TrimSpace(history.ExpandPath(history.DefaultDataDir()))
}

func (a *App) traceHistoryDataDirLocked() string {
	dir := strings.TrimSpace(a.traceHistoryDataDir)
	if dir == "" {
		dir = defaultTraceHistoryDataDir()
		a.traceHistoryDataDir = dir
	}
	return dir
}

func (a *App) traceHistoryStorageSummary() string {
	a.traceHistoryMu.Lock()
	defer a.traceHistoryMu.Unlock()

	dir := a.traceHistoryDataDirLocked()
	if strings.TrimSpace(dir) == "" {
		return fmt.Sprintf("trace history storage unavailable | retention=%dh", history.DefaultRetentionHours())
	}
	return fmt.Sprintf(
		"data-dir=%s | file=%sYYYYMMDD%s | retention=%dh",
		dir,
		traceHistorySegmentPrefix,
		traceHistorySegmentSuffix,
		history.DefaultRetentionHours(),
	)
}

func (a *App) ensureTraceHistoryLoadedLocked() {
	if a.traceHistoryLoaded {
		return
	}
	dir := a.traceHistoryDataDirLocked()
	loaded, err := readTraceHistoryEntriesFromDir(dir)
	if err != nil {
		a.traceHistory = nil
	} else {
		a.traceHistory = loaded
	}
	a.traceHistoryLoaded = true
}

// Caller must hold a.traceHistoryMu.
func (a *App) persistTraceHistoryEntryLocked(entry traceHistoryEntry) {
	dir := a.traceHistoryDataDirLocked()
	if strings.TrimSpace(dir) == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	path := filepath.Join(dir, traceHistorySegmentFileName(entry.CapturedAt))
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(append(raw, '\n'))
}

// Caller must hold a.traceHistoryMu.
func (a *App) pruneTraceHistorySegmentsByAgeLocked(now time.Time) {
	dir := a.traceHistoryDataDirLocked()
	if strings.TrimSpace(dir) == "" {
		return
	}
	retention := history.DefaultRetentionHours()
	if retention < 1 {
		return
	}
	_ = pruneTraceHistoryDataDirByAge(dir, retention, now)
}

func traceHistorySegmentFileName(t time.Time) string {
	stamp := t.Local().Format(traceHistorySegmentLayout)
	return traceHistorySegmentPrefix + stamp + traceHistorySegmentSuffix
}

type traceHistorySegmentFile struct {
	Path      string
	Name      string
	Timestamp time.Time
	Span      time.Duration
}

func parseTraceHistorySegmentWindow(name string) (time.Time, time.Duration, bool) {
	if !strings.HasPrefix(name, traceHistorySegmentPrefix) || !strings.HasSuffix(name, traceHistorySegmentSuffix) {
		return time.Time{}, 0, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, traceHistorySegmentPrefix), traceHistorySegmentSuffix)
	ts, err := time.ParseInLocation(traceHistorySegmentLayout, stamp, time.Local)
	if err != nil {
		return time.Time{}, 0, false
	}
	return ts, 24 * time.Hour, true
}

func listTraceHistorySegmentFiles(dataDir string) ([]traceHistorySegmentFile, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]traceHistorySegmentFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ts, span, ok := parseTraceHistorySegmentWindow(entry.Name())
		if !ok {
			continue
		}
		items = append(items, traceHistorySegmentFile{
			Path:      filepath.Join(dataDir, entry.Name()),
			Name:      entry.Name(),
			Timestamp: ts,
			Span:      span,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].Timestamp.Equal(items[j].Timestamp) {
			return items[i].Timestamp.Before(items[j].Timestamp)
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func readTraceHistoryEntriesFromDir(dataDir string) ([]traceHistoryEntry, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, nil
	}
	files, err := listTraceHistorySegmentFiles(dataDir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	all := make([]traceHistoryEntry, 0, 128)
	for _, file := range files {
		entries, err := readTraceHistoryEntries(file.Path)
		if err != nil {
			continue
		}
		all = append(all, entries...)
	}
	if len(all) == 0 {
		return nil, nil
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].CapturedAt.Before(all[j].CapturedAt)
	})
	return all, nil
}

func pruneTraceHistoryDataDirByAge(dataDir string, retentionHours int, now time.Time) error {
	if retentionHours < 1 {
		return nil
	}
	files, err := listTraceHistorySegmentFiles(dataDir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	now = now.Local()
	cutoff := now.Add(-time.Duration(retentionHours) * time.Hour)
	currentFile := traceHistorySegmentFileName(now)

	for _, file := range files {
		if file.Name == currentFile {
			continue
		}
		segmentEnd := file.Timestamp
		if file.Span > 0 {
			segmentEnd = segmentEnd.Add(file.Span)
		}
		if segmentEnd.Before(cutoff) {
			_ = os.Remove(file.Path)
		}
	}
	return nil
}

func readTraceHistoryEntries(path string) ([]traceHistoryEntry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	entries := make([]traceHistoryEntry, 0, 64)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry traceHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.CapturedAt.IsZero() {
			if !entry.EndedAt.IsZero() {
				entry.CapturedAt = entry.EndedAt
			} else {
				entry.CapturedAt = entry.StartedAt
			}
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
