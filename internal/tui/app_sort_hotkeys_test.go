package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newSortHotkeyTestApp(startMode SortMode, selectedIndex int) *App {
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
		{name: "queue", key: 'Q', want: SortByQueue, ok: true},
		{name: "state", key: 'S', want: SortByState, ok: true},
		{name: "peer", key: 'P', want: SortByPeer, ok: true},
		{name: "process", key: 'R', want: SortByProcess, ok: true},
		{name: "lowercase ignored", key: 'q', want: SortByQueue, ok: false},
		{name: "non sort key", key: 'x', want: SortByQueue, ok: false},
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
		wantMode  SortMode
	}{
		{name: "cycle with o", key: 'o', startMode: SortByQueue, wantMode: SortByState},
		{name: "direct queue", key: 'Q', startMode: SortByProcess, wantMode: SortByQueue},
		{name: "direct state", key: 'S', startMode: SortByQueue, wantMode: SortByState},
		{name: "direct peer", key: 'P', startMode: SortByQueue, wantMode: SortByPeer},
		{name: "direct process", key: 'R', startMode: SortByQueue, wantMode: SortByProcess},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newSortHotkeyTestApp(tc.startMode, 4)
			before := a.panels[2].GetText(true)

			ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, tc.key, 0))
			if ret != nil {
				t.Fatalf("expected nil return for handled key %q, got event back", tc.key)
			}
			if a.sortMode != tc.wantMode {
				t.Fatalf("sort mode mismatch for key %q: got=%v want=%v", tc.key, a.sortMode, tc.wantMode)
			}
			if a.selectedTalkerIndex != 0 {
				t.Fatalf("selectedTalkerIndex should reset to 0, got=%d", a.selectedTalkerIndex)
			}

			after := a.panels[2].GetText(true)
			if after == before {
				t.Fatalf("panel text did not rerender for key %q", tc.key)
			}
		})
	}
}

func TestDirectSortThenCycleTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key      rune
		wantNext SortMode
	}{
		{key: 'Q', wantNext: SortByState},
		{key: 'S', wantNext: SortByPeer},
		{key: 'P', wantNext: SortByProcess},
		{key: 'R', wantNext: SortByQueue},
	}

	for _, tc := range tests {
		mode, ok := directSortModeForRune(tc.key)
		if !ok {
			t.Fatalf("expected direct sort key %q to be mapped", tc.key)
		}
		if got := NextSortMode(mode); got != tc.wantNext {
			t.Fatalf("next sort mismatch for key %q: got=%v want=%v", tc.key, got, tc.wantNext)
		}
	}
}

func TestTopConnectionsSortHintsIncludeCycleAndDirect(t *testing.T) {
	t.Parallel()

	panel := renderTalkersPanel(nil, "", "", 20, false, 0, SortByQueue)
	if !strings.Contains(panel, "o=cycle sort") {
		t.Fatalf("hint line should mention cycle sort key")
	}
	if !strings.Contains(panel, "Shift+Q/S/P/R=direct sort") {
		t.Fatalf("hint line should mention direct Shift sort keys")
	}
}

func TestStatusHotkeysIncludeDirectSortKeys(t *testing.T) {
	t.Parallel()

	_, plain := statusHotkeysForPage("main")
	if !strings.Contains(plain, "Q/S/P/R") {
		t.Fatalf("main hotkeys should mention direct sort keys, got: %q", plain)
	}
}
