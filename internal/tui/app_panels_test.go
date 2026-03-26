package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuilayout "github.com/BlackMetalz/holyf-network/internal/tui/layout"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/gdamore/tcell/v2"
)

// --- conntrack tests ---

func TestRenderConntrackPanelHidesZeroDropsWhenStatsAvailable(t *testing.T) {
	t.Parallel()

	rendered := tuipanels.RenderConntrackPanel(collector.ConntrackRates{
		Current:        2,
		Max:            262144,
		UsagePercent:   0.0007,
		StatsAvailable: true,
		FirstReading:   false,
		TotalDrops:     0,
	}, config.ThresholdBand{Warn: 70, Crit: 85})

	if strings.Contains(rendered, "Drops: 0") {
		t.Fatalf("expected zero drops to be hidden, got: %q", rendered)
	}
	if strings.Contains(rendered, "Profile:") {
		t.Fatalf("expected profile footer to be removed, got: %q", rendered)
	}
	if !strings.Contains(rendered, "<0.1%") {
		t.Fatalf("expected tiny usage to render as <0.1%%, got: %q", rendered)
	}
}

func TestRenderConntrackPanelShowsDropCountWhenNonZero(t *testing.T) {
	t.Parallel()

	rendered := tuipanels.RenderConntrackPanel(collector.ConntrackRates{
		Current:        128,
		Max:            262144,
		UsagePercent:   0.05,
		StatsAvailable: true,
		FirstReading:   false,
		TotalDrops:     12,
	}, config.ThresholdBand{Warn: 70, Crit: 85})

	if !strings.Contains(rendered, "Drops: 12") {
		t.Fatalf("expected non-zero drops to remain visible, got: %q", rendered)
	}
}

// --- connections health tests ---

func TestRenderHealthStripSkipsRetransSeverityWhenLowSample(t *testing.T) {
	t.Parallel()

	data := collector.ConnectionData{
		States: map[string]int{
			"ESTABLISHED": 7,
		},
		Total: 7,
	}
	retrans := &collector.RetransmitRates{
		RetransPerSec:  2.4,
		OutSegsPerSec:  12,
		RetransPercent: 9.2,
	}

	rendered := tuipanels.RenderHealthStrip(data, retrans, nil, config.DefaultHealthThresholds())

	if !strings.Contains(rendered, "LOW SAMPLE") {
		t.Fatalf("expected LOW SAMPLE in health strip, got: %q", rendered)
	}
	if strings.Contains(rendered, "HEALTH CRIT") {
		t.Fatalf("low sample should not escalate retrans severity, got: %q", rendered)
	}
}

func TestRenderHealthStripUsesRetransSeverityWhenSampleReady(t *testing.T) {
	t.Parallel()

	data := collector.ConnectionData{
		States: map[string]int{
			"ESTABLISHED": 40,
		},
		Total: 40,
	}
	retrans := &collector.RetransmitRates{
		RetransPerSec:  8.7,
		OutSegsPerSec:  200,
		RetransPercent: 6.1,
	}

	rendered := tuipanels.RenderHealthStrip(data, retrans, nil, config.DefaultHealthThresholds())

	if !strings.Contains(rendered, "HEALTH CRIT") {
		t.Fatalf("expected HEALTH CRIT for high retrans with enough sample, got: %q", rendered)
	}
	if !strings.Contains(rendered, "6.1%") {
		t.Fatalf("expected retrans percentage in health strip, got: %q", rendered)
	}
}

func TestRenderConnectionsPanelShowsLowSampleDetails(t *testing.T) {
	t.Parallel()

	data := collector.ConnectionData{
		States: map[string]int{
			"ESTABLISHED": 7,
			"LISTEN":      3,
		},
		Total: 10,
	}
	retrans := &collector.RetransmitRates{
		RetransPerSec:  2.0,
		OutSegsPerSec:  20,
		RetransPercent: 7.0,
	}

	rendered := tuipanels.RenderConnectionsPanel(data, retrans, nil, config.DefaultHealthThresholds())

	if !strings.Contains(rendered, "LOW SAMPLE") {
		t.Fatalf("expected low-sample message in retrans panel, got: %q", rendered)
	}
	if strings.Contains(rendered, "⚠ high loss!") {
		t.Fatalf("low-sample mode must not show high-loss alert, got: %q", rendered)
	}
}

