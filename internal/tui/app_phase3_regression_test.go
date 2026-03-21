package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newPhase3TestApp() *App {
	panels := []*tview.TextView{
		tview.NewTextView(),
		tview.NewTextView(),
		tview.NewTextView(),
		tview.NewTextView(),
		tview.NewTextView(),
	}
	pages := tview.NewPages()
	pages.AddPage("main", tview.NewBox(), true, true)
	pages.AddPage("help", tview.NewBox(), true, false)

	return &App{
		app:          tview.NewApplication(),
		pages:        pages,
		panels:       panels,
		statusBar:    tview.NewTextView(),
		focusIndex:   2,
		ifaceName:    "eth0",
		refreshSec:   5,
		appVersion:   "test",
		stopChan:     make(chan struct{}),
		refreshChan:  make(chan struct{}, 1),
		activeBlocks: map[string]activeBlockEntry{},
		actionLogs:   make([]string, 0, 32),
		traceHistory: make([]traceHistoryEntry, 0, 8),
		// Keep tests hermetic: do not read user-level trace history file.
		traceHistoryLoaded: true,
	}
}

func TestHandleKeyEventHelpVisibleConsumesAndCloses(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.pages.ShowPage("help")
	a.pages.SendToFront("help")
	if !a.isHelpVisible() {
		t.Fatalf("help page should be visible before key press")
	}

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'x', 0))
	if ret != nil {
		t.Fatalf("help-visible key should be consumed")
	}
	if a.isHelpVisible() {
		t.Fatalf("help page should close after any key")
	}
}

func TestHandleKeyEventOverlayVisiblePassesThrough(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.pages.AddPage("search", tview.NewBox(), true, true) // Front page is overlay.

	ev := tcell.NewEventKey(tcell.KeyRune, 'r', 0)
	ret := a.handleKeyEvent(ev)
	if ret != ev {
		t.Fatalf("overlay-visible key should pass through to focused widget")
	}
	if len(a.refreshChan) != 0 {
		t.Fatalf("global refresh hotkey should not trigger while overlay is visible")
	}
}

func TestHandleKeyEventEnterOnTopConnectionsOpensKillForm(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.latestTalkers = []collector.Connection{
		{
			LocalIP:    "10.0.0.10",
			LocalPort:  22,
			RemoteIP:   "198.51.100.10",
			RemotePort: 50001,
			State:      "ESTABLISHED",
			Activity:   42,
			PID:        1,
			ProcName:   "sshd",
		},
	}

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if ret != nil {
		t.Fatalf("Enter on Top Connections should be handled")
	}
	name, _ := a.pages.GetFrontPage()
	if name != "kill-peer-form" {
		t.Fatalf("expected kill-peer-form modal, got %q", name)
	}
	if !a.paused.Load() {
		t.Fatalf("kill flow should auto-pause refresh while form is open")
	}
}

func TestHandleKeyEventEnterAndKAreDisabledInOutgoingMode(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.topDirection = topConnectionOutgoing

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if ret != nil {
		t.Fatalf("Enter in OUT mode should be handled")
	}
	if !strings.Contains(a.statusNote, "disabled in OUT mode") {
		t.Fatalf("expected OUT mode status note after Enter, got=%q", a.statusNote)
	}
	name, _ := a.pages.GetFrontPage()
	if name != "main" {
		t.Fatalf("expected no kill modal in OUT mode, got front page %q", name)
	}

	a.statusNote = ""
	ret = a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'k', 0))
	if ret != nil {
		t.Fatalf("k in OUT mode should be handled")
	}
	if !strings.Contains(a.statusNote, "disabled in OUT mode") {
		t.Fatalf("expected OUT mode status note after k, got=%q", a.statusNote)
	}
}

func TestHandleKeyEventTRequiresTopConnectionsFocus(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 0

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'T', 0))
	if ret != nil {
		t.Fatalf("T should be handled")
	}
	if !strings.Contains(a.statusNote, "Focus Top Connections before trace-packet") {
		t.Fatalf("unexpected status note: %q", a.statusNote)
	}
	name, _ := a.pages.GetFrontPage()
	if name != "main" {
		t.Fatalf("should stay on main page, got=%q", name)
	}
}

