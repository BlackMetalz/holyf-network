package tui

import (
	"strings"
	"testing"

	"github.com/BlackMetalz/holyf-network/internal/config"
)

func TestBuildAlertProfileExplainTextIncludesCurrentAndTable(t *testing.T) {
	t.Parallel()

	current := config.AlertProfileSpecFor(config.AlertProfileDB)
	text := buildAlertProfileExplainText(current, config.AlertProfiles(), "eth0", 1000, true, true)
	for _, want := range []string{
		"Current Profile",
		": DB",
		"Hotkeys",
		"y = cycle profile, Shift+Y = open this guide.",
		"Speed source: /sys/class/net/eth0/speed",
		"Current NIC speed",
		"Profile Threshold Table",
		"Conntrack warn/crit",
		"Interface util warn/crit",
		"Fallback absolute thresholds (speed unknown)",
		"WEB |",
		"DB |",
		"CACHE |",
		"Spike logic uses peak=max(RX,TX)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected explain text to contain %q, got: %q", want, text)
		}
	}
}
