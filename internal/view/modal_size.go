package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type resizableModal struct {
	*shadowedModal
	maximized bool
}

func newResizableModal(cfg components.ModalConfig) *resizableModal {
	return &resizableModal{shadowedModal: newModal(cfg)}
}

func (m *resizableModal) toggleMaximize() {
	m.maximized = !m.maximized
}

func (m *resizableModal) Draw(screen tcell.Screen) {
	if !m.maximized {
		m.shadowedModal.Draw(screen)
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
