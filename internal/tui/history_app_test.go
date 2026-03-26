package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/history"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuireplay "github.com/BlackMetalz/holyf-network/internal/tui/replay"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newHistoryTestApp(dataDir string) *HistoryApp {
	h := NewHistoryApp(dataDir, "", false, "test", nil, nil)
	h.panel = tview.NewTextView().SetDynamicColors(true)
	h.statusBar = tview.NewTextView().SetDynamicColors(true)
	h.pages = tview.NewPages()
	h.pages.AddPage("main", tview.NewBox(), true, true)
	h.pages.AddPage("history-help", tview.NewBox(), true, false)
	return h
}

func newHistoryTestAppWithRange(dataDir string, begin, end *time.Time) *HistoryApp {
	h := NewHistoryApp(dataDir, "", false, "test", begin, end)
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
		CapturedAt:      capturedAt,
		Interface:       "eth0",
		TopLimitPerSide: 500,
		IncomingGroups:  rows,
		OutgoingGroups:  []history.SnapshotGroup{},
		Version:         "test",
	})
	if err != nil {
		t.Fatalf("append snapshot fixture: %v", err)
	}
}

func appendDirectionalSnapshotFixture(t *testing.T, writer *history.SnapshotWriter, capturedAt time.Time, incomingRows, outgoingRows []history.SnapshotGroup) {
	t.Helper()
	_, err := writer.Append(history.SnapshotRecord{
		CapturedAt:      capturedAt,
		Interface:       "eth0",
		TopLimitPerSide: 500,
		IncomingGroups:  incomingRows,
		OutgoingGroups:  outgoingRows,
		Version:         "test",
	})
	if err != nil {
		t.Fatalf("append directional snapshot fixture: %v", err)
	}
}

func appendTraceHistoryFixture(t *testing.T, dataDir string, entry tuitrace.Entry) {
	t.Helper()
	if entry.CapturedAt.IsZero() {
		t.Fatalf("trace history fixture requires captured_at")
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir trace data dir: %v", err)
	}
	path := filepath.Join(dataDir, tuitrace.SegmentFileName(entry.CapturedAt))
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal trace history fixture: %v", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open trace history fixture file: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		t.Fatalf("append trace history fixture: %v", err)
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

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)
	h.renderPanel()

	if h.currentIndex != 0 {
		t.Fatalf("expected start at oldest index=0, got=%d", h.currentIndex)
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

func TestHistoryReplayRendersTraceTimelineEvents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "203.0.113.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(30*time.Second), []history.SnapshotGroup{{PeerIP: "203.0.113.20", LocalPort: 443, ProcName: "nginx", ConnCount: 1}})

	appendTraceHistoryFixture(t, dir, tuitrace.Entry{
		CapturedAt: base.Add(3 * time.Second),
		PeerIP:     "203.0.113.10",
		Port:       22,
		Preset:     "SYN/RST only",
		Scope:      "SYN/RST only",
		Severity:   "WARN",
		Issue:      "RST pressure observed",
	})
	appendTraceHistoryFixture(t, dir, tuitrace.Entry{
		CapturedAt: base.Add(28 * time.Second),
		PeerIP:     "203.0.113.20",
		Port:       443,
		Preset:     "Peer only",
		Scope:      "Peer only",
		Severity:   "INFO",
		Issue:      "No strong packet-level anomaly",
	})

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)
	h.renderPanel()
	h.updateStatusBar()

	text := h.panel.GetText(true)
	if !strings.Contains(text, "Trace timeline: 1 event(s) near this snapshot") {
		t.Fatalf("expected trace timeline section on first snapshot, got=%q", text)
	}
	if !strings.Contains(text, "SYN/RST only") {
		t.Fatalf("expected first snapshot category, got=%q", text)
	}
	status := h.statusBar.GetText(true)
	if !strings.Contains(status, "TRACE 1/2") {
		t.Fatalf("expected trace count in status bar, got=%q", status)
	}

	h.navigateNext()
	text = h.panel.GetText(true)
	if !strings.Contains(text, "Trace timeline: 1 event(s) near this snapshot") {
		t.Fatalf("expected trace timeline section on second snapshot, got=%q", text)
	}
	if !strings.Contains(text, "Peer only") {
		t.Fatalf("expected second snapshot category, got=%q", text)
	}
}

