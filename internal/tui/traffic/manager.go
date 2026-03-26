package traffic

import (
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
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

type Manager struct {
	healthThresholds         config.HealthThresholds
	ifaceSpikeEMA            float64
	ifaceSpikeCount          int
	ifaceTrafficDisplayLevel tuishared.HealthLevel
	ifaceTrafficWarnStreak   int
	ifaceTrafficClearStreak  int

	ifaceSpeedMbps   float64
	ifaceSpeedKnown  bool
	ifaceSpeedSample bool
}

func NewManager(healthThresholds config.HealthThresholds) *Manager {
	thresholds := healthThresholds
	thresholds.Normalize()
	return &Manager{
		healthThresholds:         thresholds,
		ifaceTrafficDisplayLevel: tuishared.HealthUnknown,
	}
}

func (m *Manager) SetSpeed(mbps float64, known bool) {
	m.ifaceSpeedSample = true
	m.ifaceSpeedKnown = known
	if known {
		m.ifaceSpeedMbps = mbps
	} else {
		m.ifaceSpeedMbps = 0
	}
}

func (m *Manager) IfaceSpeedSample() bool  { return m.ifaceSpeedSample }
func (m *Manager) IfaceSpeedKnown() bool   { return m.ifaceSpeedKnown }
func (m *Manager) IfaceSpeedMbps() float64 { return m.ifaceSpeedMbps }

func (m *Manager) ActiveHealthThresholds() config.HealthThresholds {
	return m.healthThresholds
}

func (m *Manager) EvaluateInterfaceSpike(rates collector.InterfaceRates, linkSpeedBps float64, linkSpeedKnown bool) tuishared.InterfaceSpikeAssessment {
	peak := max(rates.RxBytesPerSec, rates.TxBytesPerSec)
	assessment := tuishared.InterfaceSpikeAssessment{
		Level:           tuishared.HealthUnknown,
		DisplayLevel:    tuishared.HealthUnknown,
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

	if m.ifaceSpikeCount == 0 || m.ifaceSpikeEMA <= 0 {
		m.ifaceSpikeEMA = peak
		m.ifaceSpikeCount = 1
		assessment.BaselineBytesPerSec = peak
		assessment.Ratio = 1.0
		if assessment.LinkSpeedKnown {
			assessment.LinkUtilPercent = (peak / assessment.LinkSpeedBps) * 100.0
		}
		assessment.Level = interfaceAbsoluteLevel(peak, assessment.LinkSpeedBps, assessment.LinkSpeedKnown, utilThreshold, fallbackThreshold)
		assessment.DisplayLevel = m.stabilizeInterfaceTrafficLevel(assessment.Level, false)
		return assessment
	}

	baseline := m.ifaceSpikeEMA
	assessment.BaselineBytesPerSec = baseline
	denom := max(1.0, max(baseline, interfaceSpikeBaselineFloorBps))
	assessment.Ratio = peak / denom
	absLevel := interfaceAbsoluteLevel(peak, assessment.LinkSpeedBps, assessment.LinkSpeedKnown, utilThreshold, fallbackThreshold)
	if assessment.LinkSpeedKnown {
		assessment.LinkUtilPercent = (peak / assessment.LinkSpeedBps) * 100.0
	}
	ratioLevel := tuishared.ClassifyMetric(assessment.Ratio, spikeRatioThreshold)
	assessment.Level = tuishared.MaxHealthLevel(absLevel, ratioLevel)
	assessment.Ready = m.ifaceSpikeCount >= interfaceSpikeWarmupSamples
	assessment.DisplayLevel = m.stabilizeInterfaceTrafficLevel(assessment.Level, assessment.Ready)

	emaInput := peak
	if baseline > 0 && spikeRatioThreshold.Crit > 1 {
		emaInput = min(emaInput, baseline*spikeRatioThreshold.Crit)
	}
	m.ifaceSpikeEMA = baseline*(1-interfaceSpikeEMAAlpha) + emaInput*interfaceSpikeEMAAlpha
	m.ifaceSpikeCount++

	return assessment
}

func (m *Manager) stabilizeInterfaceTrafficLevel(raw tuishared.HealthLevel, ready bool) tuishared.HealthLevel {
	if !ready {
		m.ifaceTrafficDisplayLevel = tuishared.HealthUnknown
		m.ifaceTrafficWarnStreak = 0
		m.ifaceTrafficClearStreak = 0
		return tuishared.HealthUnknown
	}

	switch raw {
	case tuishared.HealthCrit:
		m.ifaceTrafficDisplayLevel = tuishared.HealthCrit
		m.ifaceTrafficWarnStreak = 0
		m.ifaceTrafficClearStreak = 0
		return m.ifaceTrafficDisplayLevel
	case tuishared.HealthWarn:
		m.ifaceTrafficClearStreak = 0
		m.ifaceTrafficWarnStreak++
		if m.ifaceTrafficDisplayLevel == tuishared.HealthCrit {
			if m.ifaceTrafficWarnStreak >= interfaceTrafficCritDownsample {
				m.ifaceTrafficDisplayLevel = tuishared.HealthWarn
				m.ifaceTrafficWarnStreak = 0
			}
			return m.ifaceTrafficDisplayLevel
		}
		if m.ifaceTrafficDisplayLevel == tuishared.HealthWarn || m.ifaceTrafficWarnStreak >= interfaceTrafficWarnSamples {
			m.ifaceTrafficDisplayLevel = tuishared.HealthWarn
			m.ifaceTrafficWarnStreak = 0
			return m.ifaceTrafficDisplayLevel
		}
		if m.ifaceTrafficDisplayLevel == tuishared.HealthUnknown {
			m.ifaceTrafficDisplayLevel = tuishared.HealthOK
		}
		return m.ifaceTrafficDisplayLevel
	default:
		m.ifaceTrafficWarnStreak = 0
		if m.ifaceTrafficDisplayLevel == tuishared.HealthUnknown {
			m.ifaceTrafficDisplayLevel = tuishared.HealthOK
			return m.ifaceTrafficDisplayLevel
		}
		m.ifaceTrafficClearStreak++
		if m.ifaceTrafficDisplayLevel == tuishared.HealthCrit {
			if m.ifaceTrafficClearStreak >= interfaceTrafficCritDownsample {
				m.ifaceTrafficDisplayLevel = tuishared.HealthWarn
				m.ifaceTrafficClearStreak = 0
			}
			return m.ifaceTrafficDisplayLevel
		}
		if m.ifaceTrafficClearStreak >= interfaceTrafficClearSamples {
			m.ifaceTrafficDisplayLevel = tuishared.HealthOK
			m.ifaceTrafficClearStreak = 0
		}
		return m.ifaceTrafficDisplayLevel
	}
}

func interfaceAbsoluteLevel(
	peakBytesPerSec float64,
	linkSpeedBps float64,
	linkSpeedKnown bool,
	utilThreshold config.ThresholdBand,
	fallbackThreshold config.ThresholdBand,
) tuishared.HealthLevel {
	if linkSpeedKnown && linkSpeedBps > 0 {
		utilPercent := (peakBytesPerSec / linkSpeedBps) * 100.0
		return tuishared.ClassifyMetric(utilPercent, utilThreshold)
	}
	return tuishared.ClassifyMetric(peakBytesPerSec, fallbackThreshold)
}
