package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/BlackMetalz/holyf-network/internal/tui/actionlog"
	"github.com/BlackMetalz/holyf-network/internal/tui/livetrace"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

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
