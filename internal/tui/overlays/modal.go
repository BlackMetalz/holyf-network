package overlays

import "github.com/rivo/tview"

func CreateCenteredTextViewModal(title, text string) (*tview.Flex, *tview.TextView) {
	view := tview.NewTextView()
	view.SetDynamicColors(true)
	view.SetText(text)
	view.SetTextAlign(tview.AlignLeft)
	view.SetBorder(true)
	view.SetTitle(title)
	view.SetTitleAlign(tview.AlignCenter)

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(view, 0, 1, true)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(inner, 88, 0, true).
		AddItem(nil, 0, 1, false)

	return modal, view
}
