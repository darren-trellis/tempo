package view

import (
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// textViewWrapped records the wrap flag, which tview keeps private, so the
// scrollbar and the JSON tree can tell a wrapped pane from one that pans.
var textViewWrapped sync.Map

func noteTextViewWrap(view *tview.TextView, wrap bool) {
	if view == nil {
		return
	}
	textViewWrapped.Store(view, wrap)
}

func textViewWraps(view *tview.TextView) bool {
	if view == nil {
		return false
	}
	wrap, ok := textViewWrapped.Load(view)
	return ok && wrap.(bool)
}

func textViewPageSize(view *tview.TextView) int {
	_, _, _, height := view.GetInnerRect()
	if height < 1 {
		return 10
	}
	return height
}

func handleTextViewScroll(view *tview.TextView, event *tcell.EventKey) bool {
	if view == nil || event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyUp:
		scrollTextView(view, -1)
		return true
	case tcell.KeyDown:
		scrollTextView(view, 1)
		return true
	case tcell.KeyLeft:
		scrollTextViewHoriz(view, -1)
		return true
	case tcell.KeyRight:
		scrollTextViewHoriz(view, 1)
		return true
	case tcell.KeyPgUp, tcell.KeyCtrlB:
		scrollTextView(view, -textViewPageSize(view))
		return true
	case tcell.KeyPgDn, tcell.KeyCtrlF:
		scrollTextView(view, textViewPageSize(view))
		return true
	case tcell.KeyHome:
		view.ScrollToBeginning()
		return true
	case tcell.KeyEnd:
		view.ScrollToEnd()
		return true
	}
	switch event.Rune() {
	case 'h':
		scrollTextViewHoriz(view, -1)
		return true
	case 'l':
		scrollTextViewHoriz(view, 1)
		return true
	case 'j':
		scrollTextView(view, 1)
		return true
	case 'k':
		scrollTextView(view, -1)
		return true
	case 'g':
		view.ScrollToBeginning()
		return true
	case 'G':
		view.ScrollToEnd()
		return true
	}
	return false
}

func wrapToggleMessage(wrap bool) string {
	if wrap {
		return "Wrap on"
	}
	return "Wrap off"
}

func setTextViewWrap(view *tview.TextView, wrap bool) {
	if view == nil {
		return
	}
	noteTextViewWrap(view, wrap)
	view.SetWrap(wrap).SetWordWrap(wrap)
	if wrap {
		row, _ := view.GetScrollOffset()
		view.ScrollTo(row, 0)
	}
}

func scrollTextView(view *tview.TextView, delta int) {
	if view == nil {
		return
	}
	row, col := view.GetScrollOffset()
	if row < 0 {
		row = 0
	}
	row += delta
	if row < 0 {
		row = 0
	}
	view.ScrollTo(row, col)
}

func scrollTextViewHoriz(view *tview.TextView, delta int) {
	if view == nil {
		return
	}
	// A wrapped pane fits the text, so a horizontal pan would only clip it.
	if textViewWraps(view) {
		row, _ := view.GetScrollOffset()
		view.ScrollTo(row, 0)
		return
	}
	row, col := view.GetScrollOffset()
	if col < 0 {
		col = 0
	}
	col += delta
	if col < 0 {
		col = 0
	}
	_, _, width, _ := view.GetInnerRect()
	max := textViewContentWidth(view) - width
	if max < 0 {
		max = 0
	}
	if col > max {
		col = max
	}
	view.ScrollTo(row, col)
}
