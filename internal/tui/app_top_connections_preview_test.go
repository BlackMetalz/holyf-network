package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/gdamore/tcell/v2"
)

func TestCalculateTopConnectionsDisplayLimitWithPreview(t *testing.T) {
	t.Parallel()

	limit, showPreview := calculateTopConnectionsDisplayLimit(27, 0, true)
	if !showPreview {
		t.Fatalf("expected preview to stay enabled when height is sufficient")
	}
	if limit != 15 {
		t.Fatalf("preview-aware row limit mismatch: got=%d want=%d", limit, 15)
	}
}

func TestCalculateTopConnectionsDisplayLimitFallsBackWithoutPreview(t *testing.T) {
	t.Parallel()

	limit, showPreview := calculateTopConnectionsDisplayLimit(14, 0, true)
	if showPreview {
		t.Fatalf("expected preview to be disabled when height is too small")
	}
	if limit != 7 {
		t.Fatalf("fallback row limit mismatch: got=%d want=%d", limit, 7)
	}
}

func TestCalculateTopConnectionsDisplayLimitAccountsForBandwidthNote(t *testing.T) {
	t.Parallel()

	limit, showPreview := calculateTopConnectionsDisplayLimit(27, 1, true)
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
	preview := &selectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			"Local: 10.0.0.10:22 -> Peer: 198.51.100.10:52001 | State: ESTABLISHED",
			"Proc: - | Queue: send 0B recv 0B | BW: tx 512B/s rx 256B/s",
			"Action: Enter/k => peer 198.51.100.10 -> local 22 (1 matches; peer+port scope)",
		},
	}

	text := renderTalkersPanelWithPreview(conns, "", "", 20, false, 0, SortByBandwidth, true, config.DefaultHealthThresholds(), "", 120, preview)
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
	preview := &selectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			"Peer: 198.51.100.10 | Proc: nginx | Conns: 2",
			"States: EST 50% (1) - TW 50% (1)",
			"Ports: 80,81 | Action: Enter/k => local 80 (1 matches)",
		},
	}

	text := renderPeerGroupPanelWithPreview(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "", 120, preview)
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

	text := renderTalkersPanelWithPreview(
		conns,
		"",
		"",
		20,
		false,
		0,
		SortByBandwidth,
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

	a.topDirection = topConnectionOutgoing
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
		topDirection:        topConnectionOutgoing,
		sortDesc:            true,
		selectedTalkerIndex: 0,
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
	preview := &selectedRowPreview{
		Title: "Selected Detail",
		Lines: []string{
			"Peer: 20.205.243.168 | Proc: client | Conns: 1",
			"States: EST 100% (1)",
			"RPorts: 443 | Action: Enter/k disabled in OUT mode",
		},
	}

	text := renderPeerGroupPanelWithPreviewDirection(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "", 120, preview, 0, topConnectionOutgoing)
	if !strings.Contains(text, "RPORTS") {
		t.Fatalf("expected outgoing group header to use RPORTS, got: %q", text)
	}
	if !strings.Contains(text, "Dir=OUT") {
		t.Fatalf("expected outgoing panel chips to show Dir=OUT, got: %q", text)
	}
}
