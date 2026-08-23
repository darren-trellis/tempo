package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
)

type resizableModal struct {
	*shadowedModal
	maximized bool
	frameless bool
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
		if m.frameless {
			m.drawFramelessContent(screen)
		}
		drawModalShadow(screen, m.GetPanel())
		return
	}
	x, y, w, h := m.GetRect()
	m.GetPanel().SetRect(x, y, w, h)
	if m.frameless {
		m.drawFramelessContent(screen)
		return
	}
	m.GetPanel().Draw(screen)
}

func (m *resizableModal) drawFramelessContent(screen tcell.Screen) {
	if m == nil {
		return
	}
	panel := m.GetPanel()
	if panel == nil {
		return
	}
	content := panel.GetContent()
	if content == nil {
		return
	}
	x, y, w, h := panel.GetRect()
	content.SetRect(x, y, w, h)
	content.Draw(screen)
}
