package tui

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func buildAlertProfileExplainText(
	current config.AlertProfileSpec,
	all []config.AlertProfileSpec,
	ifaceName string,
	speedMbps float64,
	speedKnown bool,
	speedSampled bool,
) string {
	speedPath := fmt.Sprintf("/sys/class/net/%s/speed", strings.TrimSpace(ifaceName))
	speedLine := "  [yellow]Current NIC speed[white]: unknown (fallback absolute thresholds active)."
	if !speedSampled {
		speedLine = "  [dim]Current NIC speed[white]: warming (waiting first read)."
	} else if speedKnown && speedMbps > 0 {
		speedLine = fmt.Sprintf("  [yellow]Current NIC speed[white]: %.0f Mb/s (from sysfs).", speedMbps)
	}

	lines := []string{
		fmt.Sprintf("  [yellow]Current Profile[white]: %s", current.Label),
		fmt.Sprintf("  [dim]%s[white]", current.Description),
		"",
		"  [yellow]Hotkeys[white]: y = cycle profile, Shift+Y = open this guide.",
		fmt.Sprintf("  [dim]Speed source: %s[white]", speedPath),
		speedLine,
		"",
		"  [yellow]What changes by profile[white]:",
		"  - Conntrack warning/critical thresholds",
		"  - Interface traffic thresholds by link utilization when NIC speed is known",
		"  - Interface traffic absolute fallback thresholds when NIC speed is unknown",
		"  - Interface traffic spike ratio thresholds (peak vs baseline)",
		"",
		"  [yellow]Profile Threshold Table[white]:",
		"  [dim]Profile | Conntrack warn/crit | Interface util warn/crit | Spike ratio warn/crit[white]",
	}

	for _, spec := range all {
		lines = append(lines, fmt.Sprintf(
			"  %s | %.0f/%.0f%% | %s/%s | %.1fx/%.1fx",
			spec.Label,
			spec.Thresholds.ConntrackPercent.Warn,
			spec.Thresholds.ConntrackPercent.Crit,
			fmt.Sprintf("%.0f%%", spec.Thresholds.InterfaceUtilPercent.Warn),
			fmt.Sprintf("%.0f%%", spec.Thresholds.InterfaceUtilPercent.Crit),
			spec.Thresholds.InterfaceSpikeRatio.Warn,
			spec.Thresholds.InterfaceSpikeRatio.Crit,
		))
	}

	lines = append(lines,
		"",
		"  [dim]Fallback absolute thresholds (speed unknown):[white]",
		"  [dim]WEB 80/200 MB/s, DB 40/120 MB/s, CACHE 120/320 MB/s.[white]",
		"  [dim]Spike logic uses peak=max(RX,TX) and ratio=peak/max(EWMA baseline, floor).[white]",
		"  [dim]Switching profile resets interface baseline by design.[white]",
		"",
		"  [dim]Press Enter/Esc to close[white]",
	)
	return strings.Join(lines, "\n")
}

func (a *App) promptAlertProfileExplain() {
	closeModal := func() {
		a.pages.RemovePage("alert-profile-explain")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	current := a.currentAlertProfileSpec()
	text := buildAlertProfileExplainText(
		current,
		config.AlertProfiles(),
		a.ifaceName,
		a.ifaceSpeedMbps,
		a.ifaceSpeedKnown,
		a.ifaceSpeedSample,
	)
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) {
			closeModal()
		})
	modal.SetTitle(" Alert Profile Explain ")
	modal.SetBorder(true)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	a.pages.RemovePage("alert-profile-explain")
	a.pages.AddPage("alert-profile-explain", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}
