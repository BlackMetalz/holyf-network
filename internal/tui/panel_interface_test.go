package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

func TestRenderSystemUsageLineReady(t *testing.T) {
	t.Parallel()

	line := renderSystemUsageLine(interfaceSystemSnapshot{
		Ready:      true,
		RefreshSec: 5,
		Usage: collector.SystemUsage{
			CPUPercent: 23.5,
			CPUReady:   true,
			Memory: collector.MemoryStats{
				RSSBytes: 3 * 1024 * 1024,
			},
		},
	})

	checks := []string{"CPU 23.5%", "Mem 3.0 MiB RSS", "global 5s sample"}
	for _, want := range checks {
		if !strings.Contains(line, want) {
			t.Fatalf("renderSystemUsageLine missing %q in %q", want, line)
		}
	}
}

func TestRenderSystemUsageLineWarming(t *testing.T) {
	t.Parallel()

	line := renderSystemUsageLine(interfaceSystemSnapshot{Ready: false, RefreshSec: 10})
	if !strings.Contains(line, "CPU warming") {
		t.Fatalf("expected warming text, got: %q", line)
	}
	if !strings.Contains(line, "global 10s sample") {
		t.Fatalf("expected refresh interval text, got: %q", line)
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

func TestRenderInterfacePanelIncludesSystemLine(t *testing.T) {
	t.Parallel()

	out := renderInterfacePanel(
		collector.InterfaceRates{FirstReading: true},
		interfaceSpikeAssessment{},
		interfaceSystemSnapshot{Ready: true, Usage: collector.SystemUsage{CPUReady: true, CPUPercent: 10}},
	)
	if !strings.Contains(out, "App:") {
		t.Fatalf("expected app line in panel output, got: %q", out)
	}
}
