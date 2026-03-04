package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// updateStatusBar updates the bottom status bar with current state.
func (a *App) updateStatusBar() {
	// Calculate time since last refresh
	ago := "never"
	if !a.lastRefresh.IsZero() {
		elapsed := time.Since(a.lastRefresh).Truncate(time.Second)
		if elapsed < 1*time.Second {
			ago = "just now"
		} else {
			ago = elapsed.String() + " ago"
		}
	}

	// Build state indicators
	stateText := ""
	if a.paused.Load() {
		stateText += " [red]PAUSED[white] |"
	}
	if a.zoomed {
		stateText += " [aqua]ZOOMED[white] |"
	}
	if a.sensitiveIP {
		stateText += " [yellow]IP MASK[white] |"
	}
	if time.Now().Before(a.statusNoteUntil) && a.statusNote != "" {
		stateText += fmt.Sprintf(" [yellow]%s[white] |", a.statusNote)
	}

	page := a.frontPageName()
	hotkeysStyled, hotkeysPlain := statusHotkeysForPage(page)
	leftStyled := fmt.Sprintf(
		" [yellow]%s[white] |%s Updated: [green]%s[white] | Refresh: [green]%ds[white] | %s",
		a.ifaceName,
		stateText,
		ago,
		a.refreshSec,
		hotkeysStyled,
	)
	leftPlain := fmt.Sprintf(
		" %s |%s Updated: %s | Refresh: %ds | %s",
		a.ifaceName,
		stripStatusColors(stateText),
		ago,
		a.refreshSec,
		hotkeysPlain,
	)
	versionLabel := "holyf-network " + a.appVersion
	rightStyled := " [dim]" + versionLabel + "[white]"
	rightPlain := " " + versionLabel

	text := leftStyled
	_, _, width, _ := a.statusBar.GetInnerRect()
	if width > 0 {
		pad := width - utf8.RuneCountInString(leftPlain) - utf8.RuneCountInString(rightPlain)
		if pad > 0 {
			text = leftStyled + strings.Repeat(" ", pad) + rightStyled
		}
	}

	a.statusBar.SetText(text)
}

func (a *App) frontPageName() string {
	if a.pages == nil {
		return "main"
	}
	name, _ := a.pages.GetFrontPage()
	if strings.TrimSpace(name) == "" {
		return "main"
	}
	return name
}

func statusHotkeysForPage(page string) (styled string, plain string) {
	switch page {
	case "help":
		return "[dim]any key[white]=close", "any key=close"
	case "filter":
		return "[dim]Enter[white]=apply [dim]Esc[white]=cancel", "Enter=apply Esc=cancel"
	case "search":
		return "[dim]Enter[white]=apply [dim]Esc[white]=cancel", "Enter=apply Esc=cancel"
	case "kill-peer-form":
		return "[dim]Tab[white]=field [dim]Enter[white]=next [dim]Esc[white]=cancel", "Tab=field Enter=next Esc=cancel"
	case "kill-peer":
		return "[dim]<-/->[white]=choose [dim]Enter[white]=confirm [dim]Esc[white]=cancel", "<-/->=choose Enter=confirm Esc=cancel"
	case "blocked-peers":
		return "[dim]Up/Down[white]=select [dim]Enter[white]=remove [dim]Del[white]=remove [dim]Tab[white]=buttons [dim]Esc[white]=close",
			"Up/Down=select Enter=remove Del=remove Tab=buttons Esc=close"
	case "action-log":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	case "blocked-peers-remove-result", "block-summary":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	default:
		return "[dim]r p f k o Q/S/P/R g b h z ? q[white]", "r p f k o Q/S/P/R g b h z ? q"
	}
}

func stripStatusColors(s string) string {
	replacer := strings.NewReplacer(
		"[red]", "",
		"[green]", "",
		"[yellow]", "",
		"[aqua]", "",
		"[white]", "",
		"[dim]", "",
	)
	return replacer.Replace(s)
}

func (a *App) setStatusNote(note string, ttl time.Duration) {
	a.statusNote = strings.TrimSpace(note)
	a.statusNoteUntil = time.Now().Add(ttl)
	a.updateStatusBar()
}
