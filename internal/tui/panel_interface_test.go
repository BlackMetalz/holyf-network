package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

func TestRenderSystemUsageLineReady(t *testing.T) {
	t.Parallel()

	line := renderSystemUsageLine(interfaceSystemSnapshot{
		Ready: true,
		Usage: collector.SystemUsage{
			CPUPercent: 23.5,
			CPUReady:   true,
			Memory:     collector.MemoryStats{RSSBytes: 3 * 1024 * 1024},
		},
	})

	checks := []string{"CPU 23.5%", "Mem 3.0 MiB RSS"}
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

	line := renderSystemUsageLine(interfaceSystemSnapshot{Ready: false, RefreshSec: 10})
	if !strings.Contains(line, "CPU warming") {
		t.Fatalf("expected warming text, got: %q", line)
	}
	if strings.Contains(line, "global") {
		t.Fatalf("warming text should not show sample suffix: %q", line)
	}
}

func TestRenderSystemUsageLineError(t *testing.T) {
	t.Parallel()

	line := renderSystemUsageLine(interfaceSystemSnapshot{Ready: false, Err: "cannot read /proc/self/statm"})
	if !strings.Contains(line, "unavailable") {
		t.Fatalf("expected unavailable text, got: %q", line)
	}
	if !strings.Contains(line, "cannot read /proc/self/statm") {
		t.Fatalf("expected error details, got: %q", line)
	}
}

func TestRenderSpeedLineKnown(t *testing.T) {
	t.Parallel()

	line := renderSpeedLine(interfaceSpikeAssessment{LinkSpeedKnown: true, LinkSpeedBps: 125000000, LinkUtilPercent: 68.2})
	checks := []string{"Speed:", "1.0 Gb/s", "util 68.2%"}
	for _, want := range checks {
		if !strings.Contains(line, want) {
			t.Fatalf("renderSpeedLine missing %q in %q", want, line)
		}
	}
}

func TestRenderTrafficLineUsesDisplayLevel(t *testing.T) {
	t.Parallel()

	line := renderTrafficLine(interfaceSpikeAssessment{DisplayLevel: healthWarn, Ready: true, Level: healthOK})
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

	line := renderTrafficLine(interfaceSpikeAssessment{DisplayLevel: healthOK, Ready: true})
	if strings.TrimSpace(line) != "" {
		t.Fatalf("stable traffic should be hidden, got %q", line)
	}
}

func TestRenderInterfacePanelUsesPacketRateLabel(t *testing.T) {
	t.Parallel()

	out := renderInterfacePanel(
		collector.InterfaceRates{RxBytesPerSec: 1024, TxBytesPerSec: 2048, RxPktsPerSec: 10, TxPktsPerSec: 20},
		interfaceSpikeAssessment{Ready: true, DisplayLevel: healthOK},
		interfaceSystemSnapshot{Ready: true, Usage: collector.SystemUsage{CPUReady: true, CPUPercent: 10}},
	)
	if !strings.Contains(out, "Packet rate:") {
		t.Fatalf("expected packet-rate label in panel output, got: %q", out)
	}
	if strings.Contains(out, "Traffic:") {
		t.Fatalf("stable traffic should be hidden from panel output, got: %q", out)
	}
	if !strings.Contains(out, "App:") {
		t.Fatalf("expected app line in panel output, got: %q", out)
	}
	if strings.Contains(out, "global") {
		t.Fatalf("panel output should not show sample suffix: %q", out)
	}
}
