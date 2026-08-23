package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

const (
	modalShadowX = 2
	modalShadowY = 1
)

type shadowedModal struct {
	*components.Modal
}

func newModal(cfg components.ModalConfig) *shadowedModal {
	return &shadowedModal{Modal: components.NewModal(cfg)}
}

func (m *shadowedModal) Draw(screen tcell.Screen) {
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
