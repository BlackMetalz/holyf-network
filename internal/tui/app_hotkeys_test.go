package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/tui/actionlog"
	"github.com/BlackMetalz/holyf-network/internal/tui/blocking"
	"github.com/BlackMetalz/holyf-network/internal/tui/diagnosis"
	"github.com/BlackMetalz/holyf-network/internal/tui/livetrace"
	tuioverlays "github.com/BlackMetalz/holyf-network/internal/tui/overlays"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuireplay "github.com/BlackMetalz/holyf-network/internal/tui/replay"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/BlackMetalz/holyf-network/internal/tui/traffic"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// --- from app_sort_hotkeys_test.go ---

func newSortHotkeyTestApp(startMode tuishared.SortMode, startDesc bool, selectedIndex int) *App {
	panels := []*tview.TextView{
		tview.NewTextView(),
		tview.NewTextView(),
		tview.NewTextView(),
	}
	panels[2].SetText("before")

	pages := tview.NewPages()
	pages.AddPage("main", tview.NewBox(), true, true)

	return &App{
		app:                 tview.NewApplication(),
		pages:               pages,
		panels:              panels,
		statusBar:           tview.NewTextView(),
		focusIndex:          2,
		sortMode:            startMode,
		sortDesc:            startDesc,
		selectedTalkerIndex: selectedIndex,
		blockManager:        blocking.NewManager(),
		trafficManager:      traffic.NewManager(config.DefaultHealthThresholds()),
		actionLogger:        actionlog.NewLogger(""),
		diagnosisEngine:     diagnosis.NewEngine(),
		traceEngine:         livetrace.NewEngineLoaded(),
	}
}

func TestDirectSortModeForRune(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  rune
		want tuishared.SortMode
		ok   bool
	}{
		{name: "bandwidth", key: 'B', want: tuishared.SortByBandwidth, ok: true},
		{name: "conns", key: 'C', want: tuishared.SortByConns, ok: true},
		{name: "port", key: 'P', want: tuishared.SortByPort, ok: true},
		{name: "state removed", key: 'S', want: tuishared.SortByBandwidth, ok: false},
		{name: "process removed", key: 'R', want: tuishared.SortByBandwidth, ok: false},
		{name: "lowercase ignored", key: 'q', want: tuishared.SortByBandwidth, ok: false},
		{name: "non sort key", key: 'x', want: tuishared.SortByBandwidth, ok: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := directSortModeForRune(tc.key)
			if ok != tc.ok {
				t.Fatalf("ok mismatch for key %q: got=%v want=%v", tc.key, ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("mode mismatch for key %q: got=%v want=%v", tc.key, got, tc.want)
			}
		})
	}
}

func TestHandleKeyEventSortKeysResetSelectionAndRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		key       rune
		startMode tuishared.SortMode
		startDesc bool
		wantMode  tuishared.SortMode
		wantDesc  bool
		handled   bool
	}{
		{name: "bandwidth first press keeps desc", key: 'B', startMode: tuishared.SortByConns, startDesc: false, wantMode: tuishared.SortByBandwidth, wantDesc: true, handled: true},
		{name: "same mode toggles to asc", key: 'B', startMode: tuishared.SortByBandwidth, startDesc: true, wantMode: tuishared.SortByBandwidth, wantDesc: false, handled: true},
		{name: "same mode toggles back to desc", key: 'B', startMode: tuishared.SortByBandwidth, startDesc: false, wantMode: tuishared.SortByBandwidth, wantDesc: true, handled: true},
		{name: "conns mode first press desc", key: 'C', startMode: tuishared.SortByBandwidth, startDesc: true, wantMode: tuishared.SortByConns, wantDesc: true, handled: true},
		{name: "port mode first press desc", key: 'P', startMode: tuishared.SortByBandwidth, startDesc: true, wantMode: tuishared.SortByPort, wantDesc: true, handled: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newSortHotkeyTestApp(tc.startMode, tc.startDesc, 4)
			before := a.panels[2].GetText(true)

			ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, tc.key, 0))
			if tc.handled && ret != nil {
				t.Fatalf("expected nil return for handled key %q, got event back", tc.key)
			}
			if !tc.handled && ret == nil {
				t.Fatalf("expected unhandled key %q to return event", tc.key)
			}
			if a.sortMode != tc.wantMode {
				t.Fatalf("sort mode mismatch for key %q: got=%v want=%v", tc.key, a.sortMode, tc.wantMode)
			}
			if a.sortDesc != tc.wantDesc {
				t.Fatalf("sort direction mismatch for key %q: got=%v want=%v", tc.key, a.sortDesc, tc.wantDesc)
			}
			if tc.handled && a.selectedTalkerIndex != 0 {
				t.Fatalf("selectedTalkerIndex should reset to 0, got=%d", a.selectedTalkerIndex)
			}

			after := a.panels[2].GetText(true)
			if tc.handled && after == before {
				t.Fatalf("panel text did not rerender for key %q", tc.key)
			}
		})
	}
}

func TestHandleKeyEventOTogglesTopConnectionsDirection(t *testing.T) {
	t.Parallel()

	a := newSortHotkeyTestApp(tuishared.SortByBandwidth, true, 4)
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 18080, RemoteIP: "172.25.110.137", RemotePort: 52001, State: "ESTABLISHED"},
		{LocalIP: "10.0.0.10", LocalPort: 52246, RemoteIP: "20.205.243.168", RemotePort: 443, State: "ESTABLISHED"},
	}
	a.listenPorts = map[int]struct{}{18080: {}}
	a.listenPortsKnown = true

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'o', 0))
	if ret != nil {
		t.Fatalf("o should be handled")
	}
	if a.topDirection != tuishared.TopConnectionOutgoing {
		t.Fatalf("expected o to switch top direction to OUT, got=%v", a.topDirection)
	}
	if a.selectedTalkerIndex != 0 {
		t.Fatalf("expected selection reset on direction toggle, got=%d", a.selectedTalkerIndex)
	}
	panel := a.panels[2].GetText(true)
	if !strings.Contains(panel, "Dir=OUT") {
		t.Fatalf("expected rerendered panel to show Dir=OUT, got: %q", panel)
	}
}

