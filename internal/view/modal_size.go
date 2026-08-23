package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type resizableModal struct {
	*components.Modal
	maximized bool
}

func newResizableModal(cfg components.ModalConfig) *resizableModal {
	return &resizableModal{Modal: components.NewModal(cfg)}
}

func (m *resizableModal) toggleMaximize() {
	m.maximized = !m.maximized
}

func (m *resizableModal) Draw(screen tcell.Screen) {
	if !m.maximized {
		m.Modal.Draw(screen)
		return
	}
	drawMaximizedOverlay(screen, m)
}

func drawMaximizedOverlay(screen tcell.Screen, current tview.Primitive) {
	m, ok := current.(*resizableModal)
	if !ok || !m.maximized {
		return
	}
	w, h := screen.Size()
	m.GetPanel().SetRect(0, 0, w, h)
	m.GetPanel().Draw(screen)
}