func TestHandleKeyEventTRequiresSelectedRow(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 2

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'T', 0))
	if ret != nil {
		t.Fatalf("T should be handled")
	}
	if !strings.Contains(a.statusNote, "No row selected for trace-packet") {
		t.Fatalf("unexpected status note: %q", a.statusNote)
	}
	name, _ := a.pages.GetFrontPage()
	if name != "main" {
		t.Fatalf("should stay on main page when no row is selected, got=%q", name)
	}
}

func TestRecentActionLogsDefaultLimitIsModalLimit(t *testing.T) {
	t.Parallel()

	a := &App{}
	for i := 1; i <= 30; i++ {
		a.actionLogs = append(a.actionLogs, fmt.Sprintf("line-%02d", i))
	}

	out := a.recentActionLogs(0) // default path
	if len(out) != actionLogModalLimit {
		t.Fatalf("default recentActionLogs limit mismatch: got=%d want=%d", len(out), actionLogModalLimit)
	}
	if out[0] != "line-30" {
		t.Fatalf("most recent entry mismatch: got=%q want=%q", out[0], "line-30")
	}
	if out[len(out)-1] != "line-11" {
		t.Fatalf("oldest default-limited entry mismatch: got=%q want=%q", out[len(out)-1], "line-11")
	}
}

func TestStatusHotkeysForModalPages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		page      string
		wantPlain string
	}{
		{page: "kill-peer-form", wantPlain: "Tab=field Enter=next Esc=cancel"},
		{page: "kill-peer", wantPlain: "<-/->=choose Enter=confirm Esc=cancel"},
		{page: tracePacketPageForm, wantPlain: "Tab=field Enter=next Esc=cancel"},
		{page: tracePacketPageProgress, wantPlain: "Esc=abort q=abort"},
		{page: tracePacketPageResult, wantPlain: "Enter=close Esc=close"},
		{page: "blocked-peers", wantPlain: "Up/Down=select Enter=remove Del=remove Tab=buttons Esc=close"},
		{page: "action-log", wantPlain: "Enter=close Esc=close"},
		{page: "diagnosis-history", wantPlain: "Enter=close Esc=close"},
		{page: traceHistoryPage, wantPlain: "Up/Down=select Enter=detail Esc=close"},
		{page: traceHistoryDetailPage, wantPlain: "Enter=close Esc=close"},
		{page: "socket-queue-explain", wantPlain: "Enter=close Esc=close"},
		{page: "interface-stats-explain", wantPlain: "Enter=close Esc=close"},
		{page: "blocked-peers-remove-result", wantPlain: "Enter=close Esc=close"},
		{page: "block-summary", wantPlain: "Enter=close Esc=close"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.page, func(t *testing.T) {
			t.Parallel()
			a := newPhase3TestApp()
			_, plain := a.statusHotkeysForPage(tc.page)
			if plain != tc.wantPlain {
				t.Fatalf("plain hotkeys mismatch for page=%q: got=%q want=%q", tc.page, plain, tc.wantPlain)
			}
		})
	}
}

