package panels

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/BlackMetalz/holyf-network/internal/config"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

// RenderSystemHealthPanel combines Connection States, Interface Stats, and Conntrack
// into a single unified panel with dim separators between sections.
func RenderSystemHealthPanel(
	connData collector.ConnectionData,
	retrans *collector.RetransmitRates,
	conntrackRates *collector.ConntrackRates,
	thresholds config.HealthThresholds,
	ifaceRates collector.InterfaceRates,
	spike tuishared.InterfaceSpikeAssessment,
	sys tuishared.InterfaceSystemSnapshot,
	connStateSortDesc bool,
	diagnosis *tuishared.Diagnosis,
) string {
	var sb strings.Builder

	// Section 1: Connection States (reuse existing renderer)
	sb.WriteString(RenderConnectionsPanelWithStateSort(connData, retrans, conntrackRates, thresholds, connStateSortDesc))

	// Separator
	sb.WriteString("\n\n  [dim]── Interface ──[white]\n")

	// Section 2: Interface Stats (reuse existing renderer)
	sb.WriteString(RenderInterfacePanel(ifaceRates, spike, sys))

	// Separator
	sb.WriteString("\n  [dim]── Conntrack ──[white]\n")

	// Section 3: Conntrack (reuse existing renderer)
	if conntrackRates != nil {
		sb.WriteString(RenderConntrackPanel(*conntrackRates, thresholds.ConntrackPercent))
	} else {
		sb.WriteString("  [dim]Conntrack data unavailable[white]")
	}

	// Diagnosis section (compact)
	if diagnosis != nil {
		sb.WriteString("\n  [dim]── Diagnosis ──[white]\n")
		color := tuishared.ColorForHealthLevel(diagnosis.Severity)
		sb.WriteString(fmt.Sprintf("  [%s]%s[white] %s", color, tuishared.HealthLevelLabel(diagnosis.Severity), diagnosis.Issue))
		if diagnosis.Scope != "" {
			sb.WriteString(fmt.Sprintf(" | %s", diagnosis.Scope))
		}
		sb.WriteString("\n")
		if diagnosis.Signal != "" {
			sb.WriteString(fmt.Sprintf("  [dim]Signal: %s[white]\n", diagnosis.Signal))
		}
	}

	return sb.String()
}
