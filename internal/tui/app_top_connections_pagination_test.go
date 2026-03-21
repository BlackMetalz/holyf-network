package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
)

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
	a.sortMode = SortByPort
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
	a.sortMode = SortByPort
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
	a.sortMode = SortByPort
	a.sortDesc = false
	a.topDirection = topConnectionOutgoing
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
