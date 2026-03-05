package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newHistoryTestApp(dataDir string, startAt string) *HistoryApp {
	h := NewHistoryApp(dataDir, startAt, "", false, "test")
	h.panel = tview.NewTextView().SetDynamicColors(true)
	h.statusBar = tview.NewTextView().SetDynamicColors(true)
	h.pages = tview.NewPages()
	h.pages.AddPage("main", tview.NewBox(), true, true)
	h.pages.AddPage("history-help", tview.NewBox(), true, false)
	return h
}

func appendSnapshotFixture(t *testing.T, writer *history.SnapshotWriter, capturedAt time.Time, rows []history.SnapshotGroup) {
	t.Helper()
	_, err := writer.Append(history.SnapshotRecord{
		CapturedAt: capturedAt,
		Interface:  "eth0",
		TopLimit:   500,
		Groups:     rows,
		Version:    "test",
	})
	if err != nil {
		t.Fatalf("append snapshot fixture: %v", err)
	}
}

func TestHistoryHandleKeyEventBracketNavigation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1, TotalQueue: 100}})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1, TotalQueue: 200}})

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

func TestHistoryStartAtLatestSkipsTrailingEmptySnapshots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), nil) // empty latest

	h := newHistoryTestApp(dir, HistoryStartLatest)
	h.reloadIndex(true)

	if h.currentIndex != 0 {
		t.Fatalf("expected replay start to jump to latest non-empty index=0, got=%d", h.currentIndex)
	}
}

func TestHistoryHandleKeyEventReadOnlyActions(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.refs = []history.SnapshotRef{{}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Groups: []history.SnapshotGroup{{PeerIP: "198.51.100.10"}}}

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
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "172.25.110.76", LocalPort: 22, ProcName: "sshd", ConnCount: 3}})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{{PeerIP: "172.25.110.77", LocalPort: 22, ProcName: "sshd", ConnCount: 2}})

	h := newHistoryTestApp(dir, HistoryStartLatest)
	h.reloadIndex(true)
	h.textFilter = "172.25.110.76"

	if got := len(h.visibleRows()); got != 0 {
		t.Fatalf("latest snapshot should not match filter, got=%d", got)
	}

	h.navigatePrev()
	if got := len(h.visibleRows()); got != 1 {
		t.Fatalf("previous snapshot should match filter once, got=%d", got)
	}
}

func TestHistoryNavigationSkipsEmptyByDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), nil)
	appendSnapshotFixture(t, writer, base.Add(2*time.Minute), nil)
	appendSnapshotFixture(t, writer, base.Add(3*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})

	h := newHistoryTestApp(dir, HistoryStartOldest)
	h.reloadIndex(true)
	if h.currentIndex != 0 {
		t.Fatalf("expected start at oldest non-empty index=0, got=%d", h.currentIndex)
	}

	h.navigateNext()
	if h.currentIndex != 3 {
		t.Fatalf("expected next to skip empties and jump to index=3, got=%d", h.currentIndex)
	}

	h.navigatePrev()
	if h.currentIndex != 0 {
		t.Fatalf("expected prev to skip empties and jump back to index=0, got=%d", h.currentIndex)
	}
}

func TestHistoryToggleSkipEmptyWithX(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.refs = []history.SnapshotRef{{ConnCount: 1}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Groups: []history.SnapshotGroup{{PeerIP: "198.51.100.10"}}}
	h.skipEmpty = true

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'x', 0))
	if ret != nil {
		t.Fatalf("x should be handled in replay mode")
	}
	if h.skipEmpty {
		t.Fatalf("expected skip-empty to be disabled after x")
	}

	ret = h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'x', 0))
	if ret != nil {
		t.Fatalf("x should be handled in replay mode")
	}
	if !h.skipEmpty {
		t.Fatalf("expected skip-empty to be enabled after second x")
	}
}

func TestHistoryBuildJumpSummaryMentionsMissingDate(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.Local)
	h.refs = []history.SnapshotRef{
		{CapturedAt: base},
		{CapturedAt: base.Add(10 * time.Minute)},
	}

	msg := h.buildJumpSummary(time.Date(2026, 3, 1, 20, 0, 0, 0, time.Local), 0)
	if !strings.Contains(msg, "No snapshots for 2026-03-01") {
		t.Fatalf("jump summary should mention missing date, got=%q", msg)
	}
	if !strings.Contains(msg, "Jumped to") {
		t.Fatalf("jump summary should include jump target, got=%q", msg)
	}
}

func TestHistoryStatusBarKeepsLastMessageAfterTTL(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.statusBar = tview.NewTextView().SetDynamicColors(true)
	h.setStatusNote("test message", 100*time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	h.updateStatusBar()
	text := h.statusBar.GetText(true)
	if !strings.Contains(text, "Last:test message") {
		t.Fatalf("status bar should keep last message after ttl, got=%q", text)
	}
}

func TestHistoryAggregateSortModes(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.refs = []history.SnapshotRef{{}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Groups: []history.SnapshotGroup{
		{PeerIP: "198.51.100.20", LocalPort: 443, ProcName: "sshd", ConnCount: 5, TotalQueue: 10, States: map[string]int{"ESTABLISHED": 5}},
		{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "nginx", ConnCount: 2, TotalQueue: 50, States: map[string]int{"CLOSE_WAIT": 2}},
	}}

	h.sortMode = SortByQueue
	rows := h.visibleRows()
	if len(rows) != 2 || rows[0].PeerIP != "198.51.100.10" {
		t.Fatalf("queue sort should prioritize higher queue, got=%+v", rows)
	}

	h.sortMode = SortByConns
	rows = h.visibleRows()
	if len(rows) != 2 || rows[0].PeerIP != "198.51.100.20" {
		t.Fatalf("conns sort should prioritize higher connection count, got=%+v", rows)
	}

	h.sortMode = SortByPort
	rows = h.visibleRows()
	if len(rows) != 2 || rows[0].LocalPort != 443 {
		t.Fatalf("port sort (desc default) should prioritize higher local port, got=%+v", rows)
	}
}

func TestHistoryAggregateSortDirectionToggle(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir(), HistoryStartLatest)
	h.refs = []history.SnapshotRef{{}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Groups: []history.SnapshotGroup{
		{PeerIP: "198.51.100.20", LocalPort: 443, ProcName: "sshd", ConnCount: 5, TotalQueue: 10},
		{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "nginx", ConnCount: 2, TotalQueue: 50},
	}}

	h.sortMode = SortByConns
	h.sortDesc = true
	rows := h.visibleRows()
	if len(rows) != 2 || rows[0].ConnCount != 5 {
		t.Fatalf("desc conns should prioritize higher count, got=%+v", rows)
	}

	h.sortDesc = false
	rows = h.visibleRows()
	if len(rows) != 2 || rows[0].ConnCount != 2 {
		t.Fatalf("asc conns should prioritize lower count, got=%+v", rows)
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
