package tui

import (
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
		" 1. Top Connections ",
		" 4. Conntrack ",
		" 5. Diagnosis ",
	}
	for i, want := range titles {
		if got := panels[i].GetTitle(); got != want {
			t.Fatalf("panel[%d] title mismatch: got=%q want=%q", i, got, want)
		}
	}
}

func TestRenderDiagnosisPanelShowsSummaryEvidenceAndNextChecks(t *testing.T) {
	t.Parallel()

	text := renderDiagnosisPanel(&topDiagnosis{
		Severity: healthWarn,
		Headline: "TCP retrans is high",
		Reason:   "Retrans is 6.20% with enough traffic sample.",
		Evidence: []string{
			"Retrans: 6.20% at 3.4 retrans/s.",
			"Sample ready: 133 ESTABLISHED, 190.0 out seg/s.",
		},
		NextChecks: []string{
			"Check NIC errors/drops and inspect ss -tin for retrans behavior.",
			"Validate path loss, RTT spikes, or congestion.",
		},
	}, 56)

	if !strings.Contains(text, "Summary: TCP retrans is high") {
		t.Fatalf("expected summary line, got: %q", text)
	}
	if !strings.Contains(text, "Why: Retrans is 6.20%") {
		t.Fatalf("expected why line, got: %q", text)
	}
	if !strings.Contains(text, "Evidence") || !strings.Contains(text, "Next Checks") {
		t.Fatalf("expected section headers, got: %q", text)
	}
	if !strings.Contains(text, "Retrans: 6.20% at 3.4 retrans/s.") {
		t.Fatalf("expected evidence body, got: %q", text)
	}
	if !strings.Contains(text, "Check NIC errors/drops") {
		t.Fatalf("expected next-check body, got: %q", text)
	}
}

func TestRenderDiagnosisPanelShowsPlaceholderWhenNil(t *testing.T) {
	t.Parallel()

	text := renderDiagnosisPanel(nil, 56)
	if !strings.Contains(text, "Waiting for live diagnosis data") {
		t.Fatalf("expected placeholder text, got: %q", text)
	}
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
		Headline: "TIME_WAIT churn on :8080 from 198.51.100.10",
		Reason:   "Short-lived connections are dominating more than a current path-quality issue.",
		Evidence: []string{
			"State count: 2 TIME_WAIT sockets (warn > 1).",
			"Culprit: 198.51.100.10 on :8080 via api (2 sockets).",
		},
		NextChecks: []string{
			"Check whether one service is creating short-lived connections faster than expected.",
		},
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
