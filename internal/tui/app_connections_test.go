package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/tui/actionlog"
	"github.com/BlackMetalz/holyf-network/internal/tui/diagnosis"
	"github.com/BlackMetalz/holyf-network/internal/tui/livetrace"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/gdamore/tcell/v2"
)

// ---------------------------------------------------------------------------
// Pagination helpers & tests (from app_top_connections_pagination_test.go)
// ---------------------------------------------------------------------------

func incomingPaginationFixtures(count int) ([]collector.Connection, map[int]struct{}) {
	conns := make([]collector.Connection, 0, count)
	listen := make(map[int]struct{}, count)
	for i := 0; i < count; i++ {
		localPort := 10001 + i
		conns = append(conns, collector.Connection{
			LocalIP:    "10.0.0.10",
			LocalPort:  localPort,
			RemoteIP:   fmt.Sprintf("198.51.100.%d", i+1),
			RemotePort: 50000 + i,
			State:      "ESTABLISHED",
			ProcName:   "svc",
			Activity:   int64(100 + i),
		})
		listen[localPort] = struct{}{}
	}
	return conns, listen
}

func outgoingPaginationFixtures(count int) []collector.Connection {
	conns := make([]collector.Connection, 0, count+1)
	// One listener-backed incoming row to ensure OUT mode filtering keeps only dial-out rows.
	conns = append(conns, collector.Connection{
		LocalIP:    "10.0.0.10",
		LocalPort:  18080,
		RemoteIP:   "172.25.110.50",
		RemotePort: 52001,
		State:      "ESTABLISHED",
		ProcName:   "server",
		Activity:   100,
	})
	for i := 0; i < count; i++ {
		conns = append(conns, collector.Connection{
			LocalIP:    "10.0.0.10",
			LocalPort:  40000 + i,
			RemoteIP:   fmt.Sprintf("203.0.113.%d", i+1),
			RemotePort: 4101 + i,
			State:      "ESTABLISHED",
			ProcName:   "client",
			Activity:   int64(50 + i),
		})
	}
	return conns
}

func TestRenderTopConnectionsPanelPaginationIncoming(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.sortMode = tuishared.SortByPort
	a.sortDesc = false
	a.panels[2].SetRect(0, 0, 120, 13) // Row limit becomes 6.

	a.latestTalkers, a.listenPorts = incomingPaginationFixtures(8)
	a.listenPortsKnown = true

	a.renderTopConnectionsPanel()
	page1 := a.panels[2].GetText(true)
	if !strings.Contains(page1, "Page 1/2") {
		t.Fatalf("expected first page footer, got: %q", page1)
	}
	if !strings.Contains(page1, ":10001") || !strings.Contains(page1, ":10006") {
		t.Fatalf("expected first page rows, got: %q", page1)
	}
	if strings.Contains(page1, ":10007") {
		t.Fatalf("did not expect second page rows on page 1, got: %q", page1)
	}

	if !a.moveTopConnectionPage(1) {
		t.Fatalf("expected to move to next page")
	}
	page2 := a.panels[2].GetText(true)
	if !strings.Contains(page2, "Page 2/2") {
		t.Fatalf("expected second page footer, got: %q", page2)
	}
	if !strings.Contains(page2, ":10007") || !strings.Contains(page2, ":10008") {
		t.Fatalf("expected second page rows, got: %q", page2)
	}
}

func TestHandleKeyEventBracketPagingMovesPages(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.sortMode = tuishared.SortByPort
	a.sortDesc = false
	a.panels[2].SetRect(0, 0, 120, 13)
	a.latestTalkers, a.listenPorts = incomingPaginationFixtures(8)
	a.listenPortsKnown = true

	a.renderTopConnectionsPanel()
	if a.topPageIndex != 0 {
		t.Fatalf("expected initial page index 0, got=%d", a.topPageIndex)
	}

	if ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ret != nil {
		t.Fatalf("expected ] to be handled")
	}
	if a.topPageIndex != 1 {
		t.Fatalf("expected page index to advance, got=%d", a.topPageIndex)
	}

	if ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, '[', 0)); ret != nil {
		t.Fatalf("expected [ to be handled")
	}
	if a.topPageIndex != 0 {
		t.Fatalf("expected page index to go back, got=%d", a.topPageIndex)
	}
}

