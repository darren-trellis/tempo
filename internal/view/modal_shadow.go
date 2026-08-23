package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	modalShadowX = 2
	modalShadowY = 1
)

type shadowedModal struct {
	*components.Modal
	background tview.Primitive
}

func newModal(cfg components.ModalConfig) *shadowedModal {
	cfg.Backdrop = false
	return &shadowedModal{Modal: components.NewModal(cfg)}
}

func (m *shadowedModal) setModalBackground(p tview.Primitive) {
	if m == nil || p == nil || p == m {
		return
	}
	m.background = p
}

func (m *shadowedModal) modalBackground() tview.Primitive {
	if m == nil {
		return nil
	}
	return m.background
}

func (m *shadowedModal) drawBackground(screen tcell.Screen) {
	if m == nil || m.background == nil || screen == nil {
		return
	}
	x, y, w, h := m.GetRect()
	m.background.SetRect(x, y, w, h)
	m.background.Draw(screen)
}

func (m *shadowedModal) Draw(screen tcell.Screen) {
	m.drawBackground(screen)
	m.Modal.Draw(screen)
	drawModalShadow(screen, m.GetPanel())
}

func drawModalShadow(screen tcell.Screen, panel *components.Panel) {
	if screen == nil || panel == nil {
		return
	}
	x, y, w, h := panel.GetRect()
	if w <= 0 || h <= 0 {
		return
	}
	sw, sh := screen.Size()
	if x <= 0 && y <= 0 && x+w >= sw && y+h >= sh {
		return
	}

	sx, sy := x+modalShadowX, y+modalShadowY
	for row := sy; row < sy+h; row++ {
		for col := sx; col < sx+w; col++ {
			if col >= x && col < x+w && row >= y && row < y+h {
				continue
			}
			if col < 0 || row < 0 || col >= sw || row >= sh {
				continue
			}
			mainc, combc, style, _ := screen.GetContent(col, row)
			screen.SetContent(col, row, mainc, combc, darkenStyle(style))
		}
	}
}

func darkenStyle(style tcell.Style) tcell.Style {
	fg, bg, attr := style.Decompose()
	return tcell.StyleDefault.
		Foreground(darkenColor(fg)).
		Background(darkenColor(bg)).
		Attributes(attr | tcell.AttrDim)
}

func darkenColor(c tcell.Color) tcell.Color {
	if c == tcell.ColorDefault || c == tcell.ColorReset {
		if dark := theme.BgDark(); dark != tcell.ColorDefault {
			return dark
		}
		return tcell.NewRGBColor(16, 16, 20)
	}
	r, g, b := c.RGB()
	return tcell.NewRGBColor(r/2, g/2, b/2)
}
