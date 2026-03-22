package config

import "strings"

// AlertProfile is a workload-aware alert threshold profile.
type AlertProfile string

const (
	AlertProfileWeb   AlertProfile = "web"
	AlertProfileDB    AlertProfile = "db"
	AlertProfileCache AlertProfile = "cache"
)

// AlertProfileThresholds holds profile-specific alert thresholds.
type AlertProfileThresholds struct {
	// Conntrack table usage percent thresholds.
	ConntrackPercent ThresholdBand
	// Interface utilization percent thresholds when NIC speed is known.
	InterfaceUtilPercent ThresholdBand
	// Absolute interface traffic thresholds in bytes/sec (peak of RX/TX).
	InterfaceBytesPerSec ThresholdBand
	// Spike ratio thresholds against moving baseline (peak / baseline).
	InterfaceSpikeRatio ThresholdBand
	// Baseline floor for spike ratio denominator in bytes/sec.
	InterfaceBaselineFloorPerSec float64
}

// AlertProfileSpec describes one workload profile.
type AlertProfileSpec struct {
	Name        AlertProfile
	Label       string
	Description string
	Thresholds  AlertProfileThresholds
}

var orderedAlertProfiles = []AlertProfile{
	AlertProfileWeb,
	AlertProfileDB,
	AlertProfileCache,
}

var alertProfileSpecs = map[AlertProfile]AlertProfileSpec{
	AlertProfileWeb: {
		Name:        AlertProfileWeb,
		Label:       "WEB",
		Description: "Burst-friendly profile for edge/API workloads.",
		Thresholds: AlertProfileThresholds{
			ConntrackPercent:             ThresholdBand{Warn: 70, Crit: 85},
			InterfaceUtilPercent:         ThresholdBand{Warn: 60, Crit: 85},
			InterfaceBytesPerSec:         ThresholdBand{Warn: 80.0 * 1024 * 1024, Crit: 200.0 * 1024 * 1024},
			InterfaceSpikeRatio:          ThresholdBand{Warn: 2.0, Crit: 3.0},
			InterfaceBaselineFloorPerSec: 5.0 * 1024 * 1024,
		},
	},
	AlertProfileDB: {
		Name:        AlertProfileDB,
		Label:       "DB",
		Description: "Stricter profile for steadier database traffic.",
		Thresholds: AlertProfileThresholds{
			ConntrackPercent:             ThresholdBand{Warn: 55, Crit: 70},
			InterfaceUtilPercent:         ThresholdBand{Warn: 45, Crit: 70},
			InterfaceBytesPerSec:         ThresholdBand{Warn: 40.0 * 1024 * 1024, Crit: 120.0 * 1024 * 1024},
			InterfaceSpikeRatio:          ThresholdBand{Warn: 1.6, Crit: 2.2},
			InterfaceBaselineFloorPerSec: 3.0 * 1024 * 1024,
		},
	},
	AlertProfileCache: {
		Name:        AlertProfileCache,
		Label:       "CACHE",
		Description: "Throughput-tolerant profile for cache tier nodes.",
		Thresholds: AlertProfileThresholds{
			ConntrackPercent:             ThresholdBand{Warn: 75, Crit: 90},
			InterfaceUtilPercent:         ThresholdBand{Warn: 70, Crit: 90},
			InterfaceBytesPerSec:         ThresholdBand{Warn: 120.0 * 1024 * 1024, Crit: 320.0 * 1024 * 1024},
			InterfaceSpikeRatio:          ThresholdBand{Warn: 2.5, Crit: 3.8},
			InterfaceBaselineFloorPerSec: 8.0 * 1024 * 1024,
		},
	},
}

// DefaultAlertProfile returns the default profile used by live dashboard.
func DefaultAlertProfile() AlertProfile {
	return AlertProfileWeb
}

// ParseAlertProfile validates and normalizes alert profile input.
func ParseAlertProfile(raw string) (AlertProfile, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(AlertProfileWeb):
		return AlertProfileWeb, true
	case string(AlertProfileDB):
		return AlertProfileDB, true
	case string(AlertProfileCache):
		return AlertProfileCache, true
	default:
		return "", false
	}
}

// AlertProfiles returns profile specs in UI order.
func AlertProfiles() []AlertProfileSpec {
	out := make([]AlertProfileSpec, 0, len(orderedAlertProfiles))
	for _, name := range orderedAlertProfiles {
		out = append(out, AlertProfileSpecFor(name))
	}
	return out
}

// NextAlertProfile returns the next profile in cycle order.
func NextAlertProfile(current AlertProfile) AlertProfile {
	for i, name := range orderedAlertProfiles {
		if name == current {
			return orderedAlertProfiles[(i+1)%len(orderedAlertProfiles)]
		}
	}
	return DefaultAlertProfile()
}

// AlertProfileSpecFor returns a normalized profile spec; unknown input falls back to default.
func AlertProfileSpecFor(profile AlertProfile) AlertProfileSpec {
	spec, ok := alertProfileSpecs[profile]
	if !ok {
		spec = alertProfileSpecs[DefaultAlertProfile()]
	}

	normalizeBand(&spec.Thresholds.ConntrackPercent)
	normalizeBand(&spec.Thresholds.InterfaceUtilPercent)
	normalizeBand(&spec.Thresholds.InterfaceBytesPerSec)
	normalizeBand(&spec.Thresholds.InterfaceSpikeRatio)
	if spec.Thresholds.InterfaceBaselineFloorPerSec < 0 {
		spec.Thresholds.InterfaceBaselineFloorPerSec = 0
	}
	return spec
}
