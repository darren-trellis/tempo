package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

type resizableModal struct {
	*shadowedModal
	cfg       components.ModalConfig
	maximized bool
	frameless bool
}

func newResizableModal(cfg components.ModalConfig) *resizableModal {
	return &resizableModal{shadowedModal: newModal(cfg), cfg: cfg}
}

func (m *resizableModal) toggleMaximize() {
	m.maximized = !m.maximized
}

func (m *resizableModal) Draw(screen tcell.Screen) {
	m.drawBackground(screen)
	applyRowSelectionStyle(m.body)
	if panel := m.GetPanel(); panel != nil && !m.frameless {
		panel.SetFocused(true)
	}
	if !m.maximized {
		// A frameless modal has no border, so its content takes the whole panel.
		// Drawing the bordered modal first and then stretching the content over
		// it laid the panes out at two widths every frame, and each layout
		// pulled the scroll position back to the highlighted row.
		if m.frameless {
			m.placePanel()
			m.drawFramelessContent(screen)
			drawModalShadow(screen, m.GetPanel(), m.shadowStyle())
			return
		}
		m.Modal.Draw(screen)
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

// placePanel centers the panel the way the bordered modal does, without
// drawing it. The config that sized it is not readable back off the modal.
func (m *resizableModal) placePanel() {
	if m == nil || m.GetPanel() == nil {
		return
	}
	x, y, width, height := m.GetRect()
	modalWidth := m.cfg.Width
	modalHeight := m.cfg.Height
	if modalWidth == 0 {
		modalWidth = m.cfg.MinWidth
		if modalWidth == 0 {
			modalWidth = 40
		}
	}
	if modalHeight == 0 {
		modalHeight = m.cfg.MinHeight
		if modalHeight == 0 {
			modalHeight = 10
		}
	}
	m.GetPanel().SetRect(x+(width-modalWidth)/2, y+(height-modalHeight)/2, modalWidth, modalHeight)
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
