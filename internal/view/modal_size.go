package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
)

type resizableModal struct {
	*components.Modal
	maximized bool
	margin    int
}

func newResizableModal(cfg components.ModalConfig) *resizableModal {
	return &resizableModal{
		Modal:  components.NewModal(cfg),
		margin: 1,
	}
}

func (m *resizableModal) toggleMaximize() {
	m.maximized = !m.maximized
}

func (m *resizableModal) Draw(screen tcell.Screen) {
	m.Modal.Draw(screen)
	if !m.maximized {
		return
	}
	x, y, w, h := m.GetRect()
	margin := m.margin
	if w <= 2*margin || h <= 2*margin {
		margin = 0
	}
	m.GetPanel().SetRect(x+margin, y+margin, w-2*margin, h-2*margin)
	m.GetPanel().Draw(screen)
}