func TestRenderTopConnectionsPanelPaginationOutgoing(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.sortMode = tuishared.SortByPort
	a.sortDesc = false
	a.topDirection = tuishared.TopConnectionOutgoing
	a.listenPortsKnown = true
	a.listenPorts = map[int]struct{}{18080: {}}
	a.latestTalkers = outgoingPaginationFixtures(8)
	a.panels[2].SetRect(0, 0, 120, 13) // Row limit becomes 6.

	a.renderTopConnectionsPanel()
	page1 := a.panels[2].GetText(true)
	if !strings.Contains(page1, "Dir=OUT") || !strings.Contains(page1, "Page 1/2") {
		t.Fatalf("expected OUT mode first page render, got: %q", page1)
	}
	if !strings.Contains(page1, ":4101") || !strings.Contains(page1, ":4106") {
		t.Fatalf("expected outgoing first-page remote ports, got: %q", page1)
	}
	if strings.Contains(page1, ":4107") {
		t.Fatalf("did not expect second-page remote port on page 1, got: %q", page1)
	}

	if !a.moveTopConnectionPage(1) {
		t.Fatalf("expected to move to next page in OUT mode")
	}
	page2 := a.panels[2].GetText(true)
	if !strings.Contains(page2, "Dir=OUT") || !strings.Contains(page2, "Page 2/2") {
		t.Fatalf("expected OUT mode second page render, got: %q", page2)
	}
	if !strings.Contains(page2, ":4107") || !strings.Contains(page2, ":4108") {
		t.Fatalf("expected outgoing second-page remote ports, got: %q", page2)
	}
}

// ---------------------------------------------------------------------------
// Preview helpers & tests (from app_top_connections_preview_test.go)
// ---------------------------------------------------------------------------

func TestCalculateTopConnectionsDisplayLimitWithPreview(t *testing.T) {
	t.Parallel()

	limit, showPreview := tuipanels.CalculateTopConnectionsDisplayLimit(27, 0, true)
	if !showPreview {
		t.Fatalf("expected preview to stay enabled when height is sufficient")
	}
	if limit != 15 {
		t.Fatalf("preview-aware row limit mismatch: got=%d want=%d", limit, 15)
	}
}

func TestCalculateTopConnectionsDisplayLimitFallsBackWithoutPreview(t *testing.T) {
	t.Parallel()

	limit, showPreview := tuipanels.CalculateTopConnectionsDisplayLimit(14, 0, true)
	if showPreview {
		t.Fatalf("expected preview to be disabled when height is too small")
	}
	if limit != 7 {
		t.Fatalf("fallback row limit mismatch: got=%d want=%d", limit, 7)
	}
}

func TestCalculateTopConnectionsDisplayLimitAccountsForBandwidthNote(t *testing.T) {
	t.Parallel()

	limit, showPreview := tuipanels.CalculateTopConnectionsDisplayLimit(27, 1, true)
	if !showPreview {
		t.Fatalf("expected preview to remain enabled with enough height for bandwidth note")
	}
	if limit != 14 {
		t.Fatalf("single-note row limit mismatch: got=%d want=%d", limit, 14)
	}
}

func TestBuildSelectedConnectionPreviewRespectsMasking(t *testing.T) {
	t.Parallel()

	a := &App{
		sensitiveIP:         true,
		selectedTalkerIndex: 0,
		actionLogger:        actionlog.NewLogger(""),
		diagnosisEngine:     diagnosis.NewEngine(),
		traceEngine:         livetrace.NewEngineLoaded(),
		latestTalkers: []collector.Connection{
			{
				LocalIP:       "10.0.0.10",
				LocalPort:     3306,
				RemoteIP:      "198.51.100.20",
				RemotePort:    52001,
				State:         "ESTABLISHED",
				TxQueue:       64,
				RxQueue:       128,
				TxBytesPerSec: 4096,
				RxBytesPerSec: 2048,
				ProcName:      "ct/nat",
			},
		},
	}

	preview := a.buildSelectedConnectionPreview(a.latestTalkers)
	if preview == nil {
		t.Fatalf("expected connection preview")
	}
	if !strings.Contains(preview.Lines[0], "xxx.xxx") {
		t.Fatalf("expected masked endpoints in preview, got: %q", preview.Lines[0])
	}
	if !strings.Contains(preview.Lines[1], "ct/nat") {
		t.Fatalf("expected process info in preview, got: %q", preview.Lines[1])
	}
	if !strings.Contains(preview.Lines[2], "peer xxx.xxx.100.20 -> local 3306 (1 matches; peer+port scope)") {
		t.Fatalf("expected masked action scope in preview, got: %q", preview.Lines[2])
	}
}

