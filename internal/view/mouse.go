package view

import (
	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
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

// setAppMouse turns terminal mouse reporting on or off at runtime.
func setAppMouse(app *tview.Application, enabled bool) {
	if app == nil {
		return
	}
	app.EnableMouse(enabled)
}

// mouseToggleMessage describes the new mouse state for a toast.
func mouseToggleMessage(enabled bool) string {
	if enabled {
		return "Mouse enabled"
	}
	return "Mouse disabled - terminal selection available"
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
	prev := table.GetMouseCapture()
	table.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftDoubleClick {
			if handler := table.InputHandler(); handler != nil {
				handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
			}
			return tview.MouseConsumed, nil
		}
		if prev != nil {
			return prev(action, event)
		}
		return action, event
	})
}

// bindGraphHorizontalScroll pans a node graph with a horizontal wheel. The
// component pans with h/l on its own, so forward the wheel as those keys.
func bindGraphHorizontalScroll(graph *components.NodeGraph) {
	if graph == nil {
		return
	}
	prev := graph.GetMouseCapture()
	graph.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if delta := horizontalMouseDelta(action, event); delta != 0 {
			key := 'l'
			if delta < 0 {
				key = 'h'
			}
			if handler := graph.InputHandler(); handler != nil {
				handler(tcell.NewEventKey(tcell.KeyRune, key, tcell.ModNone), func(tview.Primitive) {})
			}
			return tview.MouseConsumed, nil
		}
		if prev != nil {
			return prev(action, event)
		}
		return action, event
	})
}

// droppedWheel swallows a wheel gesture without reporting it as consumed.
// tview redraws whenever a mouse handler consumes an event, and its widgets
// consume every wheel tick and only clamp when they draw. A flick against a
// boundary therefore queues a full redraw per tick and the next gesture waits
// behind them, so a tick that cannot move the pane is better dropped.
func droppedWheel(action tview.MouseAction) (tview.MouseAction, *tcell.EventMouse) {
	return action, nil
}

func verticalMouseDelta(action tview.MouseAction, event *tcell.EventMouse) int {
	if event != nil && event.Modifiers()&tcell.ModShift != 0 {
		return 0
	}
	switch action {
	case tview.MouseScrollUp:
		return -1
	case tview.MouseScrollDown:
		return 1
	}
	return 0
}

func horizontalMouseDelta(action tview.MouseAction, event *tcell.EventMouse) int {
	switch action {
	case tview.MouseScrollLeft:
		return -1
	case tview.MouseScrollRight:
		return 1
	case tview.MouseScrollUp:
		if event != nil && event.Modifiers()&tcell.ModShift != 0 {
			return -1
		}
	case tview.MouseScrollDown:
		if event != nil && event.Modifiers()&tcell.ModShift != 0 {
			return 1
		}
	}
	return 0
}

func resolveMouseScrollStep(step func() int) int {
	if step != nil {
		if n := step(); n > 0 {
			return n
		}
	}
	return config.DefaultMouseScrollStep
}

func mouseScrollStepFromApp(app *App) int {
	if app == nil {
		return config.DefaultMouseScrollStep
	}
	return app.Config().MouseScrollStepSize()
}
