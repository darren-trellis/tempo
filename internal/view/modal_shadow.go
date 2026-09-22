package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	modalShadowX = 2
	modalShadowY = 1
)

type shadowedModal struct {
	*components.Modal
	background      tview.Primitive
	backgroundBelow bool
	body            tview.Primitive
	hints           []components.KeyHint
	interceptEscape func() bool
}

func newModal(cfg components.ModalConfig) *shadowedModal {
	cfg.Backdrop = false
	return &shadowedModal{Modal: components.NewModal(cfg)}
}

func (m *shadowedModal) SetContent(content tview.Primitive) *shadowedModal {
	if m == nil {
		return m
	}
	m.body = content
	m.rebuildContent()
	return m
}

func (m *shadowedModal) SetOnCancel(fn func()) *shadowedModal {
	if m == nil || m.Modal == nil {
		return m
	}
	m.Modal.SetOnCancel(fn)
	return m
}

func (m *shadowedModal) SetOnSubmit(fn func()) *shadowedModal {
	if m == nil || m.Modal == nil {
		return m
	}
	m.Modal.SetOnSubmit(fn)
	return m
}

func (m *shadowedModal) SetHints(hints []components.KeyHint) *shadowedModal {
	if m == nil {
		return m
	}
	m.hints = hints
	return m
}

func (m *shadowedModal) rebuildContent() {
	if m == nil || m.Modal == nil {
		return
	}
	content := m.body
	if content == nil {
		content = tview.NewBox()
	}
	m.Modal.SetContent(content)
	if panel := m.GetPanel(); panel != nil {
		panel.SetContent(content)
	}
}

func (m *shadowedModal) Hints() []components.KeyHint {
	if m == nil {
		return nil
	}
	if len(m.hints) > 0 {
		return m.hints
	}
	if m.Modal != nil {
		return m.Modal.Hints()
	}
	return nil
}

func (m *shadowedModal) setModalBackground(p tview.Primitive) {
	if m == nil || p == nil || p == m {
		return
	}
	m.background = p
}

// setBackgroundBelow records that the background is the page right under this
// modal, which the page stack already draws each frame.
func (m *shadowedModal) setBackgroundBelow(below bool) {
	if m != nil {
		m.backgroundBelow = below
	}
}

func (m *shadowedModal) modalBackground() tview.Primitive {
	if m == nil {
		return nil
	}
	return m.background
}

func (m *shadowedModal) drawBackground(screen tcell.Screen) {
	if m == nil || m.background == nil || screen == nil || m.backgroundBelow {
		return
	}
	x, y, w, h := m.GetRect()
	m.background.SetRect(x, y, w, h)
	m.background.Draw(screen)
}

func (m *shadowedModal) Draw(screen tcell.Screen) {
	m.drawBackground(screen)
	if panel := m.GetPanel(); panel != nil {
		panel.SetFocused(true)
	}
	m.Modal.Draw(screen)
	drawModalShadow(screen, m.GetPanel(), m.shadowStyle())
}

func (m *shadowedModal) shadowStyle() string {
	if m == nil {
		return config.ModalShadowDirectional
	}
	return resolvedModalShadow(configFromBackground(m.background))
}

func (m *shadowedModal) InterceptEscape() bool {
	return m != nil && m.interceptEscape != nil && m.interceptEscape()
}

func (m *shadowedModal) bindDropdowns(form *components.Form, fields ...*dropdownField) {
	if form != nil {
		form.SetInputCapture(dropdownFormCapture(fields...))
	}
	if m == nil {
		return
	}
	m.interceptEscape = func() bool {
		return collapseOpenDropdowns(fields...)
	}
	m.SetOnDismiss(func() bool {
		return !collapseOpenDropdowns(fields...)
	})
}

func (m *shadowedModal) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if event != nil && event.Key() == tcell.KeyEscape && m.InterceptEscape() {
			return
		}
		if m.Modal == nil {
			return
		}
		if handler := m.Modal.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	}
}

func drawModalShadow(screen tcell.Screen, panel *components.Panel, style string) {
	if screen == nil || panel == nil || style == config.ModalShadowNone {
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

	x0, y0, x1, y1 := x+modalShadowX, y+modalShadowY, x+modalShadowX+w, y+modalShadowY+h
	if style == config.ModalShadowUniform {
		x0, y0 = x-modalShadowX, y-modalShadowY
		x1, y1 = x+w+modalShadowX, y+h+modalShadowY
	}
	for row := y0; row < y1; row++ {
		for col := x0; col < x1; col++ {
			if col >= x && col < x+w && row >= y && row < y+h {
				continue
			}
			if col < 0 || row < 0 || col >= sw || row >= sh {
				continue
			}
			mainc, combc, cellStyle, _ := screen.GetContent(col, row)
			screen.SetContent(col, row, mainc, combc, darkenStyle(cellStyle))
		}
	}
}

func resolvedModalShadow(cfg *config.Config) string {
	if cfg == nil {
		return config.ModalShadowDirectional
	}
	return cfg.ResolvedModalShadow()
}

func configFromBackground(p tview.Primitive) *config.Config {
	if p == nil {
		return nil
	}
	switch v := p.(type) {
	case *WorkflowList:
		if v != nil && v.app != nil {
			return v.app.Config()
		}
	case *NamespaceList:
		if v != nil && v.app != nil {
			return v.app.Config()
		}
	case *NamespaceDetail:
		if v != nil && v.app != nil {
			return v.app.Config()
		}
	case *CommandOutputView:
		if v != nil && v.app != nil {
			return v.app.Config()
		}
	}
	if getter, ok := p.(interface{ modalBackground() tview.Primitive }); ok {
		if bg := getter.modalBackground(); bg != nil && bg != p {
			return configFromBackground(bg)
		}
	}
	return nil
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