func TestTopConnectionsSortHintsIncludeDirectOnly(t *testing.T) {
	t.Parallel()

	panel := tuipanels.RenderTalkersPanel(nil, "", "", 20, false, 0, tuishared.SortByBandwidth, true, config.DefaultHealthThresholds(), "")
	if strings.Contains(panel, "o=cycle sort") {
		t.Fatalf("hint line should not mention cycle sort key")
	}
	if !strings.Contains(panel, "Shift+B/C/P sort") {
		t.Fatalf("hint line should mention direct sort keys")
	}
	if !strings.Contains(panel, "T=trace packet") {
		t.Fatalf("hint line should mention trace hotkey")
	}
	if !strings.Contains(panel, "t=trace history") {
		t.Fatalf("hint line should mention trace history hotkey")
	}
}

func TestStatusHotkeysIncludeHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		focus     int
		direction tuishared.TopConnectionDirection
		want      []string
	}{
		{name: "top incoming", focus: 2, direction: tuishared.TopConnectionIncoming, want: []string{"Up/Down=select", "o=OUT", "T=trace", "t=traces", "Enter/k=act", "?=help"}},
		{name: "top outgoing", focus: 2, direction: tuishared.TopConnectionOutgoing, want: []string{"Up/Down=select", "o=IN", "T=trace", "t=traces", "Enter/k=disabled", "?=help"}},
		{name: "system health", focus: 0, direction: tuishared.TopConnectionIncoming, want: []string{"s=sort", "Ctrl+1=dashboard", "Ctrl+2=chart", "?=help"}},
		{name: "diagnosis", focus: 1, direction: tuishared.TopConnectionIncoming, want: []string{"d=history", "Ctrl+1=dashboard", "Ctrl+2=chart", "?=help"}},
	}

	for _, tc := range tests {
		_, plain := tuioverlays.LiveMainStatusHotkeys(tc.focus, tc.direction)
		for _, want := range tc.want {
			if !strings.Contains(plain, want) {
				t.Fatalf("%s hotkeys should contain %q, got: %q", tc.name, want, plain)
			}
		}
	}
}

// --- from status_hotkeys_test.go ---

func TestHistoryStatusHotkeysForModalPages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		page      string
		wantPlain string
	}{
		{page: "history-help", wantPlain: "any key=close"},
		{page: "history-filter", wantPlain: "Enter=apply Esc=cancel"},
		{page: "history-search", wantPlain: "Enter=apply Esc=cancel"},
		{page: "history-jump-time", wantPlain: "Enter=apply Esc=cancel"},
		{page: "history-timeline-search", wantPlain: "Enter=search Esc=cancel"},
		{page: "history-timeline-results", wantPlain: "Up/Down=select Enter=jump Esc=close"},
		{page: tuireplay.HistoryTracePage, wantPlain: "Up/Down=select Enter=detail c=compare Esc=close"},
		{page: tuireplay.HistoryTraceDetailPage, wantPlain: "Enter=close Esc=close"},
		{page: tuireplay.HistoryTraceComparePage, wantPlain: "Enter=close Esc=close"},
		{page: "main", wantPlain: "[=prev ]=next a e t f / Shift+S Shift+B/C/P o g h m i Shift+I x z L ? q"},
		{page: "history-socket-queue-explain", wantPlain: "Enter=close Esc=close"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.page, func(t *testing.T) {
			t.Parallel()
			_, plain := tuireplay.StatusHotkeysForPage(tc.page)
			if plain != tc.wantPlain {
				t.Fatalf("plain hotkeys mismatch for page=%q: got=%q want=%q", tc.page, plain, tc.wantPlain)
			}
		})
	}
}

// --- from help_test.go ---

func TestBuildLiveHelpTextTopOutgoingGroup(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 2
	a.topDirection = tuishared.TopConnectionOutgoing
	a.groupView = true

	text := tuioverlays.BuildLiveHelpText(tuioverlays.LiveHelpContext{FocusIndex: a.focusIndex, Direction: a.topDirection, GroupView: a.groupView})
	for _, want := range []string{
		"Current Panel",
		"Top Connections (OUT, group view)",
		"Toggle to IN mode",
		"Switch to connections view",
		"Trace packet for selected peer/port",
		"Open trace packet history",
		"Disabled in OUT mode",
		"Global Navigation",
		"Other Panels",
		"System Health",
		"Diagnosis",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q, got: %q", want, text)
		}
	}
	if strings.Count(text, "Top Connections (OUT, group view)") != 1 {
		t.Fatalf("current panel should not be repeated under Other Panels: %q", text)
	}
}

func TestBuildLiveHelpTextDiagnosisFocus(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 1

	text := tuioverlays.BuildLiveHelpText(tuioverlays.LiveHelpContext{FocusIndex: a.focusIndex, Direction: a.topDirection, GroupView: a.groupView})
	for _, want := range []string{
		"Current Panel",
		"Diagnosis",
		"Show diagnosis history",
		"Top Connections",
		"Logs / Blocks",
		"t trace history",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q, got: %q", want, text)
		}
	}
}
