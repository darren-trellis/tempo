package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type overlayModal struct {
	*components.Modal
	background tview.Primitive
	hints      []components.KeyHint
}

func newOverlayModal(cfg components.ModalConfig, background tview.Primitive) *overlayModal {
	cfg.Backdrop = false
	return &overlayModal{
		Modal:      components.NewModal(cfg),
		background: background,
	}
}

func (m *overlayModal) SetHints(hints []components.KeyHint) *overlayModal {
	m.hints = hints
	m.Modal.SetHints(hints)
	return m
}

func (m *overlayModal) Hints() []components.KeyHint {
	if len(m.hints) > 0 {
		return m.hints
	}
	return m.Modal.Hints()
}

func (m *overlayModal) Draw(screen tcell.Screen) {
	if m.background != nil {
		x, y, w, h := m.GetRect()
		m.background.SetRect(x, y, w, h)
		m.background.Draw(screen)
	}
	m.Modal.Draw(screen)
}
