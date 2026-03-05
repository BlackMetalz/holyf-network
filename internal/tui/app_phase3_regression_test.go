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
		{page: "blocked-peers", wantPlain: "Up/Down=select Enter=remove Del=remove Tab=buttons Esc=close"},
		{page: "action-log", wantPlain: "Enter=close Esc=close"},
		{page: "blocked-peers-remove-result", wantPlain: "Enter=close Esc=close"},
		{page: "block-summary", wantPlain: "Enter=close Esc=close"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.page, func(t *testing.T) {
			t.Parallel()
			_, plain := statusHotkeysForPage(tc.page)
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

func TestSelectedPeerKillTargetGroupViewUsesSelectedPeerContext(t *testing.T) {
	t.Parallel()

	a := &App{
		latestTalkers:       topConnectionFixtures(),
		groupView:           true,
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
