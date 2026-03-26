package blocking

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func PromptBlockedPeers(ctx UIContext, m *Manager) {
	blocks := m.SnapshotDisplayActiveBlocks()
	if len(blocks) == 0 {
		ctx.SetStatusNote("No active blocked peers", 4*time.Second)
		return
	}

	selectedIndex := 0
	list := tview.NewList().
		ShowSecondaryText(true)
	list.SetBorder(false)
	list.SetMainTextColor(tcell.ColorWhite)
	list.SetSecondaryTextColor(tcell.ColorGreen)

	closeModal := func() {
		ctx.RemovePage("blocked-peers")
		ctx.RestoreFocus()
		ctx.UpdateStatusBar()
	}

	showRemoveResultPopup := func(message string, onClose func()) {
		shownAt := time.Now()
		closePopup := func() {
			ctx.RemovePage("blocked-peers-remove-result")
			ctx.UpdateStatusBar()
			if onClose != nil {
				onClose()
			}
		}

		modal := tview.NewModal().
			SetText(message).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(_ int, _ string) {
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

		ctx.RemovePage("blocked-peers-remove-result")
		ctx.AddPage("blocked-peers-remove-result", modal, true, true)
		ctx.SendToFront("blocked-peers-remove-result")
		ctx.UpdateStatusBar()
		ctx.SetFocus(modal)
	}

	var refreshList func(nextIndex int)
	removeSelected := func() {
		if selectedIndex < 0 || selectedIndex >= len(blocks) {
			ctx.SetStatusNote("No blocked peer selected", 4*time.Second)
			return
		}
		spec := blocks[selectedIndex].Spec
		if err := actions.UnblockPeer(spec); err != nil {
			ctx.SetStatusNote("Unblock failed: "+tuishared.ShortStatus(err.Error(), 64), 8*time.Second)
			ctx.AddActionLog(fmt.Sprintf("remove block %s:%d failed: %s", spec.PeerIP, spec.LocalPort, tuishared.ShortStatus(err.Error(), 60)))
			return
		}
		m.RemoveActiveBlock(spec)
		ctx.SetStatusNote(fmt.Sprintf("Unblocked %s:%d", spec.PeerIP, spec.LocalPort), 6*time.Second)
		ctx.AddActionLog(fmt.Sprintf("removed block %s:%d", spec.PeerIP, spec.LocalPort))

		remaining := m.SnapshotDisplayActiveBlocks()
		if len(remaining) == 0 {
			closeModal()
			showRemoveResultPopup(
				fmt.Sprintf("Removed block %s:%d", spec.PeerIP, spec.LocalPort),
				func() {
					ctx.RestoreFocus()
				},
			)
			return
		}

		refreshList(selectedIndex)
		showRemoveResultPopup(
			fmt.Sprintf("Removed block %s:%d", spec.PeerIP, spec.LocalPort),
			func() {
				ctx.SetFocus(list)
			},
		)
	}

	list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		selectedIndex = index
	})

	refreshList = func(nextIndex int) {
		blocks = m.SnapshotDisplayActiveBlocks()
		if len(blocks) == 0 {
			closeModal()
			ctx.SetStatusNote("No active blocked peers", 4*time.Second)
			ctx.RefreshData()
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
			ctx.SetFocus(form)
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
			ctx.SetFocus(list)
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

	ctx.AddPage("blocked-peers", modal, true, true)
	ctx.UpdateStatusBar()
	ctx.SetFocus(list)
}

func formatBlockedSpec(spec actions.PeerBlockSpec) string {
	return fmt.Sprintf(" %s : %d", spec.PeerIP, spec.LocalPort)
}

func formatBlockedListSecondary(entry ActiveBlockEntry) string {
	var sb strings.Builder
	sb.WriteString("   [dim]")
	if !entry.ExpiresAt.IsZero() {
		sb.WriteString("Expires in ")
		remaining := time.Until(entry.ExpiresAt).Round(time.Second)
		if remaining < 0 {
			sb.WriteString("0s")
		} else {
			sb.WriteString(remaining.String())
		}
		sb.WriteString(" | ")
	}
	sb.WriteString("[white]")
	sb.WriteString(entry.Summary)
	return sb.String()
}
