package tui

import (
	"fmt"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

const (
	interfaceSpikeWarmupSamples = 3
	interfaceSpikeEMAAlpha      = 0.2
)

type interfaceSpikeAssessment struct {
	ProfileLabel        string
	Level               healthLevel
	PeakBytesPerSec     float64
	BaselineBytesPerSec float64
	Ratio               float64
	LinkSpeedBps        float64
	LinkUtilPercent     float64
	LinkSpeedKnown      bool
	Ready               bool
}

func (a *App) currentAlertProfileSpec() config.AlertProfileSpec {
	if a.alertProfileSpec.Name == "" {
		spec := config.AlertProfileSpecFor(config.DefaultAlertProfile())
		a.alertProfile = spec.Name
		a.alertProfileSpec = spec
	}
	return a.alertProfileSpec
}

func (a *App) activeHealthThresholds() config.HealthThresholds {
	thresholds := a.healthThresholds
	thresholds.Normalize()
	spec := a.currentAlertProfileSpec()
	thresholds.ConntrackPercent = spec.Thresholds.ConntrackPercent
	thresholds.Normalize()
	return thresholds
}

func (a *App) cycleAlertProfile() {
	next := config.NextAlertProfile(a.currentAlertProfileSpec().Name)
	a.applyAlertProfile(next)
	spec := a.currentAlertProfileSpec()
	a.setStatusNote(
		fmt.Sprintf("Alert profile: %s (conntrack %.0f/%.0f%%, Shift+Y=guide)", spec.Label, spec.Thresholds.ConntrackPercent.Warn, spec.Thresholds.ConntrackPercent.Crit),
		5*time.Second,
	)
	a.refreshData()
}

func (a *App) applyAlertProfile(profile config.AlertProfile) {
	spec := config.AlertProfileSpecFor(profile)
	a.alertProfile = spec.Name
	a.alertProfileSpec = spec
	a.ifaceSpikeEMA = 0
	a.ifaceSpikeCount = 0
}

func (a *App) evaluateInterfaceSpike(rates collector.InterfaceRates, linkSpeedBps float64, linkSpeedKnown bool) interfaceSpikeAssessment {
	spec := a.currentAlertProfileSpec()
	peak := max(rates.RxBytesPerSec, rates.TxBytesPerSec)
	assessment := interfaceSpikeAssessment{
		ProfileLabel:    spec.Label,
		Level:           healthUnknown,
		PeakBytesPerSec: peak,
		LinkSpeedBps:    linkSpeedBps,
		LinkSpeedKnown:  linkSpeedKnown && linkSpeedBps > 0,
	}

	if rates.FirstReading || peak <= 0 {
		return assessment
	}

	if a.ifaceSpikeCount == 0 || a.ifaceSpikeEMA <= 0 {
		a.ifaceSpikeEMA = peak
		a.ifaceSpikeCount = 1
		assessment.BaselineBytesPerSec = peak
		assessment.Ratio = 1.0
		if assessment.LinkSpeedKnown {
			assessment.LinkUtilPercent = (peak / assessment.LinkSpeedBps) * 100.0
		}
		assessment.Level = interfaceAbsoluteLevel(peak, assessment.LinkSpeedBps, assessment.LinkSpeedKnown, spec.Thresholds.InterfaceUtilPercent, spec.Thresholds.InterfaceBytesPerSec)
		return assessment
	}

	baseline := a.ifaceSpikeEMA
	assessment.BaselineBytesPerSec = baseline
	denom := max(1.0, max(baseline, spec.Thresholds.InterfaceBaselineFloorPerSec))
	assessment.Ratio = peak / denom
	absLevel := interfaceAbsoluteLevel(peak, assessment.LinkSpeedBps, assessment.LinkSpeedKnown, spec.Thresholds.InterfaceUtilPercent, spec.Thresholds.InterfaceBytesPerSec)
	if assessment.LinkSpeedKnown {
		assessment.LinkUtilPercent = (peak / assessment.LinkSpeedBps) * 100.0
	}
	ratioLevel := classifyMetric(assessment.Ratio, spec.Thresholds.InterfaceSpikeRatio)
	assessment.Level = maxHealthLevel(absLevel, ratioLevel)
	assessment.Ready = a.ifaceSpikeCount >= interfaceSpikeWarmupSamples

	emaInput := peak
	if baseline > 0 && spec.Thresholds.InterfaceSpikeRatio.Crit > 1 {
		emaInput = min(emaInput, baseline*spec.Thresholds.InterfaceSpikeRatio.Crit)
	}
	a.ifaceSpikeEMA = baseline*(1-interfaceSpikeEMAAlpha) + emaInput*interfaceSpikeEMAAlpha
	a.ifaceSpikeCount++

	return assessment
}

func interfaceAbsoluteLevel(
	peakBytesPerSec float64,
	linkSpeedBps float64,
	linkSpeedKnown bool,
	utilThreshold config.ThresholdBand,
	fallbackThreshold config.ThresholdBand,
) healthLevel {
	if linkSpeedKnown && linkSpeedBps > 0 {
		utilPercent := (peakBytesPerSec / linkSpeedBps) * 100.0
		return classifyMetric(utilPercent, utilThreshold)
	}
	return classifyMetric(peakBytesPerSec, fallbackThreshold)
}
