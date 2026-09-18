package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
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
	if panel := m.GetPanel(); panel != nil && !m.frameless {
		panel.SetFocused(true)
	}
	if !m.maximized {
		m.Modal.Draw(screen)
		if m.frameless {
			m.drawFramelessContent(screen)
		}
		drawModalShadow(screen, m.GetPanel(), m.shadowStyle())
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
	fillRect(screen, x, y, w, h)
	content.SetRect(x, y, w, h)
	content.Draw(screen)
}

func fillRect(screen tcell.Screen, x, y, w, h int) {
	if screen == nil || w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
	}
}