func TestLiveStatusBarKeepsLastMessageAfterTTL(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.setStatusNote("live test message", 100*time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	a.updateStatusBar()

	text := a.statusBar.GetText(true)
	if !strings.Contains(text, "Last:live test message") {
		t.Fatalf("status bar should keep last message after ttl, got=%q", text)
	}
}

func TestLiveStatusBarShowsUpdateSuffixWhenLatestTagAvailable(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.statusBar.SetRect(0, 0, 400, 1)
	a.updateLatestTag = "v0.3.33"
	a.updateStatusBar()

	text := a.statusBar.GetText(true)
	if !strings.Contains(text, "holyf-network test") || !strings.Contains(text, "(new v0.3.33)") {
		t.Fatalf("status bar should include update suffix, got=%q", text)
	}
}

func TestLiveStatusBarKeepsBaseVersionWhenNoUpdateTag(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.statusBar.SetRect(0, 0, 400, 1)
	a.updateLatestTag = ""
	a.updateStatusBar()

	text := a.statusBar.GetText(true)
	if strings.Contains(text, "(new ") {
		t.Fatalf("status bar should not include update suffix, got=%q", text)
	}
	if !strings.Contains(text, "holyf-network test") {
		t.Fatalf("status bar should keep base version label, got=%q", text)
	}
}

func TestSelectedPeerKillTargetGroupViewUsesSelectedPeerContext(t *testing.T) {
	t.Parallel()

	a := &App{
		latestTalkers:       topConnectionFixtures(),
		groupView:           true,
		sortDesc:            true,
		selectedTalkerIndex: 0,
	}

	target, ok := a.selectedPeerKillTarget()
	if !ok {
		t.Fatalf("expected selected peer target in group view")
	}
	if target.PeerIP != "198.51.100.10" {
		t.Fatalf("selected peer mismatch: got=%q want=%q", target.PeerIP, "198.51.100.10")
	}
	if target.LocalPort != 80 {
		t.Fatalf("selected local port mismatch: got=%d want=%d", target.LocalPort, 80)
	}
}

func TestHandleKeyEventIShowsSocketQueueExplain(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'i', 0))
	if ret != nil {
		t.Fatalf("i should be handled")
	}
	name, _ := a.pages.GetFrontPage()
	if name != "socket-queue-explain" {
		t.Fatalf("expected socket-queue-explain page, got %q", name)
	}
}

func TestHandleKeyEventShiftIShowsInterfaceStatsExplain(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'I', 0))
	if ret != nil {
		t.Fatalf("I should be handled")
	}
	name, _ := a.pages.GetFrontPage()
	if name != "interface-stats-explain" {
		t.Fatalf("expected interface-stats-explain page, got %q", name)
	}
}

func TestHandleKeyEventHorizontalArrowsAreDisabled(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	startFocus := a.focusIndex

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if ret != nil {
		t.Fatalf("Right arrow should be consumed")
	}
	if a.focusIndex != startFocus {
		t.Fatalf("focus index should not change on Right arrow: got=%d want=%d", a.focusIndex, startFocus)
	}

	ret = a.handleKeyEvent(tcell.NewEventKey(tcell.KeyLeft, 0, 0))
	if ret != nil {
		t.Fatalf("Left arrow should be consumed")
	}
	if a.focusIndex != startFocus {
		t.Fatalf("focus index should not change on Left arrow: got=%d want=%d", a.focusIndex, startFocus)
	}
}

func TestHandleKeyEventMTogglesSensitiveMask(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	if a.sensitiveIP {
		t.Fatalf("precondition: sensitiveIP should start false")
	}

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'm', 0))
	if ret != nil {
		t.Fatalf("m should be handled")
	}
	if !a.sensitiveIP {
		t.Fatalf("m should toggle sensitiveIP on")
	}
}

func TestHandleKeyEventSOnConnectionStatesTogglesSortDirection(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 0
	a.connStateSortDesc = true

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 's', 0))
	if ret != nil {
		t.Fatalf("s on Connection States should be handled")
	}
	if a.connStateSortDesc {
		t.Fatalf("s should toggle connStateSortDesc from DESC to ASC")
	}
}

func TestHandleKeyEventSOutsideConnectionStatesShowsHintOnly(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 2
	a.connStateSortDesc = true

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 's', 0))
	if ret != nil {
		t.Fatalf("s should be handled with hint when not focused on Connection States")
	}
	if !a.connStateSortDesc {
		t.Fatalf("s outside Connection States should not change state sort direction")
	}
	if !strings.Contains(a.statusNote, "focus Connection States") {
		t.Fatalf("expected focus hint status note, got=%q", a.statusNote)
	}
}

