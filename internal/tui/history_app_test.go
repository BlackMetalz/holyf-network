package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newHistoryTestApp(dataDir string, startAt string) *HistoryApp {
	h := NewHistoryApp(dataDir, startAt, false, "test")
	h.panel = tview.NewTextView().SetDynamicColors(true)
	h.statusBar = tview.NewTextView().SetDynamicColors(true)
	h.pages = tview.NewPages()
	h.pages.AddPage("main", tview.NewBox(), true, true)
	h.pages.AddPage("history-help", tview.NewBox(), true, false)
	return h
}

func appendSnapshotFixture(t *testing.T, writer *history.SnapshotWriter, capturedAt time.Time, remoteIP string, state string, activity int64) {
	t.Helper()
	_, err := writer.Append(history.SnapshotRecord{
		CapturedAt: capturedAt,
		Interface:  "eth0",
		TopLimit:   100,
		Connections: []collector.Connection{
			{
				LocalIP:    "10.0.0.1",
				LocalPort:  22,
				RemoteIP:   remoteIP,
				RemotePort: 40000,
				State:      state,
				Activity:   activity,
				PID:        1,
				ProcName:   "sshd",
			},
		},
		Version: "test",
	})
	if err != nil {
		t.Fatalf("append snapshot fixture: %v", err)
	}
}

func TestHistoryHandleKeyEventBracketNavigation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, MaxFiles: 72, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, "198.51.100.10", "ESTABLISHED", 100)
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), "198.51.100.20", "ESTABLISHED", 200)

	h := newHistoryTestApp(dir, HistoryStartLatest)
	h.reloadIndex(true)
	h.renderPanel()

	if h.currentIndex != 1 {
		t.Fatalf("expected start at latest index=1, got=%d", h.currentIndex)
	}

	h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, '[', 0))
	if h.currentIndex != 0 {
		t.Fatalf("expected previous snapshot index=0, got=%d", h.currentIndex)
	}

	h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, ']', 0))
	if h.currentIndex != 1 {
		t.Fatalf("expected next snapshot index=1, got=%d", h.currentIndex)
	}
}

func TestHistoryHandleKeyEventReadOnlyActions(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.refs = []history.SnapshotRef{{}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Connections: []collector.Connection{{RemoteIP: "198.51.100.10"}}}

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'k', 0))
	if ret != nil {
		t.Fatalf("k should be handled in replay mode")
	}
	if !strings.Contains(h.statusNote, "Read-only") {
		t.Fatalf("expected read-only status note after k, got=%q", h.statusNote)
	}

	ret = h.handleKeyEvent(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if ret != nil {
		t.Fatalf("Enter should be handled in replay mode")
	}
	if !strings.Contains(h.statusNote, "Read-only") {
		t.Fatalf("expected read-only status note after Enter, got=%q", h.statusNote)
	}

	name, _ := h.pages.GetFrontPage()
	if name != "main" {
		t.Fatalf("read-only actions should not open kill/block modal, got front page=%q", name)
	}
}

func TestHistoryHandleKeyEventJumpTimeModal(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.refs = []history.SnapshotRef{{CapturedAt: time.Now().UTC()}}
	h.currentIndex = 0

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 't', 0))
	if ret != nil {
		t.Fatalf("t should be handled in replay mode")
	}

	name, _ := h.pages.GetFrontPage()
	if name != "history-jump-time" {
		t.Fatalf("expected jump-time modal, got front page=%q", name)
	}
}

func TestHistoryFilterAppliesToCurrentSnapshotOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, MaxFiles: 72, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, "172.25.110.76", "ESTABLISHED", 100)
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), "172.25.110.77", "ESTABLISHED", 200)

	h := newHistoryTestApp(dir, HistoryStartLatest)
	h.reloadIndex(true)
	h.textFilter = "172.25.110.76"

	if got := len(h.visibleConnections()); got != 0 {
		t.Fatalf("latest snapshot should not match filter, got=%d", got)
	}

	h.navigatePrev()
	if got := len(h.visibleConnections()); got != 1 {
		t.Fatalf("previous snapshot should match filter once, got=%d", got)
	}
}

func TestHistoryGroupViewAndSortModes(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.refs = []history.SnapshotRef{{}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Connections: []collector.Connection{
		{RemoteIP: "198.51.100.20", LocalPort: 22, State: "ESTABLISHED", Activity: 50, PID: 1, ProcName: "sshd"},
		{RemoteIP: "198.51.100.10", LocalPort: 22, State: "CLOSE_WAIT", Activity: 10, PID: 2, ProcName: "nginx"},
	}}

	h.sortMode = SortByQueue
	conns := h.visibleConnections()
	if len(conns) != 2 || normalizeIP(conns[0].RemoteIP) != "198.51.100.20" {
		t.Fatalf("queue sort should prioritize higher activity, got=%+v", conns)
	}

	h.sortMode = SortByState
	conns = h.visibleConnections()
	if len(conns) != 2 || conns[0].State != "CLOSE_WAIT" {
		t.Fatalf("state sort should order alphabetically by state, got=%+v", conns)
	}

	h.groupView = true
	groups := h.visibleGroups()
	if len(groups) == 0 {
		t.Fatalf("expected non-empty grouped view")
	}
}

func TestParseHistoryJumpTime(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*3600)
	now := time.Date(2026, 3, 5, 9, 30, 0, 0, loc)

	tests := []struct {
		name     string
		raw      string
		want     time.Time
		wantFail bool
	}{
		{
			name: "full datetime",
			raw:  "2026-03-04 20:00:30",
			want: time.Date(2026, 3, 4, 20, 0, 30, 0, loc),
		},
		{
			name: "clock only uses today",
			raw:  "20:15",
			want: time.Date(2026, 3, 5, 20, 15, 0, 0, loc),
		},
		{
			name: "yesterday clock",
			raw:  "yesterday 20:10",
			want: time.Date(2026, 3, 4, 20, 10, 0, 0, loc),
		},
		{
			name:     "invalid",
			raw:      "tomorrow morning",
			wantFail: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseHistoryJumpTime(tc.raw, now)
			if tc.wantFail {
				if err == nil {
					t.Fatalf("expected parse failure for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse failed for %q: %v", tc.raw, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("parsed time mismatch for %q: got=%s want=%s", tc.raw, got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

func TestHistoryClosestSnapshotIndex(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	h.refs = []history.SnapshotRef{
		{CapturedAt: base},
		{CapturedAt: base.Add(10 * time.Minute)},
		{CapturedAt: base.Add(20 * time.Minute)},
	}

	tests := []struct {
		name   string
		target time.Time
		want   int
	}{
		{name: "before oldest", target: base.Add(-1 * time.Minute), want: 0},
		{name: "after latest", target: base.Add(25 * time.Minute), want: 2},
		{name: "nearest middle below", target: base.Add(7 * time.Minute), want: 1},
		{name: "nearest lower", target: base.Add(2 * time.Minute), want: 0},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := h.closestSnapshotIndex(tc.target)
			if got != tc.want {
				t.Fatalf("closest index mismatch: got=%d want=%d", got, tc.want)
			}
		})
	}
}
