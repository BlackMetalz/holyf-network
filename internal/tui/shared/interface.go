package shared

import "github.com/BlackMetalz/holyf-network/internal/collector"

type InterfaceSystemSnapshot struct {
	Usage      collector.SystemUsage
	Ready      bool
	Err        string
	RefreshSec int
}

type InterfaceSpikeAssessment struct {
	Level               HealthLevel
	DisplayLevel        HealthLevel
	PeakBytesPerSec     float64
	BaselineBytesPerSec float64
	Ratio               float64
	LinkSpeedBps        float64
	LinkUtilPercent     float64
	LinkSpeedKnown      bool
	Ready               bool
}