func TestBuildSelectedPeerGroupPreviewShowsFullStateAndPorts(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52001, State: "ESTABLISHED", ProcName: "server", Activity: 20},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52002, State: "ESTABLISHED", ProcName: "server", Activity: 10},
		{LocalIP: "10.0.0.10", LocalPort: 9090, RemoteIP: "198.51.100.40", RemotePort: 52003, State: "TIME_WAIT", ProcName: "server", Activity: 5},
		{LocalIP: "10.0.0.10", LocalPort: 9091, RemoteIP: "198.51.100.40", RemotePort: 52004, State: "CLOSE_WAIT", ProcName: "server", Activity: 1},
	}
	a := &App{
		latestTalkers:       conns,
		groupView:           true,
		sortDesc:            true,
		selectedTalkerIndex: 0,
		actionLogger:        actionlog.NewLogger(""),
		diagnosisEngine:     diagnosis.NewEngine(),
		traceEngine:         livetrace.NewEngineLoaded(),
	}

	preview := a.buildSelectedPeerGroupPreview(a.filteredPeerGroups())
	if preview == nil {
		t.Fatalf("expected peer-group preview")
	}
	if !strings.Contains(preview.Lines[1], "EST 50% (2) - TW 25% (1) - CW 25% (1)") {
		t.Fatalf("expected full state summary in preview, got: %q", preview.Lines[1])
	}
	if !strings.Contains(preview.Lines[2], "Ports: 8080,9090,9091 | Action: Enter/k => local 8080 (2 matches)") {
		t.Fatalf("expected full port list and resolved action target, got: %q", preview.Lines[2])
	}
}

func TestRenderTalkersPanelWithPreviewShowsPreviewAndFooter(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{
			LocalIP:       "10.0.0.10",
			LocalPort:     22,
			RemoteIP:      "198.51.100.10",
			RemotePort:    52001,
			State:         "ESTABLISHED",
			ProcName:      "-",
			TxBytesPerSec: 512,
			RxBytesPerSec: 256,
		},
	}
	preview := &tuipanels.SelectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			"Local: 10.0.0.10:22 -> Peer: 198.51.100.10:52001 | State: ESTABLISHED",
			"Proc: - | Queue: send 0B recv 0B | BW: tx 512B/s rx 256B/s",
			"Action: Enter/k => peer 198.51.100.10 -> local 22 (1 matches; peer+port scope)",
		},
	}

	text := tuipanels.RenderTalkersPanelWithPreview(conns, "", "", 20, false, 0, tuishared.SortByBandwidth, true, config.DefaultHealthThresholds(), "", 120, preview)
	if !strings.Contains(text, "Selected Detail") {
		t.Fatalf("expected selected detail header, got: %q", text)
	}
	if !strings.Contains(text, "Proc: - | Queue: send 0B recv 0B") {
		t.Fatalf("expected preview body, got: %q", text)
	}
	if !strings.Contains(text, "Showing 1 of 1 connections") {
		t.Fatalf("expected footer to remain visible, got: %q", text)
	}
}

func TestRenderPeerGroupPanelWithPreviewShowsPreviewAndFooter(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 80, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED", ProcName: "nginx"},
		{LocalIP: "10.0.0.10", LocalPort: 81, RemoteIP: "198.51.100.10", RemotePort: 52002, State: "TIME_WAIT", ProcName: "nginx"},
	}
	preview := &tuipanels.SelectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			"Peer: 198.51.100.10 | Proc: nginx | Conns: 2",
			"States: EST 50% (1) - TW 50% (1)",
			"Ports: 80,81 | Action: Enter/k => local 80 (1 matches)",
		},
	}

	text := tuipanels.RenderPeerGroupPanelWithPreview(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "", 120, preview)
	if !strings.Contains(text, "States: EST 50% (1) - TW 50% (1)") {
		t.Fatalf("expected preview states in rendered group panel, got: %q", text)
	}
	if !strings.Contains(text, "1 groups, 1 peers, 2 total connections") {
		t.Fatalf("expected grouped footer to remain visible, got: %q", text)
	}
}