func TestFocusOrderFollowsRequestedPanelSequence(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 2 // Top

	a.focusNext()
	if a.focusIndex != 0 { // States
		t.Fatalf("next focus mismatch: got=%d want=%d", a.focusIndex, 0)
	}
	a.focusNext()
	if a.focusIndex != 1 { // Interface
		t.Fatalf("next focus mismatch: got=%d want=%d", a.focusIndex, 1)
	}
	a.focusNext()
	if a.focusIndex != 3 { // Conntrack
		t.Fatalf("next focus mismatch: got=%d want=%d", a.focusIndex, 3)
	}
	a.focusNext()
	if a.focusIndex != 4 { // Diagnosis
		t.Fatalf("next focus mismatch: got=%d want=%d", a.focusIndex, 4)
	}
	a.focusNext()
	if a.focusIndex != 2 { // wrap Top
		t.Fatalf("next wrap mismatch: got=%d want=%d", a.focusIndex, 2)
	}
}

func TestHandleKeyEventCtrlNumberFocusShortcuts(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	tests := []struct {
		rune      rune
		wantFocus int
	}{
		{rune: '1', wantFocus: 2}, // Top
		{rune: '2', wantFocus: 0}, // States
		{rune: '3', wantFocus: 1}, // Interface
		{rune: '4', wantFocus: 3}, // Conntrack
		{rune: '5', wantFocus: 4}, // Diagnosis
	}

	for _, tc := range tests {
		ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, tc.rune, tcell.ModCtrl))
		if ret != nil {
			t.Fatalf("ctrl+%c should be handled", tc.rune)
		}
		if a.focusIndex != tc.wantFocus {
			t.Fatalf("ctrl+%c focus mismatch: got=%d want=%d", tc.rune, a.focusIndex, tc.wantFocus)
		}
	}
}

func TestHandleKeyEventZoomOnlyForTopConnections(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 0 // Connection States

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'z', 0))
	if ret != nil {
		t.Fatalf("z should be handled")
	}
	if a.zoomed {
		t.Fatalf("non-top panel should not enter zoom mode")
	}
	if !strings.Contains(a.statusNote, "Top Connections") {
		t.Fatalf("expected zoom restriction note, got=%q", a.statusNote)
	}

	a.focusIndex = 2 // Top Connections
	ret = a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'z', 0))
	if ret != nil {
		t.Fatalf("z should be handled for top panel")
	}
	if !a.zoomed {
		t.Fatalf("top panel should enter zoom mode")
	}
}

func TestHandleKeyEventArrowKeysAreBlockedOutsideTopConnections(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 4 // Diagnosis
	a.selectedTalkerIndex = 1

	up := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyUp, 0, 0))
	down := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if up != nil || down != nil {
		t.Fatalf("arrow keys should be consumed outside Top Connections")
	}
	if a.selectedTalkerIndex != 1 {
		t.Fatalf("arrow keys outside Top Connections should not change selection, got=%d", a.selectedTalkerIndex)
	}
}

func TestHandleKeyEventArrowKeysStillMoveTopConnectionSelection(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 2
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 80, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED", ProcName: "nginx"},
		{LocalIP: "10.0.0.10", LocalPort: 81, RemoteIP: "198.51.100.20", RemotePort: 52002, State: "ESTABLISHED", ProcName: "api"},
	}
	a.selectedTalkerIndex = 0

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if ret != nil {
		t.Fatalf("down arrow should be consumed in Top Connections")
	}
	if a.selectedTalkerIndex != 1 {
		t.Fatalf("expected selection to move in Top Connections, got=%d", a.selectedTalkerIndex)
	}
}

func TestSelectedPeerKillTargetGroupViewRespectsLocalPortFilter(t *testing.T) {
	t.Parallel()

	a := &App{
		latestTalkers:       topConnectionFixtures(),
		groupView:           true,
		portFilter:          "443",
		selectedTalkerIndex: 0,
	}

	target, ok := a.selectedPeerKillTarget()
	if !ok {
		t.Fatalf("expected selected peer target with group+port filter")
	}
	if target.PeerIP != "198.51.100.20" {
		t.Fatalf("filtered selected peer mismatch: got=%q want=%q", target.PeerIP, "198.51.100.20")
	}
	if target.LocalPort != 443 {
		t.Fatalf("filtered selected local port mismatch: got=%d want=%d", target.LocalPort, 443)
	}
}
