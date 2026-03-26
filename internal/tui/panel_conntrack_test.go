package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
)

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