func TestRenderTalkersPanelWithBandwidthNoteShowsBandwidthNote(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 443, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED", ProcName: "api"},
	}

	text := tuipanels.RenderTalkersPanelWithPreview(
		conns,
		"",
		"",
		20,
		false,
		0,
		tuishared.SortByBandwidth,
		true,
		config.DefaultHealthThresholds(),
		"Bandwidth baseline is warming up.",
		120,
		nil,
	)
	if !strings.Contains(text, "Bandwidth baseline is warming up.") {
		t.Fatalf("expected bandwidth note, got: %q", text)
	}
	if strings.Contains(text, "Diagnosis:") {
		t.Fatalf("did not expect diagnosis note inside top panel, got: %q", text)
	}
}

func TestRenderTopConnectionsPanelKeepsPreviewWhenHeightAllows(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 80, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED", ProcName: "nginx", Activity: 100},
	}
	a.topBandwidthNote = "Bandwidth baseline is warming up."
	a.panels[2].SetRect(0, 0, 120, 27)

	a.renderTopConnectionsPanel()
	text := a.panels[2].GetText(true)
	if strings.Contains(text, "Diagnosis:") {
		t.Fatalf("did not expect diagnosis line in top panel, got: %q", text)
	}
	if !strings.Contains(text, "Selected Detail") {
		t.Fatalf("expected preview to remain visible with bandwidth note, got: %q", text)
	}
}

func TestRenderTopConnectionsPanelDropsPreviewBeforeBandwidthNote(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 80, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED", ProcName: "nginx", Activity: 100},
	}
	a.topBandwidthNote = "Bandwidth baseline is warming up."
	a.panels[2].SetRect(0, 0, 120, 13)

	a.renderTopConnectionsPanel()
	text := a.panels[2].GetText(true)
	if !strings.Contains(text, "Bandwidth baseline is warming up.") {
		t.Fatalf("expected bandwidth note, got: %q", text)
	}
	if strings.Contains(text, "Selected Detail") {
		t.Fatalf("expected preview to be hidden before notes, got: %q", text)
	}
}

func TestRenderTopConnectionsPanelUpdatesPreviewOnSelectionAndGroupToggle(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.sortDesc = true
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 80, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED", ProcName: "nginx", Activity: 100},
		{LocalIP: "10.0.0.10", LocalPort: 81, RemoteIP: "198.51.100.20", RemotePort: 52002, State: "ESTABLISHED", ProcName: "api", Activity: 50},
	}
	a.panels[2].SetRect(0, 0, 120, 27)

	a.renderTopConnectionsPanel()
	first := a.panels[2].GetText(true)
	if strings.Contains(first, "Diagnosis:") {
		t.Fatalf("did not expect diagnosis in top panel render, got: %q", first)
	}
	if !strings.Contains(first, "peer 198.51.100.10 -> local 80") {
		t.Fatalf("expected preview for first selected row, got: %q", first)
	}

	if !a.moveTopConnectionSelection(1) {
		t.Fatalf("expected selection move to succeed")
	}
	second := a.panels[2].GetText(true)
	if !strings.Contains(second, "peer 198.51.100.20 -> local 81") {
		t.Fatalf("expected preview to update for second row, got: %q", second)
	}

	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	if ret != nil {
		t.Fatalf("g should be handled")
	}
	groupText := a.panels[2].GetText(true)
	if !strings.Contains(groupText, "Ports: 80 | Action: Enter/k => local 80 (1 matches)") &&
		!strings.Contains(groupText, "Ports: 81 | Action: Enter/k => local 81 (1 matches)") {
		t.Fatalf("expected group preview action line after toggle, got: %q", groupText)
	}
}

func TestTopConnectionsSourceHidesCurrentProcessTraffic(t *testing.T) {
	t.Parallel()

	selfPID := os.Getpid()
	a := newPhase3TestApp()
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 18080, RemoteIP: "172.25.110.137", RemotePort: 52001, State: "TIME_WAIT", ProcName: "-", PID: 2001},
		{LocalIP: "10.0.0.10", LocalPort: 52246, RemoteIP: "20.205.243.168", RemotePort: 443, State: "ESTABLISHED", ProcName: "holyf-network", PID: selfPID},
	}

	source := a.topConnectionsSource()
	if len(source) != 1 {
		t.Fatalf("expected self traffic to be filtered, got=%d rows", len(source))
	}
	if source[0].RemoteIP != "172.25.110.137" {
		t.Fatalf("unexpected remaining row after self filter: %+v", source[0])
	}
}

