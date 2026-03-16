package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
)

func TestBuildTopDiagnosisPrioritizesConntrackDropsOverRetrans(t *testing.T) {
	t.Parallel()

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
		latestTalkers: []collector.Connection{
			{LocalIP: "10.0.0.10", LocalPort: 443, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED"},
		},
	}

	diagnosis := a.buildTopDiagnosis(
		collector.ConnectionData{States: map[string]int{"ESTABLISHED": 40}, Total: 40},
		&collector.RetransmitRates{RetransPercent: 6.0, OutSegsPerSec: 120, FirstReading: false},
		&collector.ConntrackRates{UsagePercent: 10, Max: 1000, StatsAvailable: true, DropsPerSec: 1, FirstReading: false},
	)
	if diagnosis == nil {
		t.Fatalf("expected diagnosis")
	}
	if diagnosis.Headline != "Conntrack drops active" {
		t.Fatalf("expected conntrack drops diagnosis, got: %+v", diagnosis)
	}
}

func TestBuildTopDiagnosisRequiresReadySampleForRetrans(t *testing.T) {
	t.Parallel()

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
		latestTalkers: []collector.Connection{
			{LocalIP: "10.0.0.10", LocalPort: 443, RemoteIP: "198.51.100.10", RemotePort: 52001, State: "ESTABLISHED"},
		},
	}

	diagnosis := a.buildTopDiagnosis(
		collector.ConnectionData{States: map[string]int{"ESTABLISHED": 3}, Total: 3},
		&collector.RetransmitRates{RetransPercent: 9.0, OutSegsPerSec: 120, FirstReading: false},
		&collector.ConntrackRates{UsagePercent: 4, Max: 1000},
	)
	if diagnosis == nil {
		t.Fatalf("expected diagnosis")
	}
	if diagnosis.Headline == "High TCP retrans with enough sample" {
		t.Fatalf("did not expect retrans diagnosis with low sample, got: %+v", diagnosis)
	}
	if !strings.Contains(diagnosis.Reason, "LOW SAMPLE") {
		t.Fatalf("expected LOW SAMPLE fallback reason, got: %q", diagnosis.Reason)
	}
}

func TestBuildTopDiagnosisDoesNotPickTimeWaitWhenConntrackWarns(t *testing.T) {
	t.Parallel()

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
		latestTalkers:    buildStateConnections("TIME_WAIT", "198.51.100.10", 8080, "api", 1001),
	}

	diagnosis := a.buildTopDiagnosis(
		collector.ConnectionData{States: map[string]int{"TIME_WAIT": 1001}, Total: 1001},
		&collector.RetransmitRates{FirstReading: true},
		&collector.ConntrackRates{UsagePercent: 80, Max: 1000},
	)
	if diagnosis == nil {
		t.Fatalf("expected diagnosis")
	}
	if diagnosis.Headline != "Conntrack pressure near limit" {
		t.Fatalf("expected conntrack pressure to outrank TIME_WAIT churn, got: %+v", diagnosis)
	}
}

func TestBuildTopDiagnosisCloseWaitShowsAppSideLeakWordingAndMasking(t *testing.T) {
	t.Parallel()

	a := &App{
		healthThresholds: config.DefaultHealthThresholds(),
		sensitiveIP:      true,
		latestTalkers:    buildStateConnections("CLOSE_WAIT", "198.51.100.10", 8080, "server", 101),
	}

	diagnosis := a.buildTopDiagnosis(
		collector.ConnectionData{States: map[string]int{"CLOSE_WAIT": 101}, Total: 101},
		nil,
		&collector.ConntrackRates{UsagePercent: 4, Max: 1000},
	)
	if diagnosis == nil {
		t.Fatalf("expected diagnosis")
	}
	if !strings.Contains(diagnosis.Headline, "CLOSE_WAIT pressure on :8080 from xxx.xxx.100.10") {
		t.Fatalf("expected masked CLOSE_WAIT headline, got: %q", diagnosis.Headline)
	}
	if !strings.Contains(diagnosis.Reason, "local app likely is not closing sockets") {
		t.Fatalf("expected app-side socket leak wording, got: %q", diagnosis.Reason)
	}
}

func TestFindStateDiagnosisCulpritUsesActivityThenBandwidthTieBreak(t *testing.T) {
	t.Parallel()

	conns := []collector.Connection{
		{RemoteIP: "198.51.100.10", LocalPort: 8080, State: "TIME_WAIT", ProcName: "api", Activity: 10, TotalBytesPerSec: 20},
		{RemoteIP: "198.51.100.10", LocalPort: 8080, State: "TIME_WAIT", ProcName: "api", Activity: 10, TotalBytesPerSec: 20},
		{RemoteIP: "198.51.100.20", LocalPort: 9090, State: "TIME_WAIT", ProcName: "api", Activity: 20, TotalBytesPerSec: 5},
		{RemoteIP: "198.51.100.20", LocalPort: 9090, State: "TIME_WAIT", ProcName: "api", Activity: 20, TotalBytesPerSec: 5},
	}

	culprit, ok := findStateDiagnosisCulprit(conns, "TIME_WAIT")
	if !ok {
		t.Fatalf("expected culprit")
	}
	if culprit.PeerIP != "198.51.100.20" || culprit.LocalPort != 9090 {
		t.Fatalf("expected higher-activity culprit, got: %+v", culprit)
	}
}

func buildStateConnections(state, remoteIP string, localPort int, proc string, count int) []collector.Connection {
	conns := make([]collector.Connection, 0, count)
	for i := 0; i < count; i++ {
		conns = append(conns, collector.Connection{
			LocalIP:          "10.0.0.10",
			LocalPort:        localPort,
			RemoteIP:         remoteIP,
			RemotePort:       52000 + i,
			State:            state,
			ProcName:         proc,
			Activity:         int64(100 - (i % 3)),
			TotalBytesPerSec: 1024,
		})
	}
	return conns
}
