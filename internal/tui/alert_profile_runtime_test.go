package tui

import (
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

func TestActiveHealthThresholdsUseProfileConntrackBand(t *testing.T) {
	t.Parallel()

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
	}
	a.applyAlertProfile(config.AlertProfileDB)

	active := a.activeHealthThresholds()
	if active.ConntrackPercent.Warn != 55 || active.ConntrackPercent.Crit != 70 {
		t.Fatalf("conntrack thresholds mismatch for DB profile: got=%+v", active.ConntrackPercent)
	}
	if active.RetransPercent.Warn != a.healthThresholds.RetransPercent.Warn {
		t.Fatalf("non-conntrack thresholds should remain unchanged")
	}
}

func TestApplyAlertProfileResetsInterfaceSpikeBaseline(t *testing.T) {
	t.Parallel()

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
		ifaceSpikeEMA:    1234,
		ifaceSpikeCount:  9,
	}
	a.applyAlertProfile(config.AlertProfileCache)
	if a.ifaceSpikeEMA != 0 || a.ifaceSpikeCount != 0 {
		t.Fatalf("expected interface spike baseline reset, got ema=%v count=%d", a.ifaceSpikeEMA, a.ifaceSpikeCount)
	}
	if a.currentAlertProfileSpec().Name != config.AlertProfileCache {
		t.Fatalf("expected profile CACHE, got=%q", a.currentAlertProfileSpec().Name)
	}
}

func TestEvaluateInterfaceSpikeFlagsRatioSpike(t *testing.T) {
	t.Parallel()

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
	}
	a.applyAlertProfile(config.AlertProfileDB)

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

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
	}
	a.applyAlertProfile(config.AlertProfileDB)

	linkSpeedBps := 1_000_000_000.0 / 8.0 // 1Gb/s link
	_ = a.evaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 40 * 1024 * 1024,
		TxBytesPerSec: 10 * 1024 * 1024,
	}, linkSpeedBps, true)
	got := a.evaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 95 * 1024 * 1024,
		TxBytesPerSec: 30 * 1024 * 1024,
	}, linkSpeedBps, true)
	if got.Level != healthCrit {
		t.Fatalf("expected link-util crit for db profile near saturation, got level=%v util=%.2f%%", got.Level, got.LinkUtilPercent)
	}
}