func TestHistoryReplayFallsBackToTraceOnlyMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	capturedAt := time.Date(2026, 3, 22, 10, 30, 0, 0, time.UTC)
	appendTraceHistoryFixture(t, dir, tuitrace.Entry{
		CapturedAt: capturedAt,
		PeerIP:     "203.0.113.40",
		Port:       443,
		Preset:     "Custom",
		Scope:      "Custom (Peer+Port)",
		Severity:   "INFO",
		Issue:      "No strong packet-level anomaly",
	})

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)
	h.renderPanel()
	h.updateStatusBar()

	if !h.traceOnlyMode {
		t.Fatalf("expected trace-only mode when no snapshots exist")
	}
	if len(h.refs) != 1 || h.currentIndex != 0 {
		t.Fatalf("expected one synthetic trace ref selected at index 0, refs=%d idx=%d", len(h.refs), h.currentIndex)
	}
	if got := h.currentRecord.Interface; got != "trace-history" {
		t.Fatalf("expected synthetic trace interface, got=%q", got)
	}

	panel := h.panel.GetText(true)
	if !strings.Contains(panel, "Trace-only replay mode") {
		t.Fatalf("expected trace-only hint in panel, got=%q", panel)
	}
	if !strings.Contains(panel, "Trace timeline: 1 event(s) at current slot") {
		t.Fatalf("expected trace timeline section in panel, got=%q", panel)
	}

	status := h.statusBar.GetText(true)
	if !strings.Contains(status, "TRACE-ONLY") || !strings.Contains(status, "Trace: 1/1") {
		t.Fatalf("expected trace-only status bar markers, got=%q", status)
	}
}

func TestHistoryTraceOnlyModeDisablesTimelineSearch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	appendTraceHistoryFixture(t, dir, tuitrace.Entry{
		CapturedAt: time.Date(2026, 3, 22, 10, 45, 0, 0, time.UTC),
		PeerIP:     "203.0.113.41",
		Port:       22,
		Preset:     "SYN/RST only",
		Scope:      "SYN/RST only",
		Severity:   "WARN",
		Issue:      "RST pressure observed",
	})

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'S', 0))
	if ret != nil {
		t.Fatalf("Shift+S should be consumed in trace-only mode")
	}
	if !strings.Contains(h.statusNote, "unavailable in trace-only replay") {
		t.Fatalf("expected trace-only timeline-search note, got=%q", h.statusNote)
	}
	name, _ := h.pages.GetFrontPage()
	if name != "main" {
		t.Fatalf("trace-only timeline search should not open modal, front page=%q", name)
	}
}

func TestHistoryStartAtOldestSkipsLeadingEmptySnapshots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, nil) // empty oldest
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)

	if h.currentIndex != 1 {
		t.Fatalf("expected replay start to jump to oldest non-empty index=1, got=%d", h.currentIndex)
	}
}

func TestHistoryHandleKeyEventReadOnlyActions(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
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

	h := newHistoryTestApp(t.TempDir())
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

func TestHistoryHandleKeyEventIShowsSocketQueueExplain(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.refs = []history.SnapshotRef{{CapturedAt: time.Now().UTC()}}
	h.currentIndex = 0

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'i', 0))
	if ret != nil {
		t.Fatalf("i should be handled in replay mode")
	}

	name, _ := h.pages.GetFrontPage()
	if name != "history-socket-queue-explain" {
		t.Fatalf("expected socket queue explain modal, got front page=%q", name)
	}
}

func TestHistoryHandleKeyEventShiftSShowsTimelineSearchModal(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.refs = []history.SnapshotRef{{CapturedAt: time.Now().UTC()}}
	h.currentIndex = 0

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'S', 0))
	if ret != nil {
		t.Fatalf("Shift+S should be handled in replay mode")
	}

	name, _ := h.pages.GetFrontPage()
	if name != "history-timeline-search" {
		t.Fatalf("expected timeline-search modal, got front page=%q", name)
	}
}