func TestRenderConnectionsPanelWithStateSortToggleDirection(t *testing.T) {
	t.Parallel()

	data := collector.ConnectionData{
		States: map[string]int{
			"ESTABLISHED": 9,
			"TIME_WAIT":   3,
			"SYN_RECV":    1,
		},
		Total: 13,
	}

	desc := tuipanels.RenderConnectionsPanelWithStateSort(data, nil, nil, config.DefaultHealthThresholds(), true)
	asc := tuipanels.RenderConnectionsPanelWithStateSort(data, nil, nil, config.DefaultHealthThresholds(), false)

	if !(strings.Index(desc, "ESTABLISHED") < strings.Index(desc, "TIME_WAIT")) {
		t.Fatalf("desc sort should place ESTABLISHED before TIME_WAIT, got: %q", desc)
	}
	if !(strings.Index(asc, "SYN_RECV") < strings.Index(asc, "TIME_WAIT")) {
		t.Fatalf("asc sort should place SYN_RECV before TIME_WAIT, got: %q", asc)
	}
}

// --- diagnosis tests ---

func TestCreatePanelsIncludesDiagnosisPanel(t *testing.T) {
	t.Parallel()

	panels := tuilayout.CreatePanels()
	if len(panels) != 3 {
		t.Fatalf("panel count mismatch: got=%d want=%d", len(panels), 3)
	}

	titles := []string{
		" 2. System Health ",
		" 3. Bandwidth ",
		" 1. Top Incoming ",
	}
	for i, want := range titles {
		if got := panels[i].GetTitle(); got != want {
			t.Fatalf("panel[%d] title mismatch: got=%q want=%q", i, got, want)
		}
	}
}

func TestRenderDiagnosisPanelShowsV2DecisionCard(t *testing.T) {
	t.Parallel()

	text := stripTviewTags(tuipanels.RenderDiagnosisPanel(&tuishared.Diagnosis{
		Severity:   tuishared.HealthWarn,
		Confidence: "MEDIUM",
		Issue:      "TCP retrans high",
		Scope:      "host-wide",
		Signal:     "Retr 6.20% | Out 190.0/s | EST 133",
		Likely:     "packet loss, RTT spikes, NIC errors, or congestion",
		Evidence: []string{
			"Retrans: 6.20% at 190.0 retrans/s.",
			"Sample ready: 133 ESTABLISHED, 190.0 out seg/s.",
		},
		NextChecks: []string{
			"Check NIC errors/drops and inspect ss -tin for per-socket retrans behavior.",
			"Validate path loss, RTT spikes, or congestion between local host and peer path.",
		},
	}, 92))

	if !strings.Contains(text, "Issue: TCP retrans high") {
		t.Fatalf("expected issue line, got: %q", text)
	}
	if !strings.Contains(text, "Scope: host-wide") {
		t.Fatalf("expected scope line, got: %q", text)
	}
	if !strings.Contains(text, "Signal: Retrans: 6.20% | Out seg/s: 190.0/s | ESTABLISHED: 133") {
		t.Fatalf("expected signal line, got: %q", text)
	}
	if !strings.Contains(text, "Likely Cause: packet loss, RTT spikes") {
		t.Fatalf("expected likely-cause line, got: %q", text)
	}
	if !strings.Contains(text, "Confidence: MEDIUM") {
		t.Fatalf("expected confidence line, got: %q", text)
	}
	if !strings.Contains(text, "Why: Retrans: 6.20% at 190.0 retrans/s.") {
		t.Fatalf("expected why line, got: %q", text)
	}
	if !strings.Contains(text, "Next Actions:\n") {
		t.Fatalf("expected next-actions label line, got: %q", text)
	}
	if !regexp.MustCompile(`\n\s+1\)\s+Check NIC errors/drops`).MatchString(text) {
		t.Fatalf("expected first action on its own line, got: %q", text)
	}
	if !regexp.MustCompile(`\n\s+2\)\s+Validate path loss`).MatchString(text) {
		t.Fatalf("expected second action on its own line, got: %q", text)
	}
}