func TestRenderTopConnectionsPanelHidesCurrentProcessTraffic(t *testing.T) {
	t.Parallel()

	selfPID := os.Getpid()
	a := newPhase3TestApp()
	a.groupView = true
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 18080, RemoteIP: "172.25.110.137", RemotePort: 52001, State: "TIME_WAIT", ProcName: "-", PID: 2001},
		{LocalIP: "10.0.0.10", LocalPort: 52246, RemoteIP: "20.205.243.168", RemotePort: 443, State: "ESTABLISHED", ProcName: "holyf-network", PID: selfPID},
	}
	a.panels[2].SetRect(0, 0, 120, 27)

	a.renderTopConnectionsPanel()
	text := a.panels[2].GetText(true)
	if strings.Contains(text, "20.205.243.168") || strings.Contains(text, "holyf-network") {
		t.Fatalf("expected current process traffic to stay hidden from Top Connections, got: %q", text)
	}
	if !strings.Contains(text, "172.25.110.137") {
		t.Fatalf("expected non-self row to remain visible, got: %q", text)
	}
}

func TestTopConnectionsSourceRespectsIncomingOutgoingDirection(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 18080, RemoteIP: "172.25.110.137", RemotePort: 52001, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 52246, RemoteIP: "20.205.243.168", RemotePort: 443, State: "ESTABLISHED", ProcName: "client"},
	}
	a.listenPorts = map[int]struct{}{18080: {}}
	a.listenPortsKnown = true

	incoming := a.topConnectionsSource()
	if len(incoming) != 1 || incoming[0].LocalPort != 18080 {
		t.Fatalf("expected IN mode to keep listener-backed row, got: %+v", incoming)
	}

	a.topDirection = tuishared.TopConnectionOutgoing
	outgoing := a.topConnectionsSource()
	if len(outgoing) != 1 || outgoing[0].RemotePort != 443 || outgoing[0].LocalPort != 52246 {
		t.Fatalf("expected OUT mode to keep non-listener row, got: %+v", outgoing)
	}
}

func TestBuildSelectedPeerGroupPreviewOutgoingShowsRemotePortsAndDisabledAction(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 52246, RemoteIP: "20.205.243.168", RemotePort: 443, State: "ESTABLISHED", ProcName: "client", Activity: 20},
		{LocalIP: "10.0.0.10", LocalPort: 52247, RemoteIP: "20.205.243.168", RemotePort: 8443, State: "ESTABLISHED", ProcName: "client", Activity: 10},
	}
	a := &App{
		latestTalkers:       conns,
		groupView:           true,
		topDirection:        tuishared.TopConnectionOutgoing,
		sortDesc:            true,
		selectedTalkerIndex: 0,
		actionLogger:        actionlog.NewLogger(""),
		diagnosisEngine:     diagnosis.NewEngine(),
		traceEngine:         livetrace.NewEngineLoaded(),
	}

	preview := a.buildSelectedPeerGroupPreview(a.filteredPeerGroups())
	if preview == nil {
		t.Fatalf("expected outgoing peer-group preview")
	}
	if !strings.Contains(preview.Lines[2], "RPorts: 443,8443 | Action: Enter/k disabled in OUT mode") {
		t.Fatalf("expected remote ports and disabled action in OUT preview, got: %q", preview.Lines[2])
	}
}

func TestRenderPeerGroupPanelOutgoingUsesRemotePortHeader(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 52246, RemoteIP: "20.205.243.168", RemotePort: 443, State: "ESTABLISHED", ProcName: "client"},
	}
	preview := &tuipanels.SelectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			"Peer: 20.205.243.168 | Proc: client | Conns: 1",
			"States: EST 100% (1)",
			"RPorts: 443 | Action: Enter/k disabled in OUT mode",
		},
	}

	text := tuipanels.RenderPeerGroupPanelWithPreviewDirection(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "", 120, preview, 0, tuishared.TopConnectionOutgoing)
	if !strings.Contains(text, "RPORTS") {
		t.Fatalf("expected outgoing group header to use RPORTS, got: %q", text)
	}
	if !strings.Contains(text, "Dir=OUT") {
		t.Fatalf("expected outgoing panel chips to show Dir=OUT, got: %q", text)
	}
}

// ---------------------------------------------------------------------------
// Filter helpers & tests (from panel_top_connections_filters_test.go)
// ---------------------------------------------------------------------------

