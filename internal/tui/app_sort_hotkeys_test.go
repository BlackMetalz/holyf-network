package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newSortHotkeyTestApp(startMode SortMode, startDesc bool, selectedIndex int) *App {
	panels := []*tview.TextView{
		tview.NewTextView(),
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
	}
}

func TestDirectSortModeForRune(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  rune
		want SortMode
		ok   bool
	}{
		{name: "bandwidth", key: 'B', want: SortByBandwidth, ok: true},
		{name: "conns", key: 'C', want: SortByConns, ok: true},
		{name: "port", key: 'P', want: SortByPort, ok: true},
		{name: "state removed", key: 'S', want: SortByBandwidth, ok: false},
		{name: "process removed", key: 'R', want: SortByBandwidth, ok: false},
		{name: "lowercase ignored", key: 'q', want: SortByBandwidth, ok: false},
		{name: "non sort key", key: 'x', want: SortByBandwidth, ok: false},
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
		startMode SortMode
		startDesc bool
		wantMode  SortMode
		wantDesc  bool
		handled   bool
	}{
		{name: "bandwidth first press keeps desc", key: 'B', startMode: SortByConns, startDesc: false, wantMode: SortByBandwidth, wantDesc: true, handled: true},
		{name: "same mode toggles to asc", key: 'B', startMode: SortByBandwidth, startDesc: true, wantMode: SortByBandwidth, wantDesc: false, handled: true},
		{name: "same mode toggles back to desc", key: 'B', startMode: SortByBandwidth, startDesc: false, wantMode: SortByBandwidth, wantDesc: true, handled: true},
		{name: "conns mode first press desc", key: 'C', startMode: SortByBandwidth, startDesc: true, wantMode: SortByConns, wantDesc: true, handled: true},
		{name: "port mode first press desc", key: 'P', startMode: SortByBandwidth, startDesc: true, wantMode: SortByPort, wantDesc: true, handled: true},
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

	a := newSortHotkeyTestApp(SortByBandwidth, true, 4)
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
	if a.topDirection != topConnectionOutgoing {
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

	panel := renderTalkersPanel(nil, "", "", 20, false, 0, SortByBandwidth, true, config.DefaultHealthThresholds(), "")
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
		direction topConnectionDirection
		want      []string
	}{
		{name: "top incoming", focus: 2, direction: topConnectionIncoming, want: []string{"Up/Down=select", "o=OUT", "T=trace", "t=traces", "y=profile", "Y=p-help", "Enter/k=act", "?=help"}},
		{name: "top outgoing", focus: 2, direction: topConnectionOutgoing, want: []string{"Up/Down=select", "o=IN", "T=trace", "t=traces", "y=profile", "Y=p-help", "Enter/k=disabled", "?=help"}},
		{name: "states", focus: 0, direction: topConnectionIncoming, want: []string{"s=sort", "y=profile", "Y=p-help", "Ctrl+1..5=focus", "?=help"}},
		{name: "diagnosis", focus: 4, direction: topConnectionIncoming, want: []string{"d=history", "y=profile", "Y=p-help", "Ctrl+1..5=focus", "?=help"}},
	}

	for _, tc := range tests {
		_, plain := liveMainStatusHotkeys(tc.focus, tc.direction)
		for _, want := range tc.want {
			if !strings.Contains(plain, want) {
				t.Fatalf("%s hotkeys should contain %q, got: %q", tc.name, want, plain)
			}
		}
	}
}