func TestHistoryHandleKeyEventHShowsReplayTraceHistoryModal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	appendTraceHistoryFixture(t, dir, tuitrace.Entry{
		CapturedAt: time.Date(2026, 3, 22, 11, 0, 0, 0, time.UTC),
		PeerIP:     "203.0.113.60",
		Port:       443,
		Preset:     "Peer + Port",
		Scope:      "Peer + Port",
		Severity:   "INFO",
		Issue:      "No strong packet-level anomaly",
	})

	h := newHistoryTestApp(dir)
	h.refs = []history.SnapshotRef{{CapturedAt: time.Now().UTC()}}
	h.currentIndex = 0

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'h', 0))
	if ret != nil {
		t.Fatalf("h should be handled in replay mode")
	}

	name, _ := h.pages.GetFrontPage()
	if name != tuireplay.HistoryTracePage {
		t.Fatalf("expected replay trace-history modal, got front page=%q", name)
	}
}

func TestShowReplayTraceHistoryCompareOpensComparePage(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	baseline := tuitrace.Entry{
		CapturedAt:       time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
		PeerIP:           "203.0.113.10",
		Port:             443,
		DecodedPackets:   100,
		ReceivedByFilter: 100,
		DroppedByKernel:  1,
		SynCount:         20,
		SynAckCount:      18,
		RstCount:         2,
	}
	incident := tuitrace.Entry{
		CapturedAt:       time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC),
		PeerIP:           "203.0.113.10",
		Port:             443,
		DecodedPackets:   120,
		ReceivedByFilter: 120,
		DroppedByKernel:  12,
		SynCount:         30,
		SynAckCount:      12,
		RstCount:         24,
	}

	tuireplay.ShowReplayTraceHistoryCompare(h, baseline, incident, nil)
	name, _ := h.pages.GetFrontPage()
	if name != tuireplay.HistoryTraceComparePage {
		t.Fatalf("expected replay trace compare page %q, got %q", tuireplay.HistoryTraceComparePage, name)
	}
}

func TestHistoryHandleKeyEventGTogglesReplayViewMode(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.refs = []history.SnapshotRef{{CapturedAt: time.Now().UTC(), IncomingCount: 1}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{
		CapturedAt:      time.Now().UTC(),
		Interface:       "eth0",
		TopLimitPerSide: 500,
		IncomingGroups:  []history.SnapshotGroup{{PeerIP: "198.51.100.10", Port: 22, ProcName: "sshd", ConnCount: 1}},
	}

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	if ret != nil {
		t.Fatalf("g should be handled in replay mode")
	}
	if h.replayViewMode != replayViewTrace {
		t.Fatalf("expected replay view mode TRACE, got=%v", h.replayViewMode)
	}

	ret = h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	if ret != nil {
		t.Fatalf("g should be handled in replay mode")
	}
	if h.replayViewMode != replayViewConnections {
		t.Fatalf("expected replay view mode CONN after toggle back, got=%v", h.replayViewMode)
	}
}

func TestHistoryHandleKeyEventOTogglesDirectionAndUsesOutgoingRows(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.refs = []history.SnapshotRef{{IncomingCount: 1, OutgoingCount: 1, TotalCount: 2}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{
		IncomingGroups: []history.SnapshotGroup{{PeerIP: "198.51.100.10", Port: 22, ProcName: "sshd", ConnCount: 1}},
		OutgoingGroups: []history.SnapshotGroup{{PeerIP: "20.205.243.168", Port: 443, ProcName: "curl", ConnCount: 1}},
	}

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'o', 0))
	if ret != nil {
		t.Fatalf("o should be handled in replay mode")
	}
	if h.topDirection != tuishared.TopConnectionOutgoing {
		t.Fatalf("expected replay direction to switch to OUT, got=%v", h.topDirection)
	}
	rows := h.visibleRows()
	if len(rows) != 1 || rows[0].Port != 443 {
		t.Fatalf("expected outgoing rows after toggle, got=%+v", rows)
	}
	text := h.panel.GetText(true)
	if !strings.Contains(text, "Dir=OUT") || !strings.Contains(text, "RPORT") {
		t.Fatalf("expected outgoing replay panel render, got=%q", text)
	}
}

