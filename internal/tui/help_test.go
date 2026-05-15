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
		"Disabled in OUT mode",
		"Global Navigation",
		"Other Panels",
		"Connection States",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q, got: %q", want, text)
		}
	}
	for _, notWant := range []string{
		"Trace packet for selected peer/port",
		"Open trace packet history",
		"Diagnosis",
	} {
		if strings.Contains(text, notWant) {
			t.Fatalf("help text should not contain %q after removal, got: %q", notWant, text)
		}
	}
	if strings.Count(text, "Top Connections (OUT, group view)") != 1 {
		t.Fatalf("current panel should not be repeated under Other Panels: %q", text)
	}
}

func TestBuildLiveHelpTextConntrackFocus(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	a.focusIndex = 3

	text := tuioverlays.BuildLiveHelpText(tuioverlays.LiveHelpContext{FocusIndex: a.focusIndex, Direction: a.topDirection, GroupView: a.groupView})
	for _, want := range []string{
		"Current Panel",
		"Conntrack",
		"Read-only pressure panel",
		"Top Connections",
		"Logs / Blocks",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q, got: %q", want, text)
		}
	}
	for _, notWant := range []string{
		"Diagnosis",
		"t trace history",
	} {
		if strings.Contains(text, notWant) {
			t.Fatalf("help text should not contain %q after removal, got: %q", notWant, text)
		}
	}
}
