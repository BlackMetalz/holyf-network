package tui

import (
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

func TestActiveHealthThresholdsKeepsConfiguredConntrackBand(t *testing.T) {
	t.Parallel()

	thresholds := config.DefaultHealthThresholds()
	thresholds.ConntrackPercent = config.ThresholdBand{Warn: 62, Crit: 88}
	a := &App{healthThresholds: thresholds}

	active := a.activeHealthThresholds()
	if active.ConntrackPercent.Warn != 62 || active.ConntrackPercent.Crit != 88 {
		t.Fatalf("conntrack thresholds mismatch: got=%+v", active.ConntrackPercent)
	}
}

func TestEvaluateInterfaceSpikeFlagsRatioSpike(t *testing.T) {
	t.Parallel()

	a := &App{healthThresholds: config.DefaultHealthThresholds()}

	_ = a.evaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 10 * 1024 * 1024,
		TxBytesPerSec: 9 * 1024 * 1024,
	}, 0, false)
	got := a.evaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 80 * 1024 * 1024,
		TxBytesPerSec: 70 * 1024 * 1024,
	}, 0, false)
	if got.Level != healthCrit {
		t.Fatalf("expected ratio-based crit spike, got level=%v ratio=%.2f baseline=%.1f peak=%.1f", got.Level, got.Ratio, got.BaselineBytesPerSec, got.PeakBytesPerSec)
	}
}

func TestEvaluateInterfaceSpikeUsesLinkUtilWhenSpeedKnown(t *testing.T) {
	t.Parallel()

	a := &App{healthThresholds: config.DefaultHealthThresholds()}

	linkSpeedBps := 1_000_000_000.0 / 8.0 // 1Gb/s link
	_ = a.evaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 40 * 1024 * 1024,
		TxBytesPerSec: 10 * 1024 * 1024,
	}, linkSpeedBps, true)
	got := a.evaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 110 * 1024 * 1024,
		TxBytesPerSec: 30 * 1024 * 1024,
	}, linkSpeedBps, true)
	if got.Level != healthCrit {
		t.Fatalf("expected link-util crit near saturation, got level=%v util=%.2f%%", got.Level, got.LinkUtilPercent)
	}
}