func TestHistoryHandleKeyEventHorizontalArrowsAreDisabled(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.refs = []history.SnapshotRef{
		{CapturedAt: time.Now(), ConnCount: 1},
		{CapturedAt: time.Now().Add(1 * time.Minute), ConnCount: 1},
	}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{
		CapturedAt: time.Now(),
		Interface:  "eth0",
		Groups:     []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}},
	}

	ret := h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if ret != nil {
		t.Fatalf("Right arrow should be consumed in replay")
	}
	if h.currentIndex != 0 {
		t.Fatalf("current index should not change on Right arrow, got=%d", h.currentIndex)
	}

	ret = h.handleKeyEvent(tcell.NewEventKey(tcell.KeyLeft, 0, 0))
	if ret != nil {
		t.Fatalf("Left arrow should be consumed in replay")
	}
	if h.currentIndex != 0 {
		t.Fatalf("current index should not change on Left arrow, got=%d", h.currentIndex)
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

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)
	h.textFilter = "172.25.110.76"

	if got := len(h.visibleRows()); got != 1 {
		t.Fatalf("oldest snapshot should match filter once, got=%d", got)
	}

	h.navigateNext()
	if got := len(h.visibleRows()); got != 0 {
		t.Fatalf("next snapshot should not match filter, got=%d", got)
	}
}

func TestHistoryReloadIndexAppliesBeginRangeFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(2*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.30", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})

	begin := base.Add(1 * time.Minute)
	h := newHistoryTestAppWithRange(dir, &begin, nil)
	h.reloadIndex(true)

	if len(h.refs) != 2 {
		t.Fatalf("expected 2 refs with begin filter, got=%d", len(h.refs))
	}
	if h.currentIndex != 0 {
		t.Fatalf("replay should start at oldest filtered snapshot, got index=%d", h.currentIndex)
	}
	if got := h.refs[0].CapturedAt; !got.Equal(begin) {
		t.Fatalf("expected first ref at begin boundary, got=%s want=%s", got, begin)
	}
}

func TestHistoryReloadIndexAppliesEndRangeFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(2*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.30", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})

	end := base.Add(1 * time.Minute)
	h := newHistoryTestAppWithRange(dir, nil, &end)
	h.reloadIndex(true)

	if len(h.refs) != 2 {
		t.Fatalf("expected 2 refs with end filter, got=%d", len(h.refs))
	}
	if h.currentIndex != 0 {
		t.Fatalf("replay should start at oldest filtered snapshot, got index=%d", h.currentIndex)
	}
	if got := h.refs[len(h.refs)-1].CapturedAt; !got.Equal(end) {
		t.Fatalf("expected last ref at end boundary, got=%s want=%s", got, end)
	}
}

func TestHistoryReloadIndexBeginEndInclusiveAndNoMatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, base.Add(2*time.Minute), []history.SnapshotGroup{{PeerIP: "198.51.100.30", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})

	begin := base.Add(1 * time.Minute)
	end := base.Add(1 * time.Minute)
	h := newHistoryTestAppWithRange(dir, &begin, &end)
	h.reloadIndex(true)
	if len(h.refs) != 1 {
		t.Fatalf("expected exactly 1 ref for inclusive equal begin/end, got=%d", len(h.refs))
	}
	if !h.refs[0].CapturedAt.Equal(begin) {
		t.Fatalf("expected boundary snapshot, got=%s want=%s", h.refs[0].CapturedAt, begin)
	}

	noneBegin := base.Add(3 * time.Minute)
	noneEnd := base.Add(4 * time.Minute)
	hNone := newHistoryTestAppWithRange(dir, &noneBegin, &noneEnd)
	hNone.reloadIndex(true)
	if len(hNone.refs) != 0 {
		t.Fatalf("expected no refs in unmatched range, got=%d", len(hNone.refs))
	}
	hNone.renderPanel()
	if !strings.Contains(hNone.panel.GetText(true), "No snapshots in selected time range") {
		t.Fatalf("expected empty range message, got=%q", hNone.panel.GetText(true))
	}
}

