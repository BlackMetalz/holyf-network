package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	HistoryStartLatest = "latest"
	HistoryStartOldest = "oldest"
)

// HistoryApp is read-only replay UI for persisted connection snapshots.
type HistoryApp struct {
	app       *tview.Application
	pages     *tview.Pages
	panel     *tview.TextView
	statusBar *tview.TextView
	layout    *tview.Flex

	dataDir     string
	startAt     string
	segmentFile string
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
	groupView     bool
	selectedIndex int
	followLatest  bool

	statusNote      string
	statusNoteUntil time.Time

	stopChan chan struct{}
}

func NewHistoryApp(dataDir, startAt, segmentFile string, sensitiveIP bool, appVersion string) *HistoryApp {
	startAt = strings.TrimSpace(strings.ToLower(startAt))
	if startAt != HistoryStartOldest {
		startAt = HistoryStartLatest
	}
	version := strings.TrimSpace(appVersion)
	if version == "" {
		version = "dev"
	}

	return &HistoryApp{
		app:          tview.NewApplication(),
		dataDir:      history.ExpandPath(dataDir),
		startAt:      startAt,
		segmentFile:  strings.TrimSpace(segmentFile),
		sensitiveIP:  sensitiveIP,
		appVersion:   version,
		sortMode:     SortByQueue,
		currentIndex: -1,
		stopChan:     make(chan struct{}),
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
		target = h.startIndex()
	} else if h.followLatest {
		target = len(h.refs) - 1
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
		target = len(h.refs) - 1
	}

	h.loadSnapshotAt(target)
}

func (h *HistoryApp) startIndex() int {
	if len(h.refs) == 0 {
		return -1
	}
	if h.startAt == HistoryStartOldest {
		return 0
	}
	return len(h.refs) - 1
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

func (h *HistoryApp) visibleConnections() []collector.Connection {
	if len(h.currentRecord.Connections) == 0 {
		return nil
	}
	filtered := h.currentRecord.Connections
	if h.portFilter != "" {
		filtered = filterByPort(filtered, h.portFilter)
	}
	if h.textFilter != "" {
		filtered = filterByText(filtered, h.textFilter)
	}
	if len(filtered) == 0 {
		return nil
	}

	items := append([]collector.Connection(nil), filtered...)
	sortConnections(items, h.sortMode)

	limit := h.topDisplayLimit()
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (h *HistoryApp) visibleGroups() []PeerGroup {
	if len(h.currentRecord.Connections) == 0 {
		return nil
	}
	filtered := applyGroupConnectionFilters(h.currentRecord.Connections, h.portFilter, h.textFilter)
	if len(filtered) == 0 {
		return nil
	}
	groups := buildPeerGroups(filtered)
	limit := h.topDisplayLimit()
	if len(groups) > limit {
		groups = groups[:limit]
	}
	return groups
}

func (h *HistoryApp) visibleCount() int {
	if h.groupView {
		return len(h.visibleGroups())
	}
	return len(h.visibleConnections())
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
	captured := rec.CapturedAt.Local().Format("2006-01-02 15:04:05 -07")
	iface := rec.Interface
	if strings.TrimSpace(iface) == "" {
		iface = "unknown"
	}

	header := fmt.Sprintf(
		"  [dim]Snapshot %d/%d | %s | %s | records=%d | scope=%s[white]\n",
		h.currentIndex+1,
		len(h.refs),
		captured,
		iface,
		len(rec.Connections),
		h.replayScopeLabel(),
	)

	body := ""
	if h.groupView {
		body = renderPeerGroupPanelReadOnly(
			rec.Connections,
			h.portFilter,
			h.textFilter,
			h.topDisplayLimit(),
			h.sensitiveIP,
			h.selectedIndex,
		)
	} else {
		body = renderTalkersPanelReadOnly(
			rec.Connections,
			h.portFilter,
			h.textFilter,
			h.topDisplayLimit(),
			h.sensitiveIP,
			h.selectedIndex,
			h.sortMode,
		)
	}

	h.panel.SetText(header + "\n" + body)
}

func (h *HistoryApp) replayScopeLabel() string {
	file := strings.TrimSpace(h.segmentFile)
	if file == "" {
		return "ALL"
	}
	return filepath.Base(file)
}

func (h *HistoryApp) updateStatusBar() {
	if h.statusBar == nil {
		return
	}
	page := h.frontPageName()
	hotkeysStyled, hotkeysPlain := historyStatusHotkeysForPage(page)

	snapshotPart := "Snapshot: 0/0"
	if len(h.refs) > 0 && h.currentIndex >= 0 {
		snapshotPart = fmt.Sprintf("Snapshot: %d/%d", h.currentIndex+1, len(h.refs))
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
	if time.Now().Before(h.statusNoteUntil) && h.statusNote != "" {
		stateText += fmt.Sprintf(" [yellow]%s[white] |", h.statusNote)
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
	h.statusNoteUntil = time.Now().Add(ttl)
	h.updateStatusBar()
}
