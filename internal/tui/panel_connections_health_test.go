package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
)

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