func TestHistoryReloadIndexAppliesFileScopeIntersectRange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	day1 := time.Date(2026, 3, 4, 10, 0, 0, 0, time.Local)
	day2 := day1.Add(36 * time.Hour) // force another daily segment
	appendSnapshotFixture(t, writer, day1, []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})
	appendSnapshotFixture(t, writer, day2, []history.SnapshotGroup{{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1}})

	refs, _, err := history.LoadIndex(dir)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs from fixtures, got=%d", len(refs))
	}
	fileName := refs[0].FilePath
	if refs[1].CapturedAt.Before(refs[0].CapturedAt) {
		fileName = refs[1].FilePath
	}

	begin := day2
	end := day2.Add(1 * time.Hour)
	h := NewHistoryApp(dir, fileName, false, "test", &begin, &end)
	h.panel = tview.NewTextView().SetDynamicColors(true)
	h.statusBar = tview.NewTextView().SetDynamicColors(true)
	h.pages = tview.NewPages()
	h.pages.AddPage("main", tview.NewBox(), true, true)
	h.pages.AddPage("history-help", tview.NewBox(), true, false)

	h.reloadIndex(true)
	if len(h.refs) != 0 {
		t.Fatalf("expected file-scope intersect range to be empty, got=%d", len(h.refs))
	}
}

func TestHistoryTimelineSearchMatchesAcrossLoadedSnapshots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{
		{PeerIP: "198.51.100.10", LocalPort: 443, ProcName: "curl", ConnCount: 1},
		{PeerIP: "198.51.100.11", LocalPort: 443, ProcName: "curl", ConnCount: 1},
	})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{
		{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1},
	})
	appendSnapshotFixture(t, writer, base.Add(2*time.Minute), []history.SnapshotGroup{
		{PeerIP: "198.51.100.30", LocalPort: 8080, ProcName: "curl", ConnCount: 1},
	})

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)

	results := h.scanTimelineMatches("curl", h.refs)
	if len(results) != 2 {
		t.Fatalf("expected 2 matching snapshots, got=%d", len(results))
	}
	if results[0].SnapshotIndex != 0 || results[0].MatchCount != 2 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].SnapshotIndex != 2 || results[1].MatchCount != 1 {
		t.Fatalf("unexpected second result: %+v", results[1])
	}

	noMatch := h.scanTimelineMatches("definitely-not-found", h.refs)
	if len(noMatch) != 0 {
		t.Fatalf("expected no-match result to be empty, got=%d", len(noMatch))
	}
}

func TestHistoryTimelineSearchRespectsCurrentDirection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendDirectionalSnapshotFixture(t, writer, base,
		[]history.SnapshotGroup{{PeerIP: "198.51.100.10", Port: 443, ProcName: "curl", ConnCount: 1}},
		nil,
	)
	appendDirectionalSnapshotFixture(t, writer, base.Add(1*time.Minute),
		nil,
		[]history.SnapshotGroup{{PeerIP: "20.205.243.168", Port: 443, ProcName: "curl", ConnCount: 1}},
	)

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)
	h.topDirection = tuishared.TopConnectionOutgoing

	results := h.scanTimelineMatches("curl", h.refs)
	if len(results) != 1 || results[0].SnapshotIndex != 1 {
		t.Fatalf("expected only outgoing snapshot to match, got=%+v", results)
	}
}

func TestHistoryTimelineSearchJumpKeepsCurrentTextFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writer, err := history.NewSnapshotWriter(history.WriterConfig{DataDir: dir, RetentionHours: 24, PruneEverySnapshots: 10})
	if err != nil {
		t.Fatalf("new snapshot writer: %v", err)
	}
	defer writer.Close()

	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	appendSnapshotFixture(t, writer, base, []history.SnapshotGroup{
		{PeerIP: "198.51.100.10", LocalPort: 443, ProcName: "curl", ConnCount: 1},
	})
	appendSnapshotFixture(t, writer, base.Add(1*time.Minute), []history.SnapshotGroup{
		{PeerIP: "198.51.100.20", LocalPort: 22, ProcName: "sshd", ConnCount: 1},
	})

	h := newHistoryTestApp(dir)
	h.reloadIndex(true)
	if h.currentIndex != 0 {
		t.Fatalf("expected to start at oldest index=0, got=%d", h.currentIndex)
	}

	h.textFilter = "198.51.100.20"
	h.timelineSearchQuery = "curl"
	h.timelineSearchResults = []tuireplay.SearchResult{
		{
			SnapshotIndex: 0,
			CapturedAt:    base,
			MatchCount:    1,
		},
	}

	h.jumpToTimelineSearchResult(0)
	if h.currentIndex != 0 {
		t.Fatalf("expected jump to snapshot index=0, got=%d", h.currentIndex)
	}
	if h.textFilter != "198.51.100.20" {
		t.Fatalf("text filter should remain unchanged after jump, got=%q", h.textFilter)
	}
	if !strings.Contains(h.snapshotMessage, "Timeline match") {
		t.Fatalf("expected timeline jump snapshot message, got=%q", h.snapshotMessage)
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

	h := newHistoryTestApp(dir)
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

	h := newHistoryTestApp(t.TempDir())
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

func TestHistoryToggleDirectionWithSkipEmptyJumpsToNearestActiveSnapshot(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.skipEmpty = true
	h.refs = []history.SnapshotRef{
		{IncomingCount: 1, OutgoingCount: 0, TotalCount: 1},
		{IncomingCount: 0, OutgoingCount: 1, TotalCount: 1},
	}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{
		IncomingGroups: []history.SnapshotGroup{{PeerIP: "198.51.100.10", Port: 22, ProcName: "sshd", ConnCount: 1}},
	}

	h.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'o', 0))
	if h.currentIndex != 1 {
		t.Fatalf("expected skip-empty direction toggle to jump to outgoing-active snapshot, got=%d", h.currentIndex)
	}
}

