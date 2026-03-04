package tui

import (
	"fmt"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) promptBlockedPeers() {
	blocks := a.snapshotDisplayActiveBlocks()
	if len(blocks) == 0 {
		a.setStatusNote("No active blocked peers", 4*time.Second)
		return
	}

	selectedIndex := 0
	list := tview.NewList().
		ShowSecondaryText(true)
	list.SetBorder(false)
	list.SetMainTextColor(tcell.ColorWhite)
	list.SetSecondaryTextColor(tcell.ColorGreen)

	closeModal := func() {
		a.pages.RemovePage("blocked-peers")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	showRemoveResultPopup := func(message string, onClose func()) {
		shownAt := time.Now()
		closePopup := func() {
			a.pages.RemovePage("blocked-peers-remove-result")
			a.updateStatusBar()
			if onClose != nil {
				onClose()
			}
		}

		modal := tview.NewModal().
			SetText(message).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(_ int, _ string) {
				// Ignore the first Enter right after opening to avoid
				// accidentally closing immediately from the Remove button keypress.
				if time.Since(shownAt) < 200*time.Millisecond {
					return
				}
				closePopup()
			})
		modal.SetTitle(" Remove Block ")
		modal.SetBorder(true)
		modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if time.Since(shownAt) < 200*time.Millisecond {
				return nil
			}
			switch event.Key() {
			case tcell.KeyEsc:
				closePopup()
				return nil
			}
			if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
				closePopup()
				return nil
			}
			return event
		})

		a.pages.RemovePage("blocked-peers-remove-result")
		a.pages.AddPage("blocked-peers-remove-result", modal, true, true)
		a.pages.SendToFront("blocked-peers-remove-result")
		a.updateStatusBar()
		a.app.SetFocus(modal)
	}

	var refreshList func(nextIndex int)
	removeSelected := func() {
		if selectedIndex < 0 || selectedIndex >= len(blocks) {
			a.setStatusNote("No blocked peer selected", 4*time.Second)
			return
		}
		spec := blocks[selectedIndex].Spec
		if err := actions.UnblockPeer(spec); err != nil {
			a.setStatusNote("Unblock failed: "+shortStatus(err.Error(), 64), 8*time.Second)
			a.addActionLog(fmt.Sprintf("remove block %s:%d failed: %s", spec.PeerIP, spec.LocalPort, shortStatus(err.Error(), 60)))
			return
		}
		a.removeActiveBlock(spec)
		a.setStatusNote(fmt.Sprintf("Unblocked %s:%d", spec.PeerIP, spec.LocalPort), 6*time.Second)
		a.addActionLog(fmt.Sprintf("removed block %s:%d", spec.PeerIP, spec.LocalPort))

		remaining := a.snapshotDisplayActiveBlocks()
		if len(remaining) == 0 {
			closeModal()
			showRemoveResultPopup(
				fmt.Sprintf("Removed block %s:%d", spec.PeerIP, spec.LocalPort),
				func() {
					a.app.SetFocus(a.panels[a.focusIndex])
				},
			)
			return
		}

		refreshList(selectedIndex)
		showRemoveResultPopup(
			fmt.Sprintf("Removed block %s:%d", spec.PeerIP, spec.LocalPort),
			func() {
				a.app.SetFocus(list)
			},
		)
	}

	list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		selectedIndex = index
	})

	refreshList = func(nextIndex int) {
		blocks = a.snapshotDisplayActiveBlocks()
		if len(blocks) == 0 {
			closeModal()
			a.setStatusNote("No active blocked peers", 4*time.Second)
			a.refreshData()
			return
		}

		if nextIndex < 0 {
			nextIndex = 0
		}
		if nextIndex >= len(blocks) {
			nextIndex = len(blocks) - 1
		}

		list.Clear()
		for _, entry := range blocks {
			main := formatBlockedSpec(entry.Spec)
			secondary := formatBlockedListSecondary(entry)
			list.AddItem(main, secondary, 0, nil)
		}
		selectedIndex = nextIndex
		list.SetCurrentItem(nextIndex)
	}
	refreshList(0)

	form := tview.NewForm().
		AddButton("Remove", func() {
			removeSelected()
		}).
		AddButton("Close", func() {
			closeModal()
		})
	form.SetBorder(false)
	form.SetButtonsAlign(tview.AlignRight)
	form.SetItemPadding(1)
	form.SetCancelFunc(func() {
		closeModal()
	})

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		case tcell.KeyEnter:
			removeSelected()
			return nil
		case tcell.KeyTab:
			a.app.SetFocus(form)
			return nil
		case tcell.KeyDelete, tcell.KeyBackspace, tcell.KeyBackspace2:
			removeSelected()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		case tcell.KeyTab:
			a.app.SetFocus(list)
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(form, 3, 0, false)
	content.SetBorder(true)
	content.SetTitle(" Blocked Peers ")

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(content, 98, 0, true).
			AddItem(nil, 0, 1, false),
			16, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("blocked-peers", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(list)
}
