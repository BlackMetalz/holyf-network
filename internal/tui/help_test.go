package tui

import (
	"strings"
	"testing"

	tuioverlays "github.com/BlackMetalz/holyf-network/internal/tui/overlays"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

func TestBuildLiveHelpTextTopOutgoingGroup(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 2
	a.topDirection = tuishared.TopConnectionOutgoing
	a.groupView = true

	text := tuioverlays.BuildLiveHelpText(tuioverlays.LiveHelpContext{FocusIndex: a.focusIndex, Direction: a.topDirection, GroupView: a.groupView})
	for _, want := range []string{
		"Current Panel",
		"Top Connections (OUT, group view)",
		"Toggle to IN mode",
		"Switch to connections view",
		"Trace packet for selected peer/port",
		"Open trace packet history",
		"Disabled in OUT mode",
		"Global Navigation",
		"Other Panels",
		"Connection States",
		"Diagnosis",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q, got: %q", want, text)
		}
	}
	if strings.Count(text, "Top Connections (OUT, group view)") != 1 {
		t.Fatalf("current panel should not be repeated under Other Panels: %q", text)
	}
}

func TestBuildLiveHelpTextDiagnosisFocus(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 4

	text := tuioverlays.BuildLiveHelpText(tuioverlays.LiveHelpContext{FocusIndex: a.focusIndex, Direction: a.topDirection, GroupView: a.groupView})
	for _, want := range []string{
		"Current Panel",
		"Diagnosis",
		"Show diagnosis history",
		"Top Connections",
		"Logs / Blocks",
		"t trace history",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q, got: %q", want, text)
		}
	}
}
