package tui

import (
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

const (
	interfaceSpikeWarmupSamples    = 3
	interfaceSpikeEMAAlpha         = 0.2
	interfaceTrafficWarnSamples    = 2
	interfaceTrafficClearSamples   = 3
	interfaceTrafficCritDownsample = 2
	interfaceUtilWarnPercent       = 60.0
	interfaceUtilCritPercent       = 85.0
	interfaceBytesWarnPerSec       = 80.0 * 1024 * 1024
	interfaceBytesCritPerSec       = 200.0 * 1024 * 1024
	interfaceSpikeRatioWarn        = 2.0
	interfaceSpikeRatioCrit        = 3.0
	interfaceSpikeBaselineFloorBps = 5.0 * 1024 * 1024
)

type interfaceSpikeAssessment struct {
	Level               healthLevel
	DisplayLevel        healthLevel
	PeakBytesPerSec     float64
	BaselineBytesPerSec float64
	Ratio               float64
	LinkSpeedBps        float64
	LinkUtilPercent     float64
	LinkSpeedKnown      bool
	Ready               bool
}

func (a *App) activeHealthThresholds() config.HealthThresholds {
	thresholds := a.healthThresholds
	thresholds.Normalize()
	return thresholds
}

func (a *App) evaluateInterfaceSpike(rates collector.InterfaceRates, linkSpeedBps float64, linkSpeedKnown bool) interfaceSpikeAssessment {
	peak := max(rates.RxBytesPerSec, rates.TxBytesPerSec)
	assessment := interfaceSpikeAssessment{
		Level:           healthUnknown,
		DisplayLevel:    healthUnknown,
		PeakBytesPerSec: peak,
		LinkSpeedBps:    linkSpeedBps,
		LinkSpeedKnown:  linkSpeedKnown && linkSpeedBps > 0,
	}

	if rates.FirstReading || peak <= 0 {
		return assessment
	}

	utilThreshold := config.ThresholdBand{Warn: interfaceUtilWarnPercent, Crit: interfaceUtilCritPercent}
	fallbackThreshold := config.ThresholdBand{Warn: interfaceBytesWarnPerSec, Crit: interfaceBytesCritPerSec}
	spikeRatioThreshold := config.ThresholdBand{Warn: interfaceSpikeRatioWarn, Crit: interfaceSpikeRatioCrit}

	if a.ifaceSpikeCount == 0 || a.ifaceSpikeEMA <= 0 {
		a.ifaceSpikeEMA = peak
		a.ifaceSpikeCount = 1
		assessment.BaselineBytesPerSec = peak
		assessment.Ratio = 1.0
		if assessment.LinkSpeedKnown {
			assessment.LinkUtilPercent = (peak / assessment.LinkSpeedBps) * 100.0
		}
		assessment.Level = interfaceAbsoluteLevel(peak, assessment.LinkSpeedBps, assessment.LinkSpeedKnown, utilThreshold, fallbackThreshold)
		assessment.DisplayLevel = a.stabilizeInterfaceTrafficLevel(assessment.Level, false)
		return assessment
	}

	baseline := a.ifaceSpikeEMA
	assessment.BaselineBytesPerSec = baseline
	denom := max(1.0, max(baseline, interfaceSpikeBaselineFloorBps))
	assessment.Ratio = peak / denom
	absLevel := interfaceAbsoluteLevel(peak, assessment.LinkSpeedBps, assessment.LinkSpeedKnown, utilThreshold, fallbackThreshold)
	if assessment.LinkSpeedKnown {
		assessment.LinkUtilPercent = (peak / assessment.LinkSpeedBps) * 100.0
	}
	ratioLevel := classifyMetric(assessment.Ratio, spikeRatioThreshold)
	assessment.Level = maxHealthLevel(absLevel, ratioLevel)
	assessment.Ready = a.ifaceSpikeCount >= interfaceSpikeWarmupSamples
	assessment.DisplayLevel = a.stabilizeInterfaceTrafficLevel(assessment.Level, assessment.Ready)

	emaInput := peak
	if baseline > 0 && spikeRatioThreshold.Crit > 1 {
		emaInput = min(emaInput, baseline*spikeRatioThreshold.Crit)
	}
	a.ifaceSpikeEMA = baseline*(1-interfaceSpikeEMAAlpha) + emaInput*interfaceSpikeEMAAlpha
	a.ifaceSpikeCount++

	return assessment
}

func (a *App) stabilizeInterfaceTrafficLevel(raw healthLevel, ready bool) healthLevel {
	if !ready {
		a.ifaceTrafficDisplayLevel = healthUnknown
		a.ifaceTrafficWarnStreak = 0
		a.ifaceTrafficClearStreak = 0
		return healthUnknown
	}

	switch raw {
	case healthCrit:
		a.ifaceTrafficDisplayLevel = healthCrit
		a.ifaceTrafficWarnStreak = 0
		a.ifaceTrafficClearStreak = 0
		return a.ifaceTrafficDisplayLevel
	case healthWarn:
		a.ifaceTrafficClearStreak = 0
		a.ifaceTrafficWarnStreak++
		if a.ifaceTrafficDisplayLevel == healthCrit {
			if a.ifaceTrafficWarnStreak >= interfaceTrafficCritDownsample {
				a.ifaceTrafficDisplayLevel = healthWarn
				a.ifaceTrafficWarnStreak = 0
			}
			return a.ifaceTrafficDisplayLevel
		}
		if a.ifaceTrafficDisplayLevel == healthWarn || a.ifaceTrafficWarnStreak >= interfaceTrafficWarnSamples {
			a.ifaceTrafficDisplayLevel = healthWarn
			a.ifaceTrafficWarnStreak = 0
			return a.ifaceTrafficDisplayLevel
		}
		if a.ifaceTrafficDisplayLevel == healthUnknown {
			a.ifaceTrafficDisplayLevel = healthOK
		}
		return a.ifaceTrafficDisplayLevel
	default:
		a.ifaceTrafficWarnStreak = 0
		if a.ifaceTrafficDisplayLevel == healthUnknown {
			a.ifaceTrafficDisplayLevel = healthOK
			return a.ifaceTrafficDisplayLevel
		}
		a.ifaceTrafficClearStreak++
		if a.ifaceTrafficDisplayLevel == healthCrit {
			if a.ifaceTrafficClearStreak >= interfaceTrafficCritDownsample {
				a.ifaceTrafficDisplayLevel = healthWarn
				a.ifaceTrafficClearStreak = 0
			}
			return a.ifaceTrafficDisplayLevel
		}
		if a.ifaceTrafficClearStreak >= interfaceTrafficClearSamples {
			a.ifaceTrafficDisplayLevel = healthOK
			a.ifaceTrafficClearStreak = 0
		}
		return a.ifaceTrafficDisplayLevel
	}
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