func TestHistoryRenderHeaderShowsDualIndexWhenSkipEmptyOn(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.skipEmpty = true
	h.refs = []history.SnapshotRef{
		{ConnCount: 1},
		{ConnCount: 0},
		{ConnCount: 3},
	}
	h.currentIndex = 2
	h.currentRecord = history.SnapshotRecord{
		CapturedAt: time.Date(2026, 3, 5, 19, 16, 17, 0, time.FixedZone("+07", 7*3600)),
		Interface:  "eth0",
		Groups:     []history.SnapshotGroup{{PeerIP: "103.160.74.90", LocalPort: 22, ProcName: "sshd", ConnCount: 1}},
	}

	h.renderPanel()
	text := h.panel.GetText(true)
	if !strings.Contains(text, "Snapshot Active 2/2 | Raw 3/3") {
		t.Fatalf("expected dual index in header, got=%q", text)
	}
}

func TestHistoryRenderHeaderShowsRawIndexWhenSkipEmptyOff(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.skipEmpty = false
	h.refs = []history.SnapshotRef{
		{ConnCount: 1},
		{ConnCount: 0},
		{ConnCount: 3},
	}
	h.currentIndex = 2
	h.currentRecord = history.SnapshotRecord{
		CapturedAt: time.Date(2026, 3, 5, 19, 16, 17, 0, time.FixedZone("+07", 7*3600)),
		Interface:  "eth0",
		Groups:     []history.SnapshotGroup{{PeerIP: "103.160.74.90", LocalPort: 22, ProcName: "sshd", ConnCount: 1}},
	}

	h.renderPanel()
	text := h.panel.GetText(true)
	if !strings.Contains(text, "Snapshot 3/3") {
		t.Fatalf("expected raw index in header, got=%q", text)
	}
	if strings.Contains(text, "Snapshot Active") {
		t.Fatalf("header should not show active index when skip-empty is off, got=%q", text)
	}
}

func TestHistoryNavigateNextAtLastActiveShowsBoundaryMessage(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.skipEmpty = true
	h.refs = []history.SnapshotRef{
		{ConnCount: 1},
		{ConnCount: 0},
		{ConnCount: 0},
	}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{
		CapturedAt: time.Now(),
		Interface:  "eth0",
		Groups:     []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}},
	}

	h.navigateNext()
	if h.currentIndex != 0 {
		t.Fatalf("index should stay at last active snapshot, got=%d", h.currentIndex)
	}
	if !strings.Contains(h.statusNote, "Reached last active snapshot") {
		t.Fatalf("expected last-active boundary note, got=%q", h.statusNote)
	}
	if !strings.Contains(h.statusNote, "Hidden empty after: 2") {
		t.Fatalf("expected hidden-after count in boundary note, got=%q", h.statusNote)
	}
}

