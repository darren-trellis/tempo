package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type detailFocusPane int

const (
	detailFocusEvents detailFocusPane = iota
	detailFocusEventDetail
	detailFocusWorkflow
)

func (wd *WorkflowDetail) cycleFocus(delta int) {
	next := (int(wd.focusPane) + delta) % 3
	if next < 0 {
		next += 3
	}
	wd.setFocusPane(detailFocusPane(next))
}

func (wd *WorkflowDetail) setFocusPane(pane detailFocusPane) {
	wd.focusPane = pane
	if wd.app == nil || wd.app.JigApp() == nil {
		wd.applyFocusStyles()
		return
	}
	switch pane {
	case detailFocusEventDetail:
		wd.app.JigApp().SetFocus(wd.eventDetailView)
	case detailFocusWorkflow:
		wd.app.JigApp().SetFocus(wd.workflowView)
	default:
		wd.app.JigApp().SetFocus(wd.eventTable)
	}
	wd.applyFocusStyles()
	wd.app.JigApp().Menu().SetHints(wd.Hints())
}

func (wd *WorkflowDetail) applyFocusStyles() {
	if wd.eventsPanel != nil {
		wd.eventsPanel.SetFocused(wd.focusPane == detailFocusEvents)
	}
	if wd.eventDetailPanel != nil {
		wd.eventDetailPanel.SetFocused(wd.focusPane == detailFocusEventDetail)
	}
	if wd.workflowPanel != nil {
		wd.workflowPanel.SetFocused(wd.focusPane == detailFocusWorkflow)
	}
	if wd.eventTable != nil {
		wd.eventTable.SetSelectable(wd.focusPane == detailFocusEvents, false)
	}
}

func (wd *WorkflowDetail) syncFocusFromPrimitives() {
	pane := detailFocusEvents
	switch {
	case wd.workflowView != nil && wd.workflowView.HasFocus():
		pane = detailFocusWorkflow
	case wd.eventDetailView != nil && wd.eventDetailView.HasFocus():
		pane = detailFocusEventDetail
	}
	if pane != wd.focusPane {
		wd.focusPane = pane
		if wd.app != nil && wd.app.JigApp() != nil {
			wd.app.JigApp().Menu().SetHints(wd.Hints())
		}
	}
	wd.applyFocusStyles()
}

func (wd *WorkflowDetail) paneAt(x, y int) (detailFocusPane, bool) {
	if wd.workflowPanel != nil && wd.workflowPanel.InRect(x, y) {
		return detailFocusWorkflow, true
	}
	if wd.eventDetailPanel != nil && wd.eventDetailPanel.InRect(x, y) {
		return detailFocusEventDetail, true
	}
	if wd.eventsPanel != nil && wd.eventsPanel.InRect(x, y) {
		return detailFocusEvents, true
	}
	return detailFocusEvents, false
}

func (wd *WorkflowDetail) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return wd.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		if !wd.InRect(x, y) {
			return false, nil
		}
		if pane, ok := wd.paneAt(x, y); ok {
			switch action {
			case tview.MouseLeftDown, tview.MouseLeftClick:
				if pane != wd.focusPane {
					wd.setFocusPane(pane)
				}
			}
		}
		return wd.Flex.MouseHandler()(action, event, setFocus)
	})
}

func (wd *WorkflowDetail) HandleEscape() bool {
	if wd.focusPane != detailFocusEvents {
		wd.setFocusPane(detailFocusEvents)
		return true
	}
	return false
}

func (wd *WorkflowDetail) setupPaneInput(view *tview.TextView) {
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			wd.cycleFocus(1)
			return nil
		case tcell.KeyBacktab:
			wd.cycleFocus(-1)
			return nil
		case tcell.KeyEscape:
			wd.setFocusPane(detailFocusEvents)
			return nil
		}
		if event.Rune() == 'j' {
			row, col := view.GetScrollOffset()
			view.ScrollTo(row+1, col)
			return nil
		}
		if event.Rune() == 'k' {
			row, col := view.GetScrollOffset()
			if row > 0 {
				view.ScrollTo(row-1, col)
			}
			return nil
		}
		return event
	})
}

func scrollTextView(view *tview.TextView, delta int) {
	row, col := view.GetScrollOffset()
	row += delta
	if row < 0 {
		row = 0
	}
	view.ScrollTo(row, col)
}
