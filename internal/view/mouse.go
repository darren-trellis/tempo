package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type mouseCapturer interface {
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
			return tview.MouseConsumed, nil
		}
		return action, event
	})
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
