package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/history"
	tuilayout "github.com/BlackMetalz/holyf-network/internal/tui/layout"
	tuioverlays "github.com/BlackMetalz/holyf-network/internal/tui/overlays"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuireplay "github.com/BlackMetalz/holyf-network/internal/tui/replay"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type replayViewMode int

const (
	replayViewConnections replayViewMode = iota
	replayViewTrace
)

func (m replayViewMode) Label() string {
	if m == replayViewTrace {
		return "TRACE"
	}
	return "CONN"
}

// HistoryApp is read-only replay UI for persisted connection snapshots.
type HistoryApp struct {
	app       *tview.Application
	pages     *tview.Pages
	panel     *tview.TextView
	statusBar *tview.TextView
	layout    *tview.Flex

	dataDir     string
	segmentFile string
	rangeBegin  *time.Time
	rangeEnd    *time.Time
	sensitiveIP bool
	appVersion  string

	refs          []history.SnapshotRef
	currentIndex  int
	currentRecord history.SnapshotRecord

	filesCount         int
	corruptSkipped     int
	lastCorruptNoticed int

	portFilter    string
	textFilter    string
	sortMode      tuishared.SortMode
	sortDesc      bool
	topDirection  tuishared.TopConnectionDirection
	skipEmpty     bool
	selectedIndex int
	followLatest  bool

	timelineSearchQuery    string
	timelineSearchResults  []tuireplay.SearchResult
	timelineSearchSelected int
	timelineSearchRunning  bool
	replayViewMode         replayViewMode

	traceOnlyMode           bool
	traceReplayEntries      []tuitrace.Entry
	traceTimelineBySnapshot map[int][]tuitrace.Entry
	traceTimelineTotal      int
	traceTimelineAssociated int
	traceTimelineWindow     time.Duration

	statusNote       string
	statusNoteUntil  time.Time
	lastStatusNote   string
	snapshotMessage  string
	healthThresholds config.HealthThresholds

	stopChan chan struct{}
}

func NewHistoryApp(dataDir, segmentFile string, sensitiveIP bool, appVersion string, rangeBegin, rangeEnd *time.Time) *HistoryApp {
	version := strings.TrimSpace(appVersion)
	if version == "" {
		version = "dev"
	}

	return &HistoryApp{
		app:              tview.NewApplication(),
		dataDir:          history.ExpandPath(dataDir),
		segmentFile:      strings.TrimSpace(segmentFile),
		rangeBegin:       cloneOptionalTime(rangeBegin),
		rangeEnd:         cloneOptionalTime(rangeEnd),
		sensitiveIP:      sensitiveIP,
		appVersion:       version,
		sortMode:         tuishared.SortByBandwidth,
		sortDesc:         true,
		topDirection:     tuishared.TopConnectionIncoming,
		skipEmpty:        true,
		currentIndex:     -1,
		replayViewMode:   replayViewConnections,
		stopChan:         make(chan struct{}),
		healthThresholds: config.DefaultHealthThresholds(),
	}
}

func (h *HistoryApp) Run() error {
	h.panel = tuilayout.CreateHistoryPanel()
	h.statusBar = tuilayout.CreateHistoryStatusBar()
	h.layout = tuilayout.CreateHistoryLayout(h.panel, h.statusBar)
	h.pages = tview.NewPages()
	h.pages.AddPage("main", h.layout, true, true)
	historyHelpModal, _ := tuioverlays.CreateCenteredTextViewModal(" Replay Help ", tuireplay.HelpText)
	h.pages.AddPage("history-help", historyHelpModal, true, false)

	h.panel.SetBorderColor(tcell.ColorYellow)
	h.panel.SetTitleColor(tcell.ColorYellow)

	h.reloadIndex(true)
	h.renderPanel()
	h.updateStatusBar()

	h.app.SetInputCapture(h.handleKeyEvent)
	go h.startTickerLoop()

	h.app.SetRoot(h.pages, true)
	return h.app.Run()
}

func (h *HistoryApp) startTickerLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ticks := 0
	for {
		select {
		case <-ticker.C:
			ticks++
			h.app.QueueUpdateDraw(func() {
				if h.followLatest && ticks%3 == 0 {
					h.reloadIndex(false)
					h.renderPanel()
				}
				h.updateStatusBar()
			})
		case <-h.stopChan:
			return
		}
	}
}

func (h *HistoryApp) reloadIndex(selectStart bool) {
	var (
		refs  []history.SnapshotRef
		stats history.IndexStats
		err   error
	)
	if strings.TrimSpace(h.segmentFile) != "" {
		refs, stats, err = history.LoadIndexFromFile(h.dataDir, h.segmentFile)
	} else {
		refs, stats, err = history.LoadIndex(h.dataDir)
	}
	if err != nil {
		h.setStatusNote("Load snapshots failed: "+shortStatus(err.Error(), 72), 8*time.Second)
		return
	}
	refs = tuireplay.FilterSnapshotRefsByRange(refs, h.rangeBegin, h.rangeEnd)
	h.traceOnlyMode = false
	h.traceReplayEntries = nil
	if len(refs) == 0 {
		if traceRefs, traceEntries, err := tuireplay.LoadTraceOnlyRefs(h.dataDir, h.rangeBegin, h.rangeEnd); err == nil && len(traceRefs) > 0 {
			refs = traceRefs
			h.traceOnlyMode = true
			h.traceReplayEntries = traceEntries
			h.replayViewMode = replayViewTrace
			h.setStatusNote(fmt.Sprintf("Trace-only replay mode (%d events)", len(traceEntries)), 6*time.Second)
		}
	}

	prevFile := ""
	prevOffset := int64(-1)
	if h.currentIndex >= 0 && h.currentIndex < len(h.refs) {
		prev := h.refs[h.currentIndex]
		prevFile = prev.FilePath
		prevOffset = prev.Offset
	}

	h.refs = refs
	h.filesCount = stats.Files
	h.corruptSkipped = stats.Corrupt
	tuireplay.RebuildTraceTimeline(h)

	if stats.Corrupt > 0 && stats.Corrupt != h.lastCorruptNoticed {
		h.lastCorruptNoticed = stats.Corrupt
		h.setStatusNote(fmt.Sprintf("Skipped %d corrupt snapshots", stats.Corrupt), 6*time.Second)
	}

	if len(refs) == 0 {
		h.currentIndex = -1
		h.currentRecord = history.SnapshotRecord{}
		h.selectedIndex = 0
		return
	}

	target := h.currentIndex
	if selectStart {
		target = 0
		target = h.adjustStartIndexForSkipEmpty(target)
	} else if h.followLatest {
		target = h.adjustLatestIndexForSkipEmpty(len(h.refs) - 1)
	} else if prevFile != "" {
		idx := -1
		for i, ref := range h.refs {
			if ref.FilePath == prevFile && ref.Offset == prevOffset {
				idx = i
				break
			}
		}
		if idx >= 0 {
			target = idx
		}
	}

	if target < 0 || target >= len(h.refs) {
		if selectStart {
			target = 0
		} else {
			target = len(h.refs) - 1
		}
	}
	target = h.adjustGenericIndexForSkipEmpty(target)

	h.loadSnapshotAt(target)
}

