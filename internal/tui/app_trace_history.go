package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	traceHistoryPage        = "trace-history"
	traceHistoryDetailPage  = "trace-history-detail"
	traceHistoryComparePage = "trace-history-compare"
	traceHistorySampleMax   = 8
)

func (a *App) appendTraceHistory(result tracePacketResult) {
	entry := newTraceHistoryEntry(result)
	a.traceEngine.Lock()
	defer a.traceEngine.Unlock()
	a.ensureTraceHistoryLoadedLocked()
	a.traceEngine.AppendEntryLocked(entry)
	a.persistTraceHistoryEntryLocked(entry)
	a.pruneTraceHistorySegmentsByAgeLocked(entry.CapturedAt)
}

func newTraceHistoryEntry(result tracePacketResult) tuitrace.Entry {
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

	entry := tuitrace.Entry{
		CapturedAt: capturedAt,
		StartedAt:  result.StartedAt,
		EndedAt:    result.EndedAt,

		Interface:   result.Request.Interface,
		PeerIP:      tuishared.NormalizeIP(result.Request.PeerIP),
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

func (a *App) showTraceHistoryDetailModal(entry tuitrace.Entry, backFocus tview.Primitive) {
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

func (a *App) showTraceHistoryCompareModal(baseline, incident tuitrace.Entry, backFocus tview.Primitive) {
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

func traceRenderOptions(sensitiveIP bool) tuitrace.RenderOptions {
	return tuitrace.RenderOptions{
		SensitiveIP:       sensitiveIP,
		SeverityInfo:      traceSeverityInfo,
		FormatPreviewIP:   tuishared.FormatPreviewIP,
		MaskSensitiveText: maskSensitiveIPsInText,
		ShortStatus:       shortStatus,
		MaskPath:          maskTracePacketPath,
		MetricDisplay:     tracePacketMetricDisplay,
		MetricValue:       tracePacketMetricValue,
		SeverityStyled:    tracePacketSeverityStyled,
		ConfidenceStyled:  tracePacketConfidenceStyled,
	}
}

func formatTraceHistoryListItem(entry tuitrace.Entry, sensitiveIP bool) (main string, secondary string) {
	return tuitrace.FormatListItem(entry, traceRenderOptions(sensitiveIP))
}

func buildTraceHistoryDetailText(entry tuitrace.Entry, sensitiveIP bool) string {
	return tuitrace.BuildDetailText(entry, traceRenderOptions(sensitiveIP))
}

func buildTraceHistoryCompareText(baseline, incident tuitrace.Entry, sensitiveIP bool) string {
	return tuitrace.BuildCompareText(baseline, incident, traceRenderOptions(sensitiveIP))
}

func tracePacketSeverityColor(severity string) string {
	return tuitrace.SeverityColor(severity, traceSeverityInfo)
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

func traceHistoryCategory(entry tuitrace.Entry) string {
	return tuitrace.Category(entry)
}

func traceHistoryMode(entry tuitrace.Entry) string {
	return tuitrace.Mode(entry)
}

func traceHistoryPortLabel(port int) string {
	return tuitrace.PortLabel(port)
}

func blankIfUnknown(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

// --- Trace Packet Analyzer ---

const (
	traceSeverityInfo = "INFO"
	traceSeverityWarn = "WARN"
	traceSeverityCrit = "CRIT"
)

type tracePacketDiagnosis struct {
	Severity   string
	Confidence string
	Issue      string
	Signal     string
	Likely     string
	Check      string
}

func analyzeTracePacket(result tracePacketResult) tracePacketDiagnosis {
	confidence := tracePacketConfidence(result.DecodedPackets)
	dropValue := tracePacketMetricValue(result.DroppedByKernel)
	baseSignal := fmt.Sprintf(
		"Decoded %d | SYN %d | SYN-ACK %d | RST %d | Drop %s",
		result.DecodedPackets,
		result.SynCount,
		result.SynAckCount,
		result.RstCount,
		dropValue,
	)

	if result.CaptureErr != nil {
		return tracePacketDiagnosis{
			Severity:   traceSeverityCrit,
			Confidence: "HIGH",
			Issue:      "Capture failed",
			Signal:     baseSignal,
			Likely:     "tcpdump exited with an error before a reliable sample was collected.",
			Check:      "Verify privileges, interface name, and tcpdump availability on host.",
		}
	}

	if result.DroppedByKernel > 0 {
		capturedForRatio := result.Captured
		if capturedForRatio < 0 {
			capturedForRatio = result.DecodedPackets
		}
		totalObserved := capturedForRatio + result.DroppedByKernel
		dropRatio := 0.0
		if totalObserved > 0 {
			dropRatio = float64(result.DroppedByKernel) / float64(totalObserved)
		}

		severity := traceSeverityWarn
		if dropRatio >= 0.10 || result.DroppedByKernel >= 100 {
			severity = traceSeverityCrit
		}
		return tracePacketDiagnosis{
			Severity:   severity,
			Confidence: confidence,
			Issue:      "tcpdump dropped packets",
			Signal: fmt.Sprintf(
				"%s | drop ratio %.1f%%",
				baseSignal,
				dropRatio*100,
			),
			Likely: "capture path is overloaded, so packet sample can be incomplete.",
			Check:  "Reduce scope/duration, retry at lower host load, or increase tcpdump buffer (-B) if needed.",
		}
	}

	if result.DecodedPackets > 0 && result.RstCount >= 5 {
		rstRatio := float64(result.RstCount) / float64(result.DecodedPackets)
		if rstRatio >= 0.20 {
			severity := traceSeverityWarn
			if rstRatio >= 0.40 || result.RstCount >= 20 {
				severity = traceSeverityCrit
			}
			return tracePacketDiagnosis{
				Severity:   severity,
				Confidence: confidence,
				Issue:      "RST pressure",
				Signal: fmt.Sprintf(
					"%s | RST ratio %.1f%%",
					baseSignal,
					rstRatio*100,
				),
				Likely: "connections are being actively reset by peer, middlebox, or local policy.",
				Check:  "Inspect app logs and firewall reject/reset rules on both ends.",
			}
		}
	}

	if result.SynCount >= 3 {
		if result.SynAckCount == 0 {
			severity := traceSeverityWarn
			if result.SynCount >= 10 {
				severity = traceSeverityCrit
			}
			return tracePacketDiagnosis{
				Severity:   severity,
				Confidence: confidence,
				Issue:      "SYN seen but no SYN-ACK",
				Signal:     baseSignal,
				Likely:     "return path, firewall policy, or peer reachability may be blocking handshake replies.",
				Check:      "Verify route/firewall/NAT/security-group between source and destination.",
			}
		}
		synAckRatio := float64(result.SynAckCount) / float64(result.SynCount)
		if synAckRatio < 0.40 {
			return tracePacketDiagnosis{
				Severity:   traceSeverityWarn,
				Confidence: confidence,
				Issue:      "Low SYN-ACK ratio",
				Signal: fmt.Sprintf(
					"%s | SYN-ACK ratio %.1f%%",
					baseSignal,
					synAckRatio*100,
				),
				Likely: "only part of handshake attempts receive replies.",
				Check:  "Check intermittent path loss, policy throttling, or peer-side saturation.",
			}
		}
	}

	if result.DecodedPackets < 10 {
		return tracePacketDiagnosis{
			Severity:   traceSeverityInfo,
			Confidence: confidence,
			Issue:      "Low packet sample",
			Signal:     baseSignal,
			Likely:     "sample size is too small for a strong packet-level conclusion.",
			Check:      "Increase duration/packet cap and capture during the problematic window.",
		}
	}

	return tracePacketDiagnosis{
		Severity:   traceSeverityInfo,
		Confidence: confidence,
		Issue:      "No strong packet-level anomaly",
		Signal:     baseSignal,
		Likely:     "handshake/reset signals look stable in this bounded capture.",
		Check:      "Correlate with Connection States, Interface Stats, and Conntrack panels for host-level context.",
	}
}

func tracePacketConfidence(decodedPackets int) string {
	switch {
	case decodedPackets >= 100:
		return "HIGH"
	case decodedPackets >= 30:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func tracePacketSeverityStyled(severity string) string {
	switch severity {
	case traceSeverityCrit:
		return "[red]CRIT[white]"
	case traceSeverityWarn:
		return "[yellow]WARN[white]"
	default:
		return "[green]INFO[white]"
	}
}

func tracePacketConfidenceStyled(confidence string) string {
	switch confidence {
	case "HIGH":
		return "[green]HIGH[white]"
	case "MEDIUM":
		return "[yellow]MEDIUM[white]"
	default:
		return "[aqua]LOW[white]"
	}
}

func (a *App) recentTraceHistory(limit int) []tuitrace.Entry {
	if limit <= 0 {
		limit = traceHistoryModalLimit
	}

	a.traceEngine.Lock()
	defer a.traceEngine.Unlock()
	a.ensureTraceHistoryLoadedLocked()

	total := len(a.traceEngine.HistoryLocked())
	if total == 0 {
		return nil
	}
	if limit > total {
		limit = total
	}

	out := make([]tuitrace.Entry, 0, limit)
	for i := total - 1; i >= total-limit; i-- {
		out = append(out, a.traceEngine.HistoryLocked()[i])
	}
	return out
}

func defaultTraceHistoryDataDir() string {
	return strings.TrimSpace(history.ExpandPath(history.DefaultDataDir()))
}

func (a *App) traceHistoryDataDirLocked() string {
	dir := strings.TrimSpace(a.traceEngine.HistoryDataDirLocked())
	if dir == "" {
		dir = defaultTraceHistoryDataDir()
		a.traceEngine.SetHistoryDataDirLocked(dir)
	}
	return dir
}

func (a *App) traceHistoryStorageSummary() string {
	a.traceEngine.Lock()
	defer a.traceEngine.Unlock()

	dir := a.traceHistoryDataDirLocked()
	if strings.TrimSpace(dir) == "" {
		return fmt.Sprintf("trace history storage unavailable | retention=%dh", history.DefaultRetentionHours())
	}
	return fmt.Sprintf(
		"data-dir=%s | file=%sYYYYMMDD%s | retention=%dh",
		dir,
		tuitrace.SegmentPrefix,
		tuitrace.SegmentSuffix,
		history.DefaultRetentionHours(),
	)
}

func (a *App) ensureTraceHistoryLoadedLocked() {
	if a.traceEngine.IsHistoryLoadedLocked() {
		return
	}
	dir := a.traceHistoryDataDirLocked()
	loaded, err := tuitrace.ReadEntriesFromDir(dir)
	if err != nil {
		a.traceEngine.SetHistoryLocked(nil)
	} else {
		a.traceEngine.SetHistoryLocked(loaded)
	}
	a.traceEngine.MarkHistoryLoadedLocked()
}

// Caller must hold a.traceHistoryMu.
func (a *App) persistTraceHistoryEntryLocked(entry tuitrace.Entry) {
	dir := a.traceHistoryDataDirLocked()
	if strings.TrimSpace(dir) == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	path := filepath.Join(dir, tuitrace.SegmentFileName(entry.CapturedAt))
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
	_ = tuitrace.PruneDataDirByAge(dir, retention, now)
}