func TestRenderDiagnosisPanelShowsConciseStateIssue(t *testing.T) {
	t.Parallel()

	raw := tuipanels.RenderDiagnosisPanel(&tuishared.Diagnosis{
		Severity:   tuishared.HealthWarn,
		Confidence: "MEDIUM",
		Issue:      "TIME_WAIT churn",
		Scope:      "172.25.110.137 :18080",
		Signal:     "TW 4,974 | Retr LOW SAMPLE | CT 2%",
		Likely:     "short-lived conn churn, not packet loss",
		Evidence: []string{
			"State count: 4,974 TIME_WAIT sockets (warn > 600).",
		},
		NextChecks: []string{
			"Check whether one service is creating short-lived connections faster than expected.",
		},
	}, 56)
	text := stripTviewTags(raw)

	if !strings.Contains(text, "Issue: TIME_WAIT churn") {
		t.Fatalf("expected issue line, got: %q", text)
	}
	if !strings.Contains(text, "Scope: 172.25.110.137 :18080") {
		t.Fatalf("expected scope line, got: %q", text)
	}
	if !strings.Contains(text, "Signal: TIME_WAIT: 4,974 | Retrans: LOW SAMPLE |") || !strings.Contains(text, "Conntrack: 2%") {
		t.Fatalf("expected concise signal line, got: %q", text)
	}
	if !strings.Contains(text, "Likely Cause: short-lived conn churn, not packet loss") {
		t.Fatalf("expected likely-cause line, got: %q", text)
	}
	if !strings.Contains(text, "Confidence: MEDIUM") {
		t.Fatalf("expected confidence line, got: %q", text)
	}
	if !strings.Contains(text, "Why: State count: 4,974 TIME_WAIT sockets") {
		t.Fatalf("expected why line, got: %q", text)
	}
	if !strings.Contains(text, "Next Actions:\n") {
		t.Fatalf("expected next-actions label line, got: %q", text)
	}
	if !regexp.MustCompile(`\n\s+1\)\s+Check whether one service is creating`).MatchString(text) {
		t.Fatalf("expected first action on its own line, got: %q", text)
	}
	if !strings.Contains(raw, "[dim]Scope: [white][dim]") {
		t.Fatalf("expected scope value to be dimmed, got: %q", raw)
	}
}

func TestRenderDiagnosisPanelShowsPlaceholderWhenNil(t *testing.T) {
	t.Parallel()

	text := tuipanels.RenderDiagnosisPanel(nil, 56)
	if !strings.Contains(text, "Waiting for live diagnosis data") {
		t.Fatalf("expected placeholder text, got: %q", text)
	}
}

func TestRenderDiagnosisPanelIgnoresBogusStartupWidth(t *testing.T) {
	t.Parallel()

	text := stripTviewTags(tuipanels.RenderDiagnosisPanel(&tuishared.Diagnosis{
		Severity: tuishared.HealthOK,
		Issue:    "No dominant issue",
		Scope:    "host-wide",
		Signal:   "Retr LOW SAMPLE | CT 0% | States stable",
		Likely:   "no warning-level signal is dominating right now",
		Check:    "watch Top/States, wait next sample",
	}, 10))

	if strings.Contains(text, "Issue: No\n") {
		t.Fatalf("expected startup width fallback to avoid one-word wrapping, got: %q", text)
	}
	if !strings.Contains(text, "Issue: No dominant issue") {
		t.Fatalf("expected sane compact issue line with fallback width, got: %q", text)
	}
}

var tviewTagPattern = regexp.MustCompile(`\[[^\]]*\]`)

func stripTviewTags(s string) string {
	return tviewTagPattern.ReplaceAllString(s, "")
}

func TestBandwidthPanelRemainsUnchangedAcrossGroupToggle(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.healthThresholds = config.DefaultHealthThresholds()
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "TIME_WAIT", ProcName: "api", Activity: 100},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.10", RemotePort: 52002, State: "TIME_WAIT", ProcName: "api", Activity: 90},
	}

	a.panels[1].SetText("  Collecting samples...")
	before := a.panels[1].GetText(true)
	ret := a.handleKeyEvent(tcellKeyRune('g'))
	if ret != nil {
		t.Fatalf("g should be handled")
	}
	after := a.panels[1].GetText(true)
	if before != after {
		t.Fatalf("expected bandwidth panel to remain unchanged across group toggle:\nbefore=%q\nafter=%q", before, after)
	}
}

func tcellKeyRune(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, 0)
}
