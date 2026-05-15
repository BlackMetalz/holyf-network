package panels

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

func TestRenderSystemUsageLineReady(t *testing.T) {
	t.Parallel()

	line := renderSystemUsageLine(tuishared.InterfaceSystemSnapshot{
		Ready: true,
		Usage: collector.SystemUsage{
			CPUCores: 0.24,
			CPUReady: true,
			Memory:   collector.MemoryStats{RSSBytes: 3 * 1024 * 1024},
		},
	})

	checks := []string{"CPU 0.24", "Mem 3.0 MiB RSS"}
	for _, want := range checks {
		if !strings.Contains(line, want) {
			t.Fatalf("renderSystemUsageLine missing %q in %q", want, line)
		}
	}
	if strings.Contains(line, "global") {
		t.Fatalf("renderSystemUsageLine should not show sample suffix: %q", line)
	}
}

func TestRenderSystemUsageLineWarming(t *testing.T) {
	t.Parallel()

	line := renderSystemUsageLine(tuishared.InterfaceSystemSnapshot{Ready: false, RefreshSec: 10})
	if !strings.Contains(line, "CPU warming") {
		t.Fatalf("expected warming text, got: %q", line)
	}
	if strings.Contains(line, "global") {
		t.Fatalf("warming text should not show sample suffix: %q", line)
	}
}

func TestRenderSystemUsageLineError(t *testing.T) {
	t.Parallel()

	line := renderSystemUsageLine(tuishared.InterfaceSystemSnapshot{Ready: false, Err: "cannot read /proc/self/statm"})
	if !strings.Contains(line, "unavailable") {
		t.Fatalf("expected unavailable text, got: %q", line)
	}
	if !strings.Contains(line, "cannot read /proc/self/statm") {
		t.Fatalf("expected error details, got: %q", line)
	}
}

func TestRenderSpeedLineKnown(t *testing.T) {
	t.Parallel()

	line := renderSpeedLine(tuishared.InterfaceSpikeAssessment{LinkSpeedKnown: true, LinkSpeedBps: 125000000, LinkUtilPercent: 68.2})
	checks := []string{"Speed:", "1.0 Gb/s", "util 68.2%"}
	for _, want := range checks {
		if !strings.Contains(line, want) {
			t.Fatalf("renderSpeedLine missing %q in %q", want, line)
		}
	}
}

func TestRenderTrafficLineUsesDisplayLevel(t *testing.T) {
	t.Parallel()

	line := renderTrafficLine(tuishared.InterfaceSpikeAssessment{DisplayLevel: tuishared.HealthWarn, Ready: true, Level: tuishared.HealthOK})
	if !strings.Contains(line, "SPIKE WARN") {
		t.Fatalf("renderTrafficLine missing display state in %q", line)
	}
	for _, banned := range []string{"profile", "peak", "baseline", "util"} {
		if strings.Contains(line, banned) {
			t.Fatalf("renderTrafficLine should stay minimal, found %q in %q", banned, line)
		}
	}
}

func TestRenderTrafficLineHidesStable(t *testing.T) {
	t.Parallel()

	line := renderTrafficLine(tuishared.InterfaceSpikeAssessment{DisplayLevel: tuishared.HealthOK, Ready: true})
	if strings.TrimSpace(line) != "" {
		t.Fatalf("stable traffic should be hidden, got %q", line)
	}
}

func TestRenderInterfacePanelUsesPacketRateLabel(t *testing.T) {
	t.Parallel()

	out := RenderInterfacePanel(
		collector.InterfaceRates{RxBytesPerSec: 1024, TxBytesPerSec: 2048, RxPktsPerSec: 10, TxPktsPerSec: 20},
		tuishared.InterfaceSpikeAssessment{Ready: true, DisplayLevel: tuishared.HealthOK},
		tuishared.InterfaceSystemSnapshot{Ready: true, Usage: collector.SystemUsage{CPUReady: true, CPUCores: 0.10}},
	)
	if !strings.Contains(out, "Packet rate:") {
		t.Fatalf("expected packet-rate label in panel output, got: %q", out)
	}
	if strings.Contains(out, "Traffic:") {
		t.Fatalf("stable traffic should be hidden from panel output, got: %q", out)
	}
	if !strings.Contains(out, "App Usage:") {
		t.Fatalf("expected app line in panel output, got: %q", out)
	}
	if strings.Contains(out, "global") {
		t.Fatalf("panel output should not show sample suffix: %q", out)
	}
}
