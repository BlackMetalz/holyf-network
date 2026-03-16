package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/gdamore/tcell/v2"
)

func TestCalculateTopConnectionsDisplayLimitWithPreview(t *testing.T) {
	t.Parallel()

	limit, showPreview := calculateTopConnectionsDisplayLimit(27, false, true)
	if !showPreview {
		t.Fatalf("expected preview to stay enabled when height is sufficient")
	}
	if limit != 15 {
		t.Fatalf("preview-aware row limit mismatch: got=%d want=%d", limit, 15)
	}
}

func TestCalculateTopConnectionsDisplayLimitFallsBackWithoutPreview(t *testing.T) {
	t.Parallel()

	limit, showPreview := calculateTopConnectionsDisplayLimit(14, false, true)
	if showPreview {
		t.Fatalf("expected preview to be disabled when height is too small")
	}
	if limit != 7 {
		t.Fatalf("fallback row limit mismatch: got=%d want=%d", limit, 7)
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