func (h *HistoryApp) loadSnapshotAt(index int) {
	if len(h.refs) == 0 {
		h.currentIndex = -1
		h.currentRecord = history.SnapshotRecord{}
		h.selectedIndex = 0
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(h.refs) {
		index = len(h.refs) - 1
	}

	if h.traceOnlyMode {
		h.currentIndex = index
		h.currentRecord = history.SnapshotRecord{
			CapturedAt:      h.refs[index].CapturedAt,
			Interface:       "trace-history",
			TopLimitPerSide: 0,
			IncomingGroups:  []history.SnapshotGroup{},
			OutgoingGroups:  []history.SnapshotGroup{},
			Version:         h.appVersion,
		}
		h.selectedIndex = 0
		return
	}

	record, err := history.ReadSnapshot(h.refs[index])
	if err != nil {
		h.setStatusNote("Read snapshot failed: "+shortStatus(err.Error(), 72), 8*time.Second)
		return
	}

	h.currentIndex = index
	h.currentRecord = record
	h.selectedIndex = 0
}

func (h *HistoryApp) topDisplayLimit() int {
	if h.panel == nil {
		return 20
	}
	_, _, _, height := h.panel.GetInnerRect()
	if height <= 0 {
		return 20
	}
	limit := height - 8
	if limit < 5 {
		return 5
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func (h *HistoryApp) rowsForDirection(record history.SnapshotRecord, direction tuishared.TopConnectionDirection) []history.SnapshotGroup {
	history.NormalizeSnapshotRecord(&record)
	if len(record.IncomingGroups) == 0 && len(record.OutgoingGroups) == 0 && len(record.Groups) > 0 {
		return record.Groups
	}
	if direction == tuishared.TopConnectionOutgoing {
		return record.OutgoingGroups
	}
	return record.IncomingGroups
}

func (h *HistoryApp) currentRows() []history.SnapshotGroup {
	return h.rowsForDirection(h.currentRecord, h.topDirection)
}

func (h *HistoryApp) refCountForDirection(ref history.SnapshotRef, direction tuishared.TopConnectionDirection) int {
	if h.traceOnlyMode {
		return 1
	}
	if ref.IncomingCount == 0 && ref.OutgoingCount == 0 && ref.ConnCount > 0 {
		return ref.ConnCount
	}
	if direction == tuishared.TopConnectionOutgoing {
		return ref.OutgoingCount
	}
	return ref.IncomingCount
}

func (h *HistoryApp) currentRefCount(ref history.SnapshotRef) int {
	return h.refCountForDirection(ref, h.topDirection)
}

func (h *HistoryApp) visibleRows() []history.SnapshotGroup {
	rows := h.currentRows()
	if len(rows) == 0 {
		return nil
	}
	filtered := rows
	if strings.TrimSpace(h.portFilter) != "" {
		filtered = tuipanels.FilterHistoryGroupsByPort(filtered, h.portFilter)
	}
	if strings.TrimSpace(h.textFilter) != "" {
		filtered = tuipanels.FilterHistoryGroupsByText(filtered, h.textFilter)
	}
	if len(filtered) == 0 {
		return nil
	}

	items := append([]history.SnapshotGroup(nil), filtered...)
	tuipanels.SortHistoryGroups(items, h.sortMode, h.sortDesc)

	limit := h.topDisplayLimit()
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (h *HistoryApp) visibleCount() int {
	return len(h.visibleRows())
}

func (h *HistoryApp) clampSelection() {
	count := h.visibleCount()
	if count <= 0 {
		h.selectedIndex = 0
		return
	}
	if h.selectedIndex < 0 {
		h.selectedIndex = 0
		return
	}
	if h.selectedIndex >= count {
		h.selectedIndex = count - 1
	}
}

func (h *HistoryApp) moveSelection(delta int) bool {
	count := h.visibleCount()
	if count == 0 {
		return false
	}
	h.clampSelection()
	next := h.selectedIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= count {
		next = count - 1
	}
	if next == h.selectedIndex {
		return true
	}
	h.selectedIndex = next
	h.renderPanel()
	h.updateStatusBar()
	return true
}

func (h *HistoryApp) renderPanel() {
	if h.panel == nil {
		return
	}

	if len(h.refs) == 0 || h.currentIndex < 0 || h.currentIndex >= len(h.refs) {
		if h.hasReplayRange() {
			h.panel.SetText(
				"  [yellow]No snapshots in selected time range[white]\n\n" +
					fmt.Sprintf("  Scope: [aqua]%s[white]\n", h.replayScopeLabel()) +
					fmt.Sprintf("  Range: [aqua]%s[white]\n\n", h.replayRangeLabel()) +
					fmt.Sprintf("  Data dir: [dim]%s[white]", h.dataDir),
			)
			return
		}
		h.panel.SetText(
			"  [yellow]No snapshots found[white]\n\n" +
				"  Run daemon first:\n" +
				"  [aqua]holyf-network daemon start[white]\n\n" +
				fmt.Sprintf("  Data dir: [dim]%s[white]", h.dataDir),
		)
		return
	}

	h.clampSelection()

	rec := h.currentRecord
	history.NormalizeSnapshotRecord(&rec)
	captured := rec.CapturedAt.Local().Format("2006-01-02 15:04:05 -07")
	iface := rec.Interface
	if strings.TrimSpace(iface) == "" {
		iface = "unknown"
	}

	var headerParts []string
	headerParts = append(headerParts, h.headerSnapshotLabel())
	headerParts = append(headerParts, captured)
	headerParts = append(headerParts, iface)
	headerParts = append(headerParts, fmt.Sprintf("IN:%d OUT:%d", len(rec.IncomingGroups), len(rec.OutgoingGroups)))
	if rec.BandwidthAvailable && rec.SampleSeconds > 0 {
		headerParts = append(headerParts, fmt.Sprintf("BW:%.0fs", rec.SampleSeconds))
	}
	scopeLabel := h.replayScopeLabel()
	if scopeLabel != "ALL" {
		headerParts = append(headerParts, fmt.Sprintf("scope=%s", scopeLabel))
	}
	rangeLabel := h.replayRangeLabel()
	if rangeLabel != "ALL" {
		headerParts = append(headerParts, fmt.Sprintf("range=%s", rangeLabel))
	}
	header := fmt.Sprintf("  [dim]%s[white]\n", strings.Join(headerParts, " | "))
	if strings.TrimSpace(h.snapshotMessage) != "" {
		header += fmt.Sprintf("  [yellow]%s[white]\n", shortStatus(h.snapshotMessage, 160))
	}
	traceSection := tuireplay.RenderCurrentTraceTimelineSection(h)
	traceOnlyHint := ""
	if h.traceOnlyMode {
		traceOnlyHint = "  [yellow]Trace-only replay mode[white] (no connection snapshots in selected scope/range)\n\n"
	}
	if h.replayViewMode == replayViewTrace {
		h.panel.SetText(header + "\n" + traceOnlyHint + traceSection + tuireplay.RenderTraceReplayViewBody(h))
		return
	}

	if len(h.currentRows()) == 0 {
		if h.traceOnlyMode {
			h.panel.SetText(header + "\n" + traceOnlyHint + traceSection + "  [dim]Use [ / ] to move between trace events. Snapshot rows are unavailable in this mode.[white]")
			return
		}
		start, end, count, approx := h.idleStreak()
		idleLine := fmt.Sprintf("  [dim]Idle streak: %d snapshots[white]", count)
		if approx > 0 {
			idleLine = fmt.Sprintf("  [dim]Idle streak: %d snapshots (~%s)[white]", count, formatApproxDuration(approx))
		}
		rangeLine := ""
		if count > 0 && start >= 0 && end >= 0 && start < len(h.refs) && end < len(h.refs) {
			rangeLine = fmt.Sprintf(
				"\n  [dim]Range: %s -> %s[white]",
				h.refs[start].CapturedAt.Local().Format("15:04:05"),
				h.refs[end].CapturedAt.Local().Format("15:04:05"),
			)
		}
		emptyBody := fmt.Sprintf(
			"  [yellow]No active %s groups at this snapshot[white]\n\n%s%s\n\n  [dim]Use left/right bracket to move active snapshots | x=toggle skip-empty[white]",
			h.topDirection.Label(),
			idleLine,
			rangeLine,
		)
		h.panel.SetText(header + "\n" + traceSection + emptyBody)
		return
	}

	body := tuipanels.RenderHistoryAggregatePanel(
		h.currentRows(),
		h.portFilter,
		h.textFilter,
		h.topDisplayLimit(),
		h.sensitiveIP,
		h.selectedIndex,
		h.sortMode,
		h.sortDesc,
		h.topDirection,
		h.skipEmpty,
		h.healthThresholds,
		rec.BandwidthAvailable,
	)

	h.panel.SetText(header + "\n" + traceSection + body)
}

func (h *HistoryApp) replayScopeLabel() string {
	file := strings.TrimSpace(h.segmentFile)
	if file == "" {
		return "ALL"
	}
	return filepath.Base(file)
}

func (h *HistoryApp) replayRangeLabel() string {
	begin := "ALL"
	if h.rangeBegin != nil {
		begin = h.rangeBegin.Local().Format("2006-01-02 15:04:05")
	}
	end := "ALL"
	if h.rangeEnd != nil {
		end = h.rangeEnd.Local().Format("2006-01-02 15:04:05")
	}

	if h.rangeBegin != nil && h.rangeEnd != nil {
		return begin + ".." + end
	}
	if h.rangeBegin != nil {
		return ">=" + begin
	}
	if h.rangeEnd != nil {
		return "<=" + end
	}
	return "ALL"
}

func (h *HistoryApp) hasReplayRange() bool {
	return h.rangeBegin != nil || h.rangeEnd != nil
}

func (h *HistoryApp) filterRefsByReplayRange(refs []history.SnapshotRef) []history.SnapshotRef {
	if !h.hasReplayRange() || len(refs) == 0 {
		return refs
	}

	filtered := make([]history.SnapshotRef, 0, len(refs))
	for _, ref := range refs {
		if h.rangeBegin != nil && ref.CapturedAt.Before(*h.rangeBegin) {
			continue
		}
		if h.rangeEnd != nil && ref.CapturedAt.After(*h.rangeEnd) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func cloneOptionalTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	tt := *t
	return &tt
}

func (h *HistoryApp) updateStatusBar() {
	if h.statusBar == nil {
		return
	}
	page := h.frontPageName()
	hotkeysStyled, _ := tuireplay.StatusHotkeysForPage(page)

	snapshotPart := "Snapshot: 0/0"
	if len(h.refs) > 0 && h.currentIndex >= 0 {
		if h.traceOnlyMode {
			snapshotPart = fmt.Sprintf("Trace: %d/%d", h.currentIndex+1, len(h.refs))
		} else if h.skipEmpty {
			activePos, activeTotal := h.activeTimelinePosition()
			snapshotPart = fmt.Sprintf("Active: %d/%d Raw: %d/%d", activePos, activeTotal, h.currentIndex+1, len(h.refs))
		} else {
			snapshotPart = fmt.Sprintf("Snapshot: %d/%d", h.currentIndex+1, len(h.refs))
		}
	}

	// Show daemon process CPU/memory from snapshot if available
	usagePart := ""
	if h.currentRecord.RSSBytes > 0 {
		rss := float64(h.currentRecord.RSSBytes) / (1024 * 1024)
		if h.currentRecord.CPUCores > 0 {
			usagePart = fmt.Sprintf(" | [aqua]CPU:%.2fc RSS:%.1fMB[white]", h.currentRecord.CPUCores, rss)
		} else {
			usagePart = fmt.Sprintf(" | [aqua]RSS:%.1fMB[white]", rss)
		}
	}

	followState := "TAIL-OFF"
	followColor := "dim"
	if h.followLatest {
		followState = "TAIL-ON"
		followColor = "green"
	}

	notePart := ""
	if time.Now().Before(h.statusNoteUntil) && h.statusNote != "" {
		notePart = fmt.Sprintf(" | [yellow]%s[white]", h.statusNote)
	} else if strings.TrimSpace(h.lastStatusNote) != "" {
		notePart = fmt.Sprintf(" | [dim]%s[white]", shortStatus(h.lastStatusNote, 72))
	}

	corruptPart := ""
	if h.corruptSkipped > 0 {
		corruptPart = fmt.Sprintf(" | [red]Corrupt:%d[white]", h.corruptSkipped)
	}

	tracePart := ""
	if h.traceTimelineAssociated > 0 {
		tracePart = fmt.Sprintf(" | [aqua]TRACE %d/%d[white]", tuireplay.CurrentTraceTimelineCount(h), h.traceTimelineAssociated)
	}

	h.statusBar.SetText(fmt.Sprintf(
		" [yellow]replay[white] | %s%s%s%s%s | Files:%d | [%s]%s[white] | %s | [dim]%s[white]",
		snapshotPart,
		notePart,
		usagePart,
		tracePart,
		corruptPart,
		h.filesCount,
		followColor,
		followState,
		hotkeysStyled,
		h.appVersion,
	))
}

func (h *HistoryApp) frontPageName() string {
	if h.pages == nil {
		return "main"
	}
	name, _ := h.pages.GetFrontPage()
	if strings.TrimSpace(name) == "" {
		return "main"
	}
	return name
}

func (h *HistoryApp) setStatusNote(note string, ttl time.Duration) {
	h.statusNote = strings.TrimSpace(note)
	if h.statusNote != "" {
		h.lastStatusNote = h.statusNote
	}
	h.statusNoteUntil = time.Now().Add(ttl)
	h.updateStatusBar()
}

func (h *HistoryApp) setSnapshotMessage(msg string) {
	h.snapshotMessage = strings.TrimSpace(msg)
}

func (h *HistoryApp) headerSnapshotLabel() string {
	if len(h.refs) == 0 || h.currentIndex < 0 || h.currentIndex >= len(h.refs) {
		return "Snapshot 0/0"
	}
	if h.traceOnlyMode {
		return fmt.Sprintf("Trace Event %d/%d", h.currentIndex+1, len(h.refs))
	}
	if !h.skipEmpty {
		return fmt.Sprintf("Snapshot %d/%d", h.currentIndex+1, len(h.refs))
	}
	activePos, activeTotal := h.activeTimelinePosition()
	return fmt.Sprintf("Snapshot Active %d/%d | Raw %d/%d", activePos, activeTotal, h.currentIndex+1, len(h.refs))
}

func (h *HistoryApp) activeTimelinePosition() (int, int) {
	if len(h.refs) == 0 || h.currentIndex < 0 || h.currentIndex >= len(h.refs) {
		return 0, 0
	}

	total := 0
	pos := 0
	for i, ref := range h.refs {
		if h.currentRefCount(ref) <= 0 {
			continue
		}
		total++
		if i == h.currentIndex {
			pos = total
		}
	}
	if total == 0 {
		return 0, 0
	}
	if pos == 0 {
		// Current snapshot can be empty when there are no active rows nearby.
		return 0, total
	}
	return pos, total
}

func (h *HistoryApp) hasSnapshotsOnLocalDate(target time.Time) bool {
	y, m, d := target.Local().Date()
	for _, ref := range h.refs {
		ry, rm, rd := ref.CapturedAt.Local().Date()
		if y == ry && m == rm && d == rd {
			return true
		}
	}
	return false
}

func (h *HistoryApp) buildJumpSummary(requested time.Time, index int) string {
	if index < 0 || index >= len(h.refs) {
		return ""
	}
	actual := h.refs[index].CapturedAt.Local().Format("2006-01-02 15:04:05")
	base := fmt.Sprintf("Jumped to %s (snapshot %d/%d).", actual, index+1, len(h.refs))
	if h.hasSnapshotsOnLocalDate(requested) {
		return base
	}
	return fmt.Sprintf(
		"No snapshots for %s; %s",
		requested.Local().Format("2006-01-02"),
		base,
	)
}

func (h *HistoryApp) adjustStartIndexForSkipEmpty(target int) int {
	if !h.skipEmpty || len(h.refs) == 0 {
		return target
	}
	if target < 0 || target >= len(h.refs) {
		return target
	}
	if h.currentRefCount(h.refs[target]) > 0 {
		return target
	}

	if idx, skipped, ok := tuireplay.FindNextNonEmptyIndex(h.refs, target, h.currentRefCount); ok {
		if skipped > 0 {
			h.setStatusNote(
				fmt.Sprintf(
					"Jumped to oldest active snapshot (%s). Hidden empty before: %d.",
					tuireplay.RawPositionLabel(h.refs, idx),
					skipped,
				),
				6*time.Second,
			)
		}
		return idx
	}
	return target
}

func (h *HistoryApp) adjustLatestIndexForSkipEmpty(target int) int {
	if !h.skipEmpty || len(h.refs) == 0 {
		return target
	}
	if target < 0 || target >= len(h.refs) {
		return target
	}
	if h.currentRefCount(h.refs[target]) > 0 {
		return target
	}
	if idx, _, ok := tuireplay.FindPrevNonEmptyIndex(h.refs, target, h.currentRefCount); ok {
		return idx
	}
	return target
}

func (h *HistoryApp) adjustGenericIndexForSkipEmpty(target int) int {
	if !h.skipEmpty || len(h.refs) == 0 {
		return target
	}
	if target < 0 || target >= len(h.refs) {
		return target
	}
	if h.currentRefCount(h.refs[target]) > 0 {
		return target
	}
	if idx, ok := tuireplay.FindNearestNonEmptyIndex(h.refs, target, h.currentRefCount); ok {
		return idx
	}
	return target
}

func (h *HistoryApp) idleStreak() (start, end, count int, approx time.Duration) {
	if len(h.refs) == 0 || h.currentIndex < 0 || h.currentIndex >= len(h.refs) {
		return -1, -1, 0, 0
	}
	if h.currentRefCount(h.refs[h.currentIndex]) > 0 {
		return h.currentIndex, h.currentIndex, 0, 0
	}

	start, end = h.currentIndex, h.currentIndex
	for start > 0 && h.currentRefCount(h.refs[start-1]) == 0 {
		start--
	}
	for end+1 < len(h.refs) && h.currentRefCount(h.refs[end+1]) == 0 {
		end++
	}

	count = end - start + 1
	if count <= 1 {
		prevGap := time.Duration(0)
		nextGap := time.Duration(0)
		if h.currentIndex > 0 {
			prevGap = h.refs[h.currentIndex].CapturedAt.Sub(h.refs[h.currentIndex-1].CapturedAt)
		}
		if h.currentIndex+1 < len(h.refs) {
			nextGap = h.refs[h.currentIndex+1].CapturedAt.Sub(h.refs[h.currentIndex].CapturedAt)
		}
		if prevGap > 0 && (nextGap <= 0 || prevGap <= nextGap) {
			approx = prevGap
		} else if nextGap > 0 {
			approx = nextGap
		}
		return start, end, count, approx
	}

	approx = h.refs[end].CapturedAt.Sub(h.refs[start].CapturedAt)
	if approx < 0 {
		approx = 0
	}
	return start, end, count, approx
}

func formatApproxDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d >= time.Hour {
		return d.Round(time.Minute).String()
	}
	if d >= time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(100 * time.Millisecond).String()
}

// --- UIContext Implementation ---

func (h *HistoryApp) DataDir() string        { return h.dataDir }
func (h *HistoryApp) RangeBegin() *time.Time { return h.rangeBegin }
func (h *HistoryApp) RangeEnd() *time.Time   { return h.rangeEnd }
func (h *HistoryApp) RangeLabel() string     { return h.replayRangeLabel() }
func (h *HistoryApp) SensitiveIP() bool      { return h.sensitiveIP }

func (h *HistoryApp) AddPage(name string, item tview.Primitive, resize, visible bool) {
	h.pages.AddPage(name, item, resize, visible)
}
func (h *HistoryApp) RemovePage(name string)                      { h.pages.RemovePage(name) }
func (h *HistoryApp) SetFocus(p tview.Primitive)                  { h.app.SetFocus(p) }
func (h *HistoryApp) BackFocus()                                  { h.app.SetFocus(h.panel) }
func (h *HistoryApp) UpdateStatusBar()                            { h.updateStatusBar() }
func (h *HistoryApp) SetStatusNote(msg string, ttl time.Duration) { h.setStatusNote(msg, ttl) }
func (h *HistoryApp) TopDisplayLimit() int                        { return h.topDisplayLimit() }

func (h *HistoryApp) TraceHistoryStorageSummary() string {
	dir := strings.TrimSpace(h.dataDir)
	if dir == "" {
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

func (h *HistoryApp) TraceTimelineBySnapshot() map[int][]tuitrace.Entry {
	return h.traceTimelineBySnapshot
}
func (h *HistoryApp) TraceTimelineTotal() int                        { return h.traceTimelineTotal }
func (h *HistoryApp) TraceTimelineAssociated() int                   { return h.traceTimelineAssociated }
func (h *HistoryApp) TraceTimelineWindow() time.Duration             { return h.traceTimelineWindow }
func (h *HistoryApp) TraceOnlyMode() bool                            { return h.traceOnlyMode }
func (h *HistoryApp) TraceReplayEntries() []tuitrace.Entry           { return h.traceReplayEntries }
func (h *HistoryApp) SnapshotRefs() []history.SnapshotRef            { return h.refs }
func (h *HistoryApp) CurrentIndex() int                              { return h.currentIndex }
func (h *HistoryApp) TopDirection() tuishared.TopConnectionDirection { return h.topDirection }

func (h *HistoryApp) SetTraceTimeline(bySnapshot map[int][]tuitrace.Entry, total, associated int, window time.Duration) {
	h.traceTimelineBySnapshot = bySnapshot
	h.traceTimelineTotal = total
	h.traceTimelineAssociated = associated
	h.traceTimelineWindow = window
}

// --- Key Event Handling ---

func (h *HistoryApp) handleKeyEvent(event *tcell.EventKey) *tcell.EventKey {
	if h.isHelpVisible() {
		h.hideHelp()
		return nil
	}
	if h.isOverlayVisible() {
		return event
	}

	switch event.Key() {
	case tcell.KeyUp:
		if h.moveSelection(-1) {
			return nil
		}
		return event
	case tcell.KeyDown:
		if h.moveSelection(1) {
			return nil
		}
		return event
	case tcell.KeyLeft, tcell.KeyRight:
		// Disable horizontal panning in replay text panel.
		return nil
	case tcell.KeyEnter:
		h.setStatusNote("Read-only replay mode", 4*time.Second)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'q':
			select {
			case <-h.stopChan:
			default:
				close(h.stopChan)
			}
			h.app.Stop()
			return nil
		case '?':
			h.showHelp()
			return nil
		case '[':
			h.navigatePrev()
			return nil
		case ']':
			h.navigateNext()
			return nil
		case 'a', 'A':
			h.navigateOldest()
			return nil
		case 'e', 'E':
			h.navigateLatest()
			return nil
		case 'f':
			h.promptPortFilter()
			return nil
		case '/':
			h.promptTextFilter()
			return nil
		case 'o':
			if h.topDirection == tuishared.TopConnectionIncoming {
				h.topDirection = tuishared.TopConnectionOutgoing
			} else {
				h.topDirection = tuishared.TopConnectionIncoming
			}
			if h.skipEmpty {
				h.currentIndex = h.adjustGenericIndexForSkipEmpty(h.currentIndex)
			}
			h.selectedIndex = 0
			h.setStatusNote(fmt.Sprintf("Replay direction: %s", h.topDirection.Label()), 4*time.Second)
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'g', 'G':
			if h.replayViewMode == replayViewConnections {
				h.replayViewMode = replayViewTrace
				if h.traceTimelineAssociated == 0 {
					h.setStatusNote("Replay view: TRACE (no trace events in current scope/range)", 5*time.Second)
				} else {
					h.setStatusNote("Replay view: TRACE", 4*time.Second)
				}
			} else {
				if h.traceOnlyMode {
					h.setStatusNote("Connection view unavailable in trace-only replay", 5*time.Second)
					return nil
				}
				h.replayViewMode = replayViewConnections
				h.setStatusNote("Replay view: CONN", 4*time.Second)
			}
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'h', 'H':
			tuireplay.PromptReplayTraceHistory(h)
			return nil
		case 't', 'T':
			h.promptJumpToTime()
			return nil
		case 'S':
			if h.traceOnlyMode {
				h.setStatusNote("Timeline search is unavailable in trace-only replay", 5*time.Second)
				return nil
			}
			h.promptTimelineSearch()
			return nil
		case 'B', 'C', 'P':
			mode, _ := directSortModeForRune(event.Rune())
			h.applySortInput(mode)
			return nil
		case 'm':
			h.sensitiveIP = !h.sensitiveIP
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'i', 'I':
			h.promptSocketQueueExplain()
			return nil
		case 'x', 'X':
			h.skipEmpty = !h.skipEmpty
			if h.skipEmpty {
				h.setStatusNote("Skip-empty enabled", 4*time.Second)
				h.currentIndex = h.adjustGenericIndexForSkipEmpty(h.currentIndex)
			} else {
				h.setStatusNote("Skip-empty disabled", 4*time.Second)
			}
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'L':
			h.followLatest = !h.followLatest
			if h.followLatest {
				h.reloadIndex(false)
				h.navigateLatest()
				h.setStatusNote("Live tail ON — auto-jumping to newest snapshot", 4*time.Second)
			} else {
				h.setStatusNote("Live tail OFF", 4*time.Second)
			}
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'z':
			h.setStatusNote("Single-panel mode: zoom not needed", 4*time.Second)
			return nil
		case 'k', 'b':
			h.setStatusNote("Read-only replay mode", 4*time.Second)
			return nil
		}
	}

	return event
}

func (h *HistoryApp) navigatePrev() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	target := h.currentIndex - 1
	if h.skipEmpty {
		if idx, skipped, ok := tuireplay.FindPrevNonEmptyIndex(h.refs, target, h.currentRefCount); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(fmt.Sprintf("Skipped %d empty snapshots", skipped), 4*time.Second)
			}
		} else {
			h.setStatusNote(h.prevActiveBoundaryMessage(), 6*time.Second)
			h.renderPanel()
			h.updateStatusBar()
			return
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateNext() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	target := h.currentIndex + 1
	if h.skipEmpty {
		if idx, skipped, ok := tuireplay.FindNextNonEmptyIndex(h.refs, target, h.currentRefCount); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(fmt.Sprintf("Skipped %d empty snapshots", skipped), 4*time.Second)
			}
		} else {
			h.setStatusNote(h.nextActiveBoundaryMessage(), 6*time.Second)
			h.renderPanel()
			h.updateStatusBar()
			return
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateOldest() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	target := 0
	if h.skipEmpty {
		if idx, skipped, ok := tuireplay.FindNextNonEmptyIndex(h.refs, target, h.currentRefCount); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(
					fmt.Sprintf(
						"Jumped to oldest active snapshot (%s). Hidden empty before: %d.",
						tuireplay.RawPositionLabel(h.refs, target),
						skipped,
					),
					6*time.Second,
				)
			}
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateLatest() {
	if len(h.refs) == 0 {
		return
	}
	target := len(h.refs) - 1
	if h.skipEmpty {
		if idx, skipped, ok := tuireplay.FindPrevNonEmptyIndex(h.refs, target, h.currentRefCount); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(
					fmt.Sprintf(
						"Jumped to latest active snapshot (%s). Hidden empty after: %d.",
						tuireplay.RawPositionLabel(h.refs, target),
						skipped,
					),
					6*time.Second,
				)
			}
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) promptPortFilter() {
	if h.portFilter != "" || h.textFilter != "" {
		h.portFilter = ""
		h.textFilter = ""
		h.selectedIndex = 0
		h.renderPanel()
		h.updateStatusBar()
		return
	}

	input := tview.NewInputField()
	if h.topDirection == tuishared.TopConnectionOutgoing {
		input.SetLabel("Filter by remote port: ")
		input.SetTitle(" Remote Port Filter ")
	} else {
		input.SetLabel("Filter by local port: ")
		input.SetTitle(" Local Port Filter ")
	}
	input.SetFieldWidth(10)
	input.SetBorder(true)
	input.SetAcceptanceFunc(tview.InputFieldInteger)

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			parsed, err := tuireplay.ParsePortFilter(input.GetText())
			if err != nil {
				h.setStatusNote("Invalid port filter", 4*time.Second)
				return
			}
			h.portFilter = parsed
			h.selectedIndex = 0
			h.renderPanel()
		}
		h.pages.RemovePage("history-filter")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 30, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-filter", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) promptTextFilter() {
	input := tview.NewInputField()
	input.SetLabel("Search (contains): ")
	input.SetFieldWidth(36)
	input.SetText(h.textFilter)
	input.SetBorder(true)
	input.SetTitle(" Search Filter ")

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			entered := strings.TrimSpace(input.GetText())
			if entered == "" {
				h.portFilter = ""
				h.textFilter = ""
			} else {
				h.textFilter = entered
			}
			h.selectedIndex = 0
			h.renderPanel()
		}
		h.pages.RemovePage("history-search")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 54, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-search", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) promptJumpToTime() {
	if len(h.refs) == 0 {
		h.setStatusNote("No snapshots available", 4*time.Second)
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Jump to time: ")
	input.SetFieldWidth(36)
	if h.currentIndex >= 0 && h.currentIndex < len(h.refs) {
		input.SetText(h.refs[h.currentIndex].CapturedAt.Local().Format("2006-01-02 15:04:05"))
	}
	input.SetBorder(true)
	input.SetTitle(" Jump To Snapshot Time ")

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			target, err := parseHistoryJumpTime(input.GetText(), time.Now())
			if err != nil {
				h.setStatusNote("Invalid time. Use YYYY-MM-DD HH:MM[:SS], HH:MM[:SS], or yesterday HH:MM", 6*time.Second)
				return
			}

			index := tuireplay.ClosestSnapshotIndex(h.refs, target)
			if index >= 0 {
				if h.skipEmpty {
					if idx, ok := tuireplay.FindNearestNonEmptyIndex(h.refs, index, h.currentRefCount); ok {
						index = idx
					}
				}
				h.followLatest = false
				h.loadSnapshotAt(index)
				summary := h.buildJumpSummary(target, index)
				h.setSnapshotMessage(summary)
				h.setStatusNote(summary, 10*time.Second)
			}
			h.renderPanel()
		}
		h.pages.RemovePage("history-jump-time")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 60, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-jump-time", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) showHelp() {
	h.pages.SendToFront("history-help")
	h.pages.ShowPage("history-help")
	h.updateStatusBar()
}

func (h *HistoryApp) hideHelp() {
	h.pages.HidePage("history-help")
	h.updateStatusBar()
}

func (h *HistoryApp) isHelpVisible() bool {
	name, _ := h.pages.GetFrontPage()
	return name == "history-help"
}

func (h *HistoryApp) isOverlayVisible() bool {
	name, _ := h.pages.GetFrontPage()
	return name != "main" && name != "history-help"
}

func (h *HistoryApp) applySortInput(mode tuishared.SortMode) {
	if h.sortMode == mode {
		h.sortDesc = !h.sortDesc
	} else {
		h.sortMode = mode
		h.sortDesc = true // first hit on mode starts DESC
	}
	h.selectedIndex = 0
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) snapshotSummary() string {
	if len(h.refs) == 0 || h.currentIndex < 0 {
		return "Snapshot: 0/0"
	}
	rec := h.currentRecord
	when := rec.CapturedAt.Local().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("Snapshot: %d/%d (%s)", h.currentIndex+1, len(h.refs), when)
}

func (h *HistoryApp) nextActiveBoundaryMessage() string {
	if len(h.refs) == 0 || h.currentIndex < 0 || h.currentIndex >= len(h.refs) {
		return "Reached last active snapshot"
	}
	return fmt.Sprintf(
		"Reached last active snapshot (%s). Hidden empty after: %d. Press x to include empty snapshots.",
		tuireplay.RawPositionLabel(h.refs, h.currentIndex),
		tuireplay.EmptyCountAfter(h.refs, h.currentIndex, h.currentRefCount),
	)
}

func (h *HistoryApp) prevActiveBoundaryMessage() string {
	if len(h.refs) == 0 || h.currentIndex < 0 || h.currentIndex >= len(h.refs) {
		return "Reached first active snapshot"
	}
	return fmt.Sprintf(
		"Reached first active snapshot (%s). Hidden empty before: %d. Press x to include empty snapshots.",
		tuireplay.RawPositionLabel(h.refs, h.currentIndex),
		tuireplay.EmptyCountBefore(h.refs, h.currentIndex, h.currentRefCount),
	)
}

func parseHistoryJumpTime(raw string, now time.Time) (time.Time, error) {
	return history.ParseReplayTime(raw, now)
}

func (h *HistoryApp) promptSocketQueueExplain() {
	closeModal := func() {
		h.pages.RemovePage("history-socket-queue-explain")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	}

	modal := tview.NewModal().
		SetText(tuioverlays.BuildSocketQueueExplainText(true)).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) {
			closeModal()
		})
	modal.SetTitle(" Send-Q / Recv-Q Explain ")
	modal.SetBorder(true)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	h.pages.RemovePage("history-socket-queue-explain")
	h.pages.AddPage("history-socket-queue-explain", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(modal)
}

// --- Timeline Search ---

func (h *HistoryApp) promptTimelineSearch() {
	if len(h.refs) == 0 {
		h.setStatusNote("No snapshots available", 4*time.Second)
		return
	}
	if h.timelineSearchRunning {
		h.setStatusNote("Timeline search is already running", 4*time.Second)
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Search timeline (contains): ")
	input.SetFieldWidth(44)
	input.SetText(h.timelineSearchQuery)
	input.SetBorder(true)
	input.SetTitle(" Timeline Search ")

	closeModal := func() {
		h.pages.RemovePage("history-timeline-search")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	}

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			query := strings.TrimSpace(input.GetText())
			closeModal()
			if query == "" {
				return
			}
			h.startTimelineSearch(query)
			return
		}
		closeModal()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 68, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-timeline-search", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) startTimelineSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	if h.timelineSearchRunning {
		h.setStatusNote("Timeline search is already running", 4*time.Second)
		return
	}
	if len(h.refs) == 0 {
		h.setStatusNote("No snapshots available", 4*time.Second)
		return
	}

	h.followLatest = false
	h.timelineSearchRunning = true
	h.timelineSearchQuery = query
	h.timelineSearchResults = nil
	h.timelineSearchSelected = 0
	h.setStatusNote(fmt.Sprintf("Searching timeline for '%s'...", shortStatus(query, 48)), 5*time.Second)

	refs := append([]history.SnapshotRef(nil), h.refs...)
	go func(searchQuery string, searchRefs []history.SnapshotRef) {
		results := h.scanTimelineMatches(searchQuery, searchRefs)

		h.app.QueueUpdateDraw(func() {
			h.timelineSearchRunning = false
			h.timelineSearchQuery = searchQuery
			h.timelineSearchResults = results
			h.timelineSearchSelected = 0

			if len(results) == 0 {
				h.setStatusNote(fmt.Sprintf("No snapshots matched '%s'", shortStatus(searchQuery, 48)), 6*time.Second)
				return
			}

			h.showTimelineSearchResults()
			h.setStatusNote(
				fmt.Sprintf("Found %d matching snapshots for '%s'", len(results), shortStatus(searchQuery, 48)),
				6*time.Second,
			)
		})
	}(query, refs)
}

func (h *HistoryApp) scanTimelineMatches(query string, refs []history.SnapshotRef) []tuireplay.SearchResult {
	query = strings.TrimSpace(query)
	if query == "" || len(refs) == 0 {
		return nil
	}

	return tuireplay.ScanTimelineMatches(refs, func(ref history.SnapshotRef) (int, error) {
		record, err := history.ReadSnapshot(ref)
		if err != nil {
			return 0, err
		}
		return len(tuipanels.FilterHistoryGroupsByText(h.rowsForDirection(record, h.topDirection), query)), nil
	})
}

func (h *HistoryApp) showTimelineSearchResults() {
	if len(h.timelineSearchResults) == 0 {
		return
	}

	closeModal := func() {
		h.pages.RemovePage("history-timeline-results")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	}

	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true)
	list.SetTitle(" Timeline Search Results ")
	list.SetTitleAlign(tview.AlignCenter)

	totalSnapshots := len(h.refs)
	for _, result := range h.timelineSearchResults {
		label := fmt.Sprintf(
			"%s | snapshot %d/%d | matches=%d",
			result.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			result.SnapshotIndex+1,
			totalSnapshots,
			result.MatchCount,
		)
		list.AddItem(label, "", 0, nil)
	}

	if h.timelineSearchSelected < 0 {
		h.timelineSearchSelected = 0
	}
	if h.timelineSearchSelected >= len(h.timelineSearchResults) {
		h.timelineSearchSelected = len(h.timelineSearchResults) - 1
	}
	list.SetCurrentItem(h.timelineSearchSelected)

	list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		h.jumpToTimelineSearchResult(index)
		closeModal()
	})
	list.SetDoneFunc(func() {
		closeModal()
	})
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	resultHeight := len(h.timelineSearchResults) + 4
	if resultHeight < 8 {
		resultHeight = 8
	}
	if resultHeight > 24 {
		resultHeight = 24
	}

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(list, 104, 0, true).
			AddItem(nil, 0, 1, false),
			resultHeight, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.RemovePage("history-timeline-results")
	h.pages.AddPage("history-timeline-results", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(list)
}

func (h *HistoryApp) jumpToTimelineSearchResult(resultIndex int) {
	if resultIndex < 0 || resultIndex >= len(h.timelineSearchResults) {
		return
	}
	result := h.timelineSearchResults[resultIndex]
	if result.SnapshotIndex < 0 || result.SnapshotIndex >= len(h.refs) {
		return
	}

	h.followLatest = false
	h.timelineSearchSelected = resultIndex
	h.loadSnapshotAt(result.SnapshotIndex)

	msg := fmt.Sprintf(
		"Timeline match '%s': snapshot %d/%d (%d rows)",
		shortStatus(h.timelineSearchQuery, 48),
		result.SnapshotIndex+1,
		len(h.refs),
		result.MatchCount,
	)
	h.setSnapshotMessage(msg)
	h.setStatusNote(msg, 8*time.Second)
	h.renderPanel()
	h.updateStatusBar()
}
