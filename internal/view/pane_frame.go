package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const paneSizeButton = 3

// paneFrame is a panel with a size button at its top right, [+] to maximize
// and [-] to minimize, that toggles the pane the way m does.
type paneFrame struct {
	*components.Panel
	maximized func() bool
	toggle    func()
}

func newPaneFrame(maximized func() bool, toggle func()) *paneFrame {
	return &paneFrame{Panel: components.NewPanel(), maximized: maximized, toggle: toggle}
}

func (f *paneFrame) isMaximized() bool {
	return f != nil && f.maximized != nil && f.maximized()
}

func (f *paneFrame) Draw(screen tcell.Screen) {
	f.Panel.Draw(screen)
	drawPaneSizeButton(screen, f, f.isMaximized())
}

func (f *paneFrame) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return f.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event != nil && f.toggle != nil {
			x, y := event.Position()
			if paneSizeButtonHit(f, x, y) && (action == tview.MouseLeftDown || action == tview.MouseLeftClick) {
				f.toggle()
				return true, nil
			}
		}
		if handler := f.Panel.MouseHandler(); handler != nil {
			return handler(action, event, setFocus)
		}
		return false, nil
	})
}

func paneSizeButtonLabel(maximized bool) string {
	if maximized {
		return "[-]"
	}
	return "[+]"
}

func paneSizeHint(maximized bool) string {
	if maximized {
		return "Minimize"
	}
	return "Maximize"
}

func paneSizeButtonOrigin(p tview.Primitive) (x, y int) {
	px, py, pw, _ := p.GetRect()
	return px + pw - 2 - paneSizeButton, py
}

func paneSizeButtonHit(p tview.Primitive, x, y int) bool {
	bx, by := paneSizeButtonOrigin(p)
	return y == by && x >= bx && x < bx+paneSizeButton
}

func drawPaneSizeButton(screen tcell.Screen, p tview.Primitive, maximized bool) {
	if screen == nil || p == nil {
		return
	}
	x, y := paneSizeButtonOrigin(p)
	_, _, w, h := p.GetRect()
	if w < paneSizeButton+4 || h < 1 {
		return
	}
	style := tcell.StyleDefault.Foreground(theme.PanelTitle()).Background(theme.Bg())
	for i, r := range paneSizeButtonLabel(maximized) {
		screen.SetContent(x+i, y, r, nil, style)
	}
}
