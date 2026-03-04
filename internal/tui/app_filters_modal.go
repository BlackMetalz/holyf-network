package tui

import (
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// promptPortFilter shows a simple input dialog for port filtering.
// Uses tview.InputField as a modal overlay.
func (a *App) promptPortFilter() {
	// If any filter is active, clear all filters.
	if a.portFilter != "" || a.textFilter != "" {
		a.portFilter = ""
		a.textFilter = ""
		a.selectedTalkerIndex = 0
		a.refreshData()
		return
	}

	// Create input field
	input := tview.NewInputField()
	input.SetLabel("Filter by port: ")
	input.SetFieldWidth(10)
	input.SetBorder(true)
	input.SetTitle(" Port Filter ")

	// Accept only numbers
	input.SetAcceptanceFunc(tview.InputFieldInteger)

	// On Enter: set filter, close dialog, refresh
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			a.portFilter = input.GetText()
			a.selectedTalkerIndex = 0
		}
		// On Enter or Escape: close the dialog
		a.pages.RemovePage("filter")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
		a.refreshData()
	})

	// Center the input field using Flex spacers
	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 30, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("filter", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(input)
}

func (a *App) promptTextFilter() {
	if a.focusIndex != 2 {
		a.setStatusNote("Focus Top Connections before search", 4*time.Second)
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Search (contains): ")
	input.SetFieldWidth(36)
	input.SetText(a.textFilter)
	input.SetBorder(true)
	input.SetTitle(" Search Filter ")

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			entered := strings.TrimSpace(input.GetText())
			if entered == "" {
				a.portFilter = ""
				a.textFilter = ""
			} else {
				a.textFilter = entered
			}
			a.selectedTalkerIndex = 0
			a.refreshData()
		}
		a.pages.RemovePage("search")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 54, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("search", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(input)
}
