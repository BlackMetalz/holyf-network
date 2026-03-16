package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

func TestRenderConntrackPanelHidesZeroDropsWhenStatsAvailable(t *testing.T) {
	t.Parallel()

	rendered := renderConntrackPanel(collector.ConntrackRates{
		Current:        2,
		Max:            262144,
		UsagePercent:   0.0007,
		StatsAvailable: true,
		FirstReading:   false,
		TotalDrops:     0,
	})

	if strings.Contains(rendered, "Drops: 0") {
		t.Fatalf("expected zero drops to be hidden, got: %q", rendered)
	}
}

func TestRenderConntrackPanelShowsDropCountWhenNonZero(t *testing.T) {
	t.Parallel()

	rendered := renderConntrackPanel(collector.ConntrackRates{
		Current:        128,
		Max:            262144,
		UsagePercent:   0.05,
		StatsAvailable: true,
		FirstReading:   false,
		TotalDrops:     12,
	})

	if !strings.Contains(rendered, "Drops: 12") {
		t.Fatalf("expected non-zero drops to remain visible, got: %q", rendered)
	}
}
