package tui

import (
	"strings"
	"testing"

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
		{name: "o removed", key: 'o', startMode: SortByBandwidth, startDesc: true, wantMode: SortByBandwidth, wantDesc: true, handled: false},
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

func TestTopConnectionsSortHintsIncludeDirectOnly(t *testing.T) {
	t.Parallel()

	panel := renderTalkersPanel(nil, "", "", 20, false, 0, SortByBandwidth, true, config.DefaultHealthThresholds(), false)
	if strings.Contains(panel, "o=cycle sort") {
		t.Fatalf("hint line should not mention cycle sort key")
	}
	if !strings.Contains(panel, "Shift+B/C/P sort") {
		t.Fatalf("hint line should mention direct sort keys")
	}
}

func TestStatusHotkeysIncludeDirectSortKeys(t *testing.T) {
	t.Parallel()

	_, plain := statusHotkeysForPage("main")
	if strings.Contains(plain, " o ") {
		t.Fatalf("main hotkeys should not mention o, got: %q", plain)
	}
	if !strings.Contains(plain, "Shift+B/C/P") {
		t.Fatalf("main hotkeys should mention direct sort keys, got: %q", plain)
	}
}
