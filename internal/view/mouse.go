package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type mouseCapturer interface {
	InRect(x, y int) bool
	SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse)) *tview.Box
}

func enableAppMouse(app *tview.Application, chrome ...mouseCapturer) {
	if app == nil {
		return
	}
	app.EnableMouse(true)
	for _, c := range chrome {
		if c != nil {
			ignoreMouseFocus(c)
		}
	}
}

func ignoreMouseFocus(box mouseCapturer) {
	box.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftClick, tview.MouseLeftDoubleClick:
			if event != nil && box.InRect(event.Position()) {
				return tview.MouseConsumed, nil
			}
		}
		return action, event
	})
}

type modalPaneler interface {
	GetPanel() *components.Panel
}

func bindModalMouse(app *tview.Application, current func() tview.Primitive) {
	if app == nil || current == nil {
		return
	}
	app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if event == nil {
			return nil, action
		}
		if routeModalMouse(current(), action, event, func(p tview.Primitive) {
			app.SetFocus(p)
		}) {
			return nil, tview.MouseConsumed
		}
		return event, action
	})
}

func routeModalMouse(current tview.Primitive, action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) bool {
	paneler, ok := current.(modalPaneler)
	if !ok {
		return false
	}
	panel := paneler.GetPanel()
	if panel == nil {
		return false
	}

	x, y := event.Position()
	if panel.InRect(x, y) {
		if handler := panel.MouseHandler(); handler != nil {
			consumed, _ := handler(action, event, setFocus)
			if consumed {
				return true
			}
		}
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftClick, tview.MouseLeftDoubleClick,
			tview.MouseScrollUp, tview.MouseScrollDown:
			if setFocus != nil {
				setFocus(current)
			}
			return true
		}
		return false
	}

	switch action {
	case tview.MouseLeftDown, tview.MouseLeftClick, tview.MouseLeftDoubleClick,
		tview.MouseScrollUp, tview.MouseScrollDown:
		return true
	}
	return false
}

func bindTableDoubleClick(table *components.Table) {
	if table == nil {
		return
	}
	table.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action != tview.MouseLeftDoubleClick {
			return action, event
		}
		if handler := table.InputHandler(); handler != nil {
			handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
		}
		return tview.MouseConsumed, nil
	})
}
