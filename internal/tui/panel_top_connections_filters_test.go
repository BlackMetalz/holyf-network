package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
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

	allText := renderTalkersPanel(conns, "", "", 20, false, 0, SortByBandwidth, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(allText, "Port Filter = ALL") {
		t.Fatalf("default chip should render Port Filter = ALL, got: %q", allText)
	}

	selectedText := renderTalkersPanel(conns, "443", "", 20, false, 0, SortByBandwidth, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(selectedText, "Port Filter = 443") {
		t.Fatalf("selected chip should render Port Filter = 443, got: %q", selectedText)
	}
}

func TestRenderPeerGroupPanelPortFilterChipText(t *testing.T) {
	t.Parallel()

	conns := topConnectionFixtures()

	allText := renderPeerGroupPanel(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(allText, "Port Filter = ALL") {
		t.Fatalf("group chip should render Port Filter = ALL, got: %q", allText)
	}

	selectedText := renderPeerGroupPanel(conns, "443", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(selectedText, "Port Filter = 443") {
		t.Fatalf("group chip should render Port Filter = 443, got: %q", selectedText)
	}
	if !strings.Contains(allText, "STATE %") {
		t.Fatalf("group panel should render STATE %% column, got: %q", allText)
	}
}

func TestVisiblePeerGroupsPortFilterAffectsGroupedResultsAndClearingRestoresAll(t *testing.T) {
	t.Parallel()

	a := &App{
		latestTalkers: topConnectionFixtures(),
		portFilter:    "443",
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
	if got := formatProcessInfo(conn); got != "ct/nat" {
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
	text := renderTalkersPanel(conns, "", "", 20, false, 0, SortByBandwidth, true, config.DefaultHealthThresholds(), "")
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

	text := renderPeerGroupPanel(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(text, "sshd") || !strings.Contains(text, "ct/nat") {
		t.Fatalf("expected both sshd and ct/nat groups, got: %q", text)
	}
	if !strings.Contains(text, "2 groups, 1 peers, 3 total connections") {
		t.Fatalf("expected grouped footer for split-process view, got: %q", text)
	}
}

func TestRenderPeerGroupPanelShowsSingleStatePercent(t *testing.T) {
	t.Parallel()

	text := renderPeerGroupPanel(topConnectionFixtures(), "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(text, "EST 100%") {
		t.Fatalf("expected grouped panel to show 100%% state share, got: %q", text)
	}
}

func TestRenderPeerGroupPanelShowsTopThreeStatePercents(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52001, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52002, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52003, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52004, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52005, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52006, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52007, State: "ESTABLISHED", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52008, State: "TIME_WAIT", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52009, State: "CLOSE_WAIT", ProcName: "server"},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.40", RemotePort: 52010, State: "SYN_SENT", ProcName: "server"},
	}

	text := renderPeerGroupPanel(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(text, "EST 70% - TW 10% - CW 10%") {
		t.Fatalf("expected top three state shares in grouped panel, got: %q", text)
	}
	if strings.Contains(text, "SS 10%") {
		t.Fatalf("expected grouped panel to limit state summary to top three entries, got: %q", text)
	}
}

func TestRenderPeerGroupPanelShowsSmallStatePercentAsLessThanOne(t *testing.T) {
	t.Parallel()

	conns := make([]collector.Connection, 0, 102)
	for i := 0; i < 100; i++ {
		conns = append(conns, collector.Connection{
			LocalIP:    "10.0.0.10",
			LocalPort:  8080,
			RemoteIP:   "198.51.100.50",
			RemotePort: 52000 + i,
			State:      "TIME_WAIT",
			ProcName:   "server",
		})
	}
	conns = append(conns,
		collector.Connection{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.50", RemotePort: 52101, State: "ESTABLISHED", ProcName: "server"},
		collector.Connection{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.50", RemotePort: 52102, State: "SYN_SENT", ProcName: "server"},
	)

	text := renderPeerGroupPanel(conns, "", "", 20, false, 0, true, config.DefaultHealthThresholds(), "")
	if !strings.Contains(text, "TW 98% - EST <1% - SS <1%") {
		t.Fatalf("expected grouped panel to preserve tiny non-zero states, got: %q", text)
	}
	if strings.Contains(text, "EST 0%") || strings.Contains(text, "SS 0%") {
		t.Fatalf("expected grouped panel to avoid rounding tiny states down to 0%%, got: %q", text)
	}
}
