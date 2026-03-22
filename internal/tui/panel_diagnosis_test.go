package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/gdamore/tcell/v2"
)

func TestCreatePanelsIncludesDiagnosisPanel(t *testing.T) {
	t.Parallel()

	panels := createPanels()
	if len(panels) != 5 {
		t.Fatalf("panel count mismatch: got=%d want=%d", len(panels), 5)
	}

	titles := []string{
		" 2. Connection States ",
		" 3. Interface Stats ",
		" 1. Top Incoming ",
		" 4. Conntrack ",
		" 5. Diagnosis ",
	}
	for i, want := range titles {
		if got := panels[i].GetTitle(); got != want {
			t.Fatalf("panel[%d] title mismatch: got=%q want=%q", i, got, want)
		}
	}
}

func TestRenderDiagnosisPanelShowsV2DecisionCard(t *testing.T) {
	t.Parallel()

	text := stripTviewTags(renderDiagnosisPanel(&topDiagnosis{
		Severity:   healthWarn,
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

	raw := renderDiagnosisPanel(&topDiagnosis{
		Severity:   healthWarn,
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

	text := renderDiagnosisPanel(nil, 56)
	if !strings.Contains(text, "Waiting for live diagnosis data") {
		t.Fatalf("expected placeholder text, got: %q", text)
	}
}

func TestRenderDiagnosisPanelIgnoresBogusStartupWidth(t *testing.T) {
	t.Parallel()

	text := stripTviewTags(renderDiagnosisPanel(&topDiagnosis{
		Severity: healthOK,
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

func TestDiagnosisPanelRemainsHostGlobalAcrossGroupToggle(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.healthThresholds = config.DefaultHealthThresholds()
	a.latestTalkers = []collector.Connection{
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "TIME_WAIT", ProcName: "api", Activity: 100},
		{LocalIP: "10.0.0.10", LocalPort: 8080, RemoteIP: "198.51.100.10", RemotePort: 52002, State: "TIME_WAIT", ProcName: "api", Activity: 90},
	}
	a.topDiagnosis = &topDiagnosis{
		Severity: healthWarn,
		Issue:    "TIME_WAIT churn",
		Scope:    "198.51.100.10 :8080",
		Signal:   "TW 2 | Retr LOW SAMPLE | CT 4%",
		Likely:   "short-lived conn churn, not packet loss",
		Check:    "keepalive, conn reuse, client retries",
	}

	a.renderDiagnosisPanel()
	before := a.panels[4].GetText(true)
	ret := a.handleKeyEvent(tcellKeyRune('g'))
	if ret != nil {
		t.Fatalf("g should be handled")
	}
	after := a.panels[4].GetText(true)
	if before != after {
		t.Fatalf("expected diagnosis panel to remain unchanged across group toggle:\nbefore=%q\nafter=%q", before, after)
	}
}

func tcellKeyRune(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, 0)
}