func TestHistoryNavigatePrevAtFirstActiveShowsBoundaryMessage(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.skipEmpty = true
	h.refs = []history.SnapshotRef{
		{ConnCount: 0},
		{ConnCount: 0},
		{ConnCount: 1},
	}
	h.currentIndex = 2
	h.currentRecord = history.SnapshotRecord{
		CapturedAt: time.Now(),
		Interface:  "eth0",
		Groups:     []history.SnapshotGroup{{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "sshd", ConnCount: 1}},
	}

	h.navigatePrev()
	if h.currentIndex != 2 {
		t.Fatalf("index should stay at first active snapshot, got=%d", h.currentIndex)
	}
	if !strings.Contains(h.statusNote, "Reached first active snapshot") {
		t.Fatalf("expected first-active boundary note, got=%q", h.statusNote)
	}
	if !strings.Contains(h.statusNote, "Hidden empty before: 2") {
		t.Fatalf("expected hidden-before count in boundary note, got=%q", h.statusNote)
	}
}

func TestHistoryAggregateHintLineChangesWithSkipEmpty(t *testing.T) {
	t.Parallel()

	rows := []history.SnapshotGroup{
		{
			PeerIP:    "103.160.74.90",
			LocalPort: 22,
			ProcName:  "sshd",
			ConnCount: 1,
			States:    map[string]int{"ESTABLISHED": 1},
		},
	}
	thresholds := config.DefaultHealthThresholds()

	withSkip := tuipanels.RenderHistoryAggregatePanel(rows, "", "", 20, false, 0, tuishared.SortByBandwidth, true, tuishared.TopConnectionIncoming, true, thresholds, true)
	if !strings.Contains(withSkip, "]=next active snapshot") || !strings.Contains(withSkip, "x=show all snapshots") {
		t.Fatalf("expected active hint line when skip-empty on, got=%q", withSkip)
	}

	withoutSkip := tuipanels.RenderHistoryAggregatePanel(rows, "", "", 20, false, 0, tuishared.SortByBandwidth, true, tuishared.TopConnectionIncoming, false, thresholds, true)
	if !strings.Contains(withoutSkip, "]=next snapshot") || !strings.Contains(withoutSkip, "x=skip empty snapshots") {
		t.Fatalf("expected raw hint line when skip-empty off, got=%q", withoutSkip)
	}
}

func TestHistoryBuildJumpSummaryMentionsMissingDate(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
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

	h := newHistoryTestApp(t.TempDir())
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

	h := newHistoryTestApp(t.TempDir())
	h.refs = []history.SnapshotRef{{}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Groups: []history.SnapshotGroup{
		{PeerIP: "198.51.100.20", LocalPort: 443, ProcName: "sshd", ConnCount: 5, TotalQueue: 10, TotalBytesDelta: 200, States: map[string]int{"ESTABLISHED": 5}},
		{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "nginx", ConnCount: 2, TotalQueue: 50, TotalBytesDelta: 500, States: map[string]int{"CLOSE_WAIT": 2}},
	}}

	h.sortMode = tuishared.SortByBandwidth
	rows := h.visibleRows()
	if len(rows) != 2 || rows[0].PeerIP != "198.51.100.10" {
		t.Fatalf("bandwidth sort should prioritize higher delta bytes, got=%+v", rows)
	}

	h.sortMode = tuishared.SortByConns
	rows = h.visibleRows()
	if len(rows) != 2 || rows[0].PeerIP != "198.51.100.20" {
		t.Fatalf("conns sort should prioritize higher connection count, got=%+v", rows)
	}

	h.sortMode = tuishared.SortByPort
	rows = h.visibleRows()
	if len(rows) != 2 || rows[0].LocalPort != 443 {
		t.Fatalf("port sort (desc default) should prioritize higher local port, got=%+v", rows)
	}
}

func TestHistoryAggregateSortDirectionToggle(t *testing.T) {
	t.Parallel()

	h := newHistoryTestApp(t.TempDir())
	h.refs = []history.SnapshotRef{{}}
	h.currentIndex = 0
	h.currentRecord = history.SnapshotRecord{Groups: []history.SnapshotGroup{
		{PeerIP: "198.51.100.20", LocalPort: 443, ProcName: "sshd", ConnCount: 5, TotalQueue: 10},
		{PeerIP: "198.51.100.10", LocalPort: 22, ProcName: "nginx", ConnCount: 2, TotalQueue: 50},
	}}

	h.sortMode = tuishared.SortByConns
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

	h := newHistoryTestApp(t.TempDir())
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
			got := tuireplay.ClosestSnapshotIndex(h.refs, tc.target)
			if got != tc.want {
				t.Fatalf("closest index mismatch: got=%d want=%d", got, tc.want)
			}
		})
	}
}
