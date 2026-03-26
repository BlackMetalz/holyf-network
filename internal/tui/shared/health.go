package shared

import (
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

type HealthLevel int

const (
	HealthUnknown HealthLevel = iota
	HealthOK
	HealthWarn
	HealthCrit
)

type RetransSampleStatus struct {
	Ready            bool
	Established      int
	MinEstablished   int
	OutSegsPerSec    float64
	MinOutSegsPerSec float64
}

func EvaluateRetransSample(
	data collector.ConnectionData,
	retrans *collector.RetransmitRates,
	thresholds config.HealthThresholds,
) RetransSampleStatus {
	established := data.States["ESTABLISHED"]
	status := RetransSampleStatus{
		Ready:            false,
		Established:      established,
		MinEstablished:   thresholds.RetransMinEstablished,
		OutSegsPerSec:    0,
		MinOutSegsPerSec: thresholds.RetransMinOutSegsPerSec,
	}
	if retrans == nil {
		return status
	}
	status.OutSegsPerSec = retrans.OutSegsPerSec
	status.Ready = established >= thresholds.RetransMinEstablished &&
		retrans.OutSegsPerSec >= thresholds.RetransMinOutSegsPerSec
	return status
}

func ClassifyMetric(value float64, threshold config.ThresholdBand) HealthLevel {
	if value >= threshold.Crit {
		return HealthCrit
	}
	if value >= threshold.Warn {
		return HealthWarn
	}
	return HealthOK
}

func ColorForHealthLevel(level HealthLevel) string {
	switch level {
	case HealthCrit:
		return "red"
	case HealthWarn:
		return "yellow"
	case HealthOK:
		return "green"
	default:
		return "dim"
	}
}

func MaxHealthLevel(a, b HealthLevel) HealthLevel {
	if a > b {
		return a
	}
	return b
}
