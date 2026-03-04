package tui

import "github.com/gdamore/tcell/v2"

// handleKeyEvent processes all keyboard input.
// Returning nil means "I handled it, don't pass to focused widget".
// Returning the event means "I didn't handle it, let tview process it".
func (a *App) handleKeyEvent(event *tcell.EventKey) *tcell.EventKey {
	// If help is showing, any key closes it
	if a.isHelpVisible() {
		a.hideHelp()
		return nil
	}
	// When non-help overlays are visible (forms/modals), let focused widget handle keys.
	if a.isOverlayVisible() {
		return event
	}

	// Handle key by type
	switch event.Key() {
	case tcell.KeyUp:
		if a.focusIndex == 2 && a.moveTopConnectionSelection(-1) {
			return nil
		}
		return event

	case tcell.KeyDown:
		if a.focusIndex == 2 && a.moveTopConnectionSelection(1) {
			return nil
		}
		return event

	case tcell.KeyEnter:
		if a.focusIndex == 2 {
			a.promptKillPeer()
			return nil
		}
		return event

	case tcell.KeyTab:
		if !a.zoomed {
			a.focusNext()
		}
		return nil

	case tcell.KeyBacktab: // Shift+Tab
		if !a.zoomed {
			a.focusPrev()
		}
		return nil

	case tcell.KeyEsc:
		if a.zoomed {
			a.exitZoom()
		}
		return nil

	case tcell.KeyRune:
		// tcell.KeyRune means a regular character key (not special key)
		switch event.Rune() {
		case 'q':
			a.cleanupActiveBlocks()
			close(a.stopChan) // Signal goroutines to stop
			a.app.Stop()
			return nil
		case '?':
			a.showHelp()
			return nil
		case 'r':
			// Send signal to refresh goroutine (non-blocking)
			select {
			case a.refreshChan <- struct{}{}:
			default: // Channel full, refresh already pending
			}
			return nil
		case 'p':
			a.paused.Store(!a.paused.Load())
			a.updateStatusBar()
			return nil
		case 's':
			a.sensitiveIP = !a.sensitiveIP
			a.refreshData()
			return nil
		case 'f':
			a.promptPortFilter()
			return nil
		case '/':
			a.promptTextFilter()
			return nil
		case 'k':
			a.promptKillPeer()
			return nil
		case 'b':
			a.promptBlockedPeers()
			return nil
		case 'h':
			a.promptActionLog()
			return nil
		case 'o':
			a.applyTopConnectionSortMode(NextSortMode(a.sortMode))
			return nil
		case 'Q', 'S', 'P', 'R':
			mode, _ := directSortModeForRune(event.Rune())
			a.applyTopConnectionSortMode(mode)
			return nil
		case 'g':
			a.groupView = !a.groupView
			a.selectedTalkerIndex = 0
			a.renderTopConnectionsPanel()
			return nil
		case 'z':
			a.toggleZoom()
			return nil
		}
	}

	// Let tview handle other keys (arrow keys for scrolling, etc.)
	return event
}

// focusNext moves focus to the next panel (wraps around).
func (a *App) focusNext() {
	a.focusIndex = (a.focusIndex + 1) % len(a.panels)
	highlightPanel(a.panels, a.focusIndex)
}

// focusPrev moves focus to the previous panel (wraps around).
func (a *App) focusPrev() {
	a.focusIndex = (a.focusIndex - 1 + len(a.panels)) % len(a.panels)
	highlightPanel(a.panels, a.focusIndex)
}

func directSortModeForRune(r rune) (SortMode, bool) {
	switch r {
	case 'Q':
		return SortByQueue, true
	case 'S':
		return SortByState, true
	case 'P':
		return SortByPeer, true
	case 'R':
		return SortByProcess, true
	default:
		return SortByQueue, false
	}
}

func (a *App) applyTopConnectionSortMode(mode SortMode) {
	a.sortMode = mode
	a.selectedTalkerIndex = 0
	a.renderTopConnectionsPanel()
}