func topConnectionFixtures() []collector.Connection {
	return []collector.Connection{
		{
			LocalIP:    "10.0.0.10",
			LocalPort:  80,
			RemoteIP:   "198.51.100.10",
			RemotePort: 52001,
			State:      "ESTABLISHED",
			Activity:   900,
			PID:        101,
			ProcName:   "nginx",
		},
		{
			LocalIP:    "10.0.0.10",
			LocalPort:  81,
			RemoteIP:   "198.51.100.10",
			RemotePort: 52002,
			State:      "ESTABLISHED",
			Activity:   300,
			PID:        101,
			ProcName:   "nginx",
		},
		{
			LocalIP:    "10.0.0.10",
			LocalPort:  443,
			RemoteIP:   "198.51.100.20",
			RemotePort: 52003,
			State:      "ESTABLISHED",
			Activity:   700,
			PID:        202,
			ProcName:   "api",
		},
		{
			LocalIP:    "10.0.0.10",
			LocalPort:  5555,
			RemoteIP:   "198.51.100.30",
			RemotePort: 443,
			State:      "ESTABLISHED",
			Activity:   600,
			PID:        303,
			ProcName:   "proxy",
		},
	}
}

func TestRenderTalkersPanelPortFilterChipText(t *testing.T) {
	t.Parallel()

	conns := topConnectionFixtures()

	allText := tuipanels.RenderTalkersPanel(conns, "", "", 20, false, 0, tuishared.SortByBandwidth, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(allText, "Port Filter = ALL") {
		t.Fatalf("default chip should render Port Filter = ALL, got: %q", allText)
	}

	selectedText := tuipanels.RenderTalkersPanel(conns, "443", "", 20, false, 0, tuishared.SortByBandwidth, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(selectedText, "Port Filter = 443") {
		t.Fatalf("selected chip should render Port Filter = 443, got: %q", selectedText)
	}
}

func TestRenderPeerGroupPanelPortFilterChipText(t *testing.T) {
	t.Parallel()

	conns := topConnectionFixtures()

	allText := tuipanels.RenderPeerGroupPanel(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(allText, "Port Filter = ALL") {
		t.Fatalf("group chip should render Port Filter = ALL, got: %q", allText)
	}

	selectedText := tuipanels.RenderPeerGroupPanel(conns, "443", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(selectedText, "Port Filter = 443") {
		t.Fatalf("group chip should render Port Filter = 443, got: %q", selectedText)
	}
}

func TestVisiblePeerGroupsPortFilterAffectsGroupedResultsAndClearingRestoresAll(t *testing.T) {
	t.Parallel()

	a := &App{
		latestTalkers: topConnectionFixtures(),
		portFilter:    "443",
		actionLogger:  actionlog.NewLogger(""),
		traceEngine:   livetrace.NewEngineLoaded(),
	}

	filtered := a.visiblePeerGroups()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 peer group for local port 443, got %d", len(filtered))
	}

	gotFilteredPeers := map[string]struct{}{}
	for _, g := range filtered {
		gotFilteredPeers[g.PeerIP] = struct{}{}
		if _, ok := g.LocalPorts[443]; !ok {
			t.Fatalf("filtered group should only include local port 443, got ports=%v", g.LocalPorts)
		}
	}
	if _, ok := gotFilteredPeers["198.51.100.20"]; !ok {
		t.Fatalf("filtered groups should include peer 198.51.100.20 (local port 443), got: %#v", gotFilteredPeers)
	}
	if _, ok := gotFilteredPeers["198.51.100.30"]; ok {
		t.Fatalf("filtered groups should exclude peer 198.51.100.30 (remote-only 443), got: %#v", gotFilteredPeers)
	}
	if _, ok := gotFilteredPeers["198.51.100.10"]; ok {
		t.Fatalf("filtered groups should exclude peer 198.51.100.10 when filter=443, got: %#v", gotFilteredPeers)
	}

	a.portFilter = ""
	restored := a.visiblePeerGroups()
	if len(restored) != 3 {
		t.Fatalf("expected 3 peer groups after clearing filter, got %d", len(restored))
	}
}

func TestFormatProcessInfoSupportsConntrackSyntheticRow(t *testing.T) {
	t.Parallel()

	conn := collector.Connection{
		PID:      0,
		ProcName: "ct/nat",
	}
	if got := tuipanels.FormatProcessInfo(conn); got != "ct/nat" {
		t.Fatalf("expected synthetic process label ct/nat, got=%q", got)
	}
}

func TestRenderTalkersPanelShowsSyntheticProcessWhenNoPID(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{
			LocalIP:    "172.25.110.76",
			LocalPort:  3306,
			RemoteIP:   "172.25.110.116",
			RemotePort: 43286,
			State:      "ESTABLISHED",
			ProcName:   "ct/nat",
		},
	}
	text := tuipanels.RenderTalkersPanel(conns, "", "", 20, false, 0, tuishared.SortByBandwidth, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(text, "ct/nat") {
		t.Fatalf("expected synthetic process label to render, got: %q", text)
	}
}

func TestRenderPeerGroupPanelSplitsSamePeerByProcess(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{
			LocalIP:    "172.25.110.76",
			LocalPort:  22,
			RemoteIP:   "172.25.110.116",
			RemotePort: 52754,
			State:      "ESTABLISHED",
			ProcName:   "sshd",
		},
		{
			LocalIP:    "172.25.110.76",
			LocalPort:  22,
			RemoteIP:   "172.25.110.116",
			RemotePort: 33974,
			State:      "ESTABLISHED",
			ProcName:   "sshd",
		},
		{
			LocalIP:    "172.25.110.76",
			LocalPort:  3306,
			RemoteIP:   "172.25.110.116",
			RemotePort: 48062,
			State:      "ESTABLISHED",
			ProcName:   "ct/nat",
		},
	}

	text := tuipanels.RenderPeerGroupPanel(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(text, "sshd") || !strings.Contains(text, "ct/nat") {
		t.Fatalf("expected both sshd and ct/nat groups, got: %q", text)
	}
	if !strings.Contains(text, "2 groups, 1 peers, 3 total connections") {
		t.Fatalf("expected grouped footer for split-process view, got: %q", text)
	}
}

func TestFormatStatePercentDoesNotRoundPartialShareUpToHundred(t *testing.T) {
	t.Parallel()

	if got := tuipanels.FormatStatePercent(10584, 10585); got != "99%" {
		t.Fatalf("expected near-total share to clamp below 100%%, got: %q", got)
	}
	if got := tuipanels.FormatStatePercent(109, 109); got != "100%" {
		t.Fatalf("expected exact total share to remain 100%%, got: %q", got)
	}
}

func TestLimitPeerGroupsKeepsTopTwentyByCountEvenWhenDisplayIsAscending(t *testing.T) {
	t.Parallel()

	conns := make([]collector.Connection, 0, 400)
	for count := 1; count <= 25; count++ {
		peer := "198.51.100." + strconv.Itoa(count)
		for n := 0; n < count; n++ {
			conns = append(conns, collector.Connection{
				LocalIP:    "10.0.0.10",
				LocalPort:  8080,
				RemoteIP:   peer,
				RemotePort: 50000 + n,
				State:      "ESTABLISHED",
				ProcName:   "api",
			})
		}
	}

	groups := tuipanels.BuildPeerGroups(conns, true)
	limited := tuipanels.LimitPeerGroups(groups, tuipanels.TopConnectionsGroupCap, false)
	if len(limited) != tuipanels.TopConnectionsGroupCap {
		t.Fatalf("expected capped group count=%d, got=%d", tuipanels.TopConnectionsGroupCap, len(limited))
	}
	if limited[0].Count != 6 {
		t.Fatalf("expected ascending view to start at lowest count inside top-20 set, got=%d", limited[0].Count)
	}
	if limited[len(limited)-1].Count != 25 {
		t.Fatalf("expected top-20 set to retain highest count, got tail=%d", limited[len(limited)-1].Count)
	}
}

func TestRenderPeerGroupPanelShowsShownOverTotalWhenCapped(t *testing.T) {
	t.Parallel()

	conns := make([]collector.Connection, 0, 400)
	for count := 1; count <= 25; count++ {
		peer := "198.51.100." + strconv.Itoa(count)
		for n := 0; n < count; n++ {
			conns = append(conns, collector.Connection{
				LocalIP:    "10.0.0.10",
				LocalPort:  8080,
				RemoteIP:   peer,
				RemotePort: 50000 + n,
				State:      "ESTABLISHED",
				ProcName:   "api",
			})
		}
	}

	text := tuipanels.RenderPeerGroupPanel(conns, "", "", 30, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(text, "20 shown / 25 groups") {
		t.Fatalf("expected capped footer to mention shown/total groups, got: %q", text)
	}
}
