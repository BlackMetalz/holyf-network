package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/history"
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
	sortMode      SortMode
	sortDesc      bool
	topDirection  topConnectionDirection
	skipEmpty     bool
	selectedIndex int
	followLatest  bool

	timelineSearchQuery    string
	timelineSearchResults  []timelineSearchResult
	timelineSearchSelected int
	timelineSearchRunning  bool
	replayViewMode         replayViewMode

	traceOnlyMode           bool
	traceReplayEntries      []traceHistoryEntry
	traceTimelineBySnapshot map[int][]traceHistoryEntry
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
		sortMode:         SortByBandwidth,
		sortDesc:         true,
		topDirection:     topConnectionIncoming,
		skipEmpty:        true,
		currentIndex:     -1,
		replayViewMode:   replayViewConnections,
		stopChan:         make(chan struct{}),
		healthThresholds: config.DefaultHealthThresholds(),
	}
}

func (h *HistoryApp) Run() error {
	h.panel = createHistoryPanel()
	h.statusBar = createHistoryStatusBar()
	h.layout = createHistoryLayout(h.panel, h.statusBar)
	h.pages = tview.NewPages()
	h.pages.AddPage("main", h.layout, true, true)
	h.pages.AddPage("history-help", createHistoryHelpModal(), true, false)

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
	refs = h.filterRefsByReplayRange(refs)
	h.traceOnlyMode = false
	h.traceReplayEntries = nil
	if len(refs) == 0 {
		if traceRefs, traceEntries := h.buildTraceOnlyRefs(); len(traceRefs) > 0 {
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
	h.rebuildTraceTimeline()

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

func (h *HistoryApp) rowsForDirection(record history.SnapshotRecord, direction topConnectionDirection) []history.SnapshotGroup {
	history.NormalizeSnapshotRecord(&record)
	if len(record.IncomingGroups) == 0 && len(record.OutgoingGroups) == 0 && len(record.Groups) > 0 {
		return record.Groups
	}
	if direction == topConnectionOutgoing {
		return record.OutgoingGroups
	}
	return record.IncomingGroups
}

func (h *HistoryApp) currentRows() []history.SnapshotGroup {
	return h.rowsForDirection(h.currentRecord, h.topDirection)
}

func (h *HistoryApp) refCountForDirection(ref history.SnapshotRef, direction topConnectionDirection) int {
	if h.traceOnlyMode {
		return 1
	}
	if ref.IncomingCount == 0 && ref.OutgoingCount == 0 && ref.ConnCount > 0 {
		return ref.ConnCount
	}
	if direction == topConnectionOutgoing {
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
		filtered = filterHistoryGroupsByPort(filtered, h.portFilter)
	}
	if strings.TrimSpace(h.textFilter) != "" {
		filtered = filterHistoryGroupsByText(filtered, h.textFilter)
	}
	if len(filtered) == 0 {
		return nil
	}

	items := append([]history.SnapshotGroup(nil), filtered...)
	sortHistoryGroups(items, h.sortMode, h.sortDesc)

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

	header := fmt.Sprintf(
		"  [dim]%s | %s | %s | rows=in:%d out:%d | dir=%s | view=%s | scope=%s | range=%s[white]\n",
		h.headerSnapshotLabel(),
		captured,
		iface,
		len(rec.IncomingGroups),
		len(rec.OutgoingGroups),
		h.topDirection.Label(),
		h.replayViewMode.Label(),
		h.replayScopeLabel(),
		h.replayRangeLabel(),
	)
	if rec.BandwidthAvailable && rec.SampleSeconds > 0 {
		header += fmt.Sprintf("  [dim]BW sample: %.1fs (conntrack delta)[white]\n", rec.SampleSeconds)
	} else {
		header += "  [dim]BW sample: unavailable[white]\n"
	}
	if strings.TrimSpace(h.snapshotMessage) != "" {
		header += fmt.Sprintf("  [yellow]%s[white]\n", shortStatus(h.snapshotMessage, 160))
	}
	traceSection := h.renderCurrentTraceTimelineSection()
	traceOnlyHint := ""
	if h.traceOnlyMode {
		traceOnlyHint = "  [yellow]Trace-only replay mode[white] (no connection snapshots in selected scope/range)\n\n"
	}
	if h.replayViewMode == replayViewTrace {
		h.panel.SetText(header + "\n" + traceOnlyHint + traceSection + h.renderTraceReplayViewBody())
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

	body := renderHistoryAggregatePanel(
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
	hotkeysStyled, hotkeysPlain := historyStatusHotkeysForPage(page)

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

	followState := "FOLLOW-OFF"
	followColor := "dim"
	if h.followLatest {
		followState = "FOLLOW-ON"
		followColor = "green"
	}

	stateText := ""
	if h.sensitiveIP {
		stateText += " [yellow]IP MASK[white] |"
	}
	if h.traceOnlyMode {
		stateText += " [aqua]TRACE-ONLY[white] |"
	} else {
		stateText += fmt.Sprintf(" [aqua]DIR-%s[white] |", h.topDirection.Label())
		if h.skipEmpty {
			stateText += " [aqua]SKIP-EMPTY[white] |"
		}
	}
	stateText += fmt.Sprintf(" [aqua]VIEW-%s[white] |", h.replayViewMode.Label())
	if h.traceTimelineAssociated > 0 {
		stateText += fmt.Sprintf(" [aqua]TRACE %d/%d[white] |", h.currentTraceTimelineCount(), h.traceTimelineAssociated)
	}
	if time.Now().Before(h.statusNoteUntil) && h.statusNote != "" {
		stateText += fmt.Sprintf(" [yellow]%s[white] |", h.statusNote)
	} else if strings.TrimSpace(h.lastStatusNote) != "" {
		stateText += fmt.Sprintf(" [dim]Last:%s[white] |", shortStatus(h.lastStatusNote, 72))
	}

	leftStyled := fmt.Sprintf(
		" [yellow]history[white] |%s %s | Files:%d | Corrupt:%d | [%s]%s[white] | %s",
		stateText,
		snapshotPart,
		h.filesCount,
		h.corruptSkipped,
		followColor,
		followState,
		hotkeysStyled,
	)
	leftPlain := fmt.Sprintf(
		" history |%s %s | Files:%d | Corrupt:%d | %s | %s",
		stripStatusColors(stateText),
		snapshotPart,
		h.filesCount,
		h.corruptSkipped,
		followState,
		hotkeysPlain,
	)

	rightStyled := fmt.Sprintf(" [dim]holyf-network %s[white]", h.appVersion)
	rightPlain := fmt.Sprintf(" holyf-network %s", h.appVersion)

	text := leftStyled
	_, _, width, _ := h.statusBar.GetInnerRect()
	if width > 0 {
		pad := width - utf8.RuneCountInString(leftPlain) - utf8.RuneCountInString(rightPlain)
		if pad > 0 {
			text = leftStyled + strings.Repeat(" ", pad) + rightStyled
		}
	}
	h.statusBar.SetText(text)
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

	if idx, skipped, ok := h.findNextNonEmptyIndex(target); ok {
		if skipped > 0 {
			h.setStatusNote(
				fmt.Sprintf(
					"Jumped to oldest active snapshot (%s). Hidden empty before: %d.",
					h.rawPositionLabel(idx),
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
	if idx, _, ok := h.findPrevNonEmptyIndex(target); ok {
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
	if idx, ok := h.findNearestNonEmptyIndex(target); ok {
		return idx
	}
	return target
}

func (h *HistoryApp) findPrevNonEmptyIndex(from int) (int, int, bool) {
	if len(h.refs) == 0 {
		return -1, 0, false
	}
	if from >= len(h.refs) {
		from = len(h.refs) - 1
	}
	skipped := 0
	for i := from; i >= 0; i-- {
		if h.currentRefCount(h.refs[i]) > 0 {
			return i, skipped, true
		}
		skipped++
	}
	return -1, skipped, false
}

func (h *HistoryApp) findNextNonEmptyIndex(from int) (int, int, bool) {
	if len(h.refs) == 0 {
		return -1, 0, false
	}
	if from < 0 {
		from = 0
	}
	skipped := 0
	for i := from; i < len(h.refs); i++ {
		if h.currentRefCount(h.refs[i]) > 0 {
			return i, skipped, true
		}
		skipped++
	}
	return -1, skipped, false
}

func (h *HistoryApp) findNearestNonEmptyIndex(target int) (int, bool) {
	if len(h.refs) == 0 {
		return -1, false
	}
	if target < 0 || target >= len(h.refs) {
		return -1, false
	}
	if h.currentRefCount(h.refs[target]) > 0 {
		return target, true
	}

	left, right := target-1, target+1
	for left >= 0 || right < len(h.refs) {
		if left >= 0 && h.currentRefCount(h.refs[left]) > 0 {
			return left, true
		}
		if right < len(h.refs) && h.currentRefCount(h.refs[right]) > 0 {
			return right, true
		}
		left--
		right++
	}
	return -1, false
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
