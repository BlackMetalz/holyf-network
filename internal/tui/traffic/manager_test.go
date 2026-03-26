package traffic

import (
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

func TestActiveHealthThresholdsKeepsConfiguredConntrackBand(t *testing.T) {
	t.Parallel()

	thresholds := config.DefaultHealthThresholds()
	thresholds.ConntrackPercent = config.ThresholdBand{Warn: 62, Crit: 88}
	m := NewManager(thresholds)

	active := m.ActiveHealthThresholds()
	if active.ConntrackPercent.Warn != 62 || active.ConntrackPercent.Crit != 88 {
		t.Fatalf("conntrack thresholds mismatch: got=%+v", active.ConntrackPercent)
	}
}

func TestEvaluateInterfaceSpikeFlagsRatioSpike(t *testing.T) {
	t.Parallel()

	m := NewManager(config.DefaultHealthThresholds())

	_ = m.EvaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 10 * 1024 * 1024,
		TxBytesPerSec: 9 * 1024 * 1024,
	}, 0, false)
	got := m.EvaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 80 * 1024 * 1024,
		TxBytesPerSec: 70 * 1024 * 1024,
	}, 0, false)
	if got.Level != tuishared.HealthCrit {
		t.Fatalf("expected ratio-based crit spike, got level=%v ratio=%.2f baseline=%.1f peak=%.1f", got.Level, got.Ratio, got.BaselineBytesPerSec, got.PeakBytesPerSec)
	}
}

func TestEvaluateInterfaceSpikeUsesLinkUtilWhenSpeedKnown(t *testing.T) {
	t.Parallel()

	m := NewManager(config.DefaultHealthThresholds())

	linkSpeedBps := 1_000_000_000.0 / 8.0 // 1Gb/s link
	_ = m.EvaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 40 * 1024 * 1024,
		TxBytesPerSec: 10 * 1024 * 1024,
	}, linkSpeedBps, true)
	got := m.EvaluateInterfaceSpike(collector.InterfaceRates{
		RxBytesPerSec: 110 * 1024 * 1024,
		TxBytesPerSec: 30 * 1024 * 1024,
	}, linkSpeedBps, true)
	if got.Level != tuishared.HealthCrit {
		t.Fatalf("expected link-util crit near saturation, got level=%v util=%.2f%%", got.Level, got.LinkUtilPercent)
	}
}
