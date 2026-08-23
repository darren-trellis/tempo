package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
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
	m.drawBackground(screen)
	if !m.maximized {
		m.Modal.Draw(screen)
		drawModalShadow(screen, m.GetPanel())
		return
	}
	x, y, w, h := m.GetRect()
	m.GetPanel().SetRect(x, y, w, h)
	m.GetPanel().Draw(screen)
}
