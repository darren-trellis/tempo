package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type overlayModal struct {
	*resizableModal
	background tview.Primitive
}

func newOverlayModal(cfg components.ModalConfig, background tview.Primitive) *overlayModal {
	cfg.Backdrop = false
	modal := &overlayModal{
		resizableModal: newResizableModal(cfg),
		background:     background,
	}
	modal.setModalBackground(background)
	return modal
}

func (m *overlayModal) SetHints(hints []components.KeyHint) *overlayModal {
	if m != nil && m.resizableModal != nil {
		m.resizableModal.SetHints(hints)
	}
	return m
}

func (m *overlayModal) Hints() []components.KeyHint {
	if m == nil || m.resizableModal == nil {
		return nil
	}
	return m.resizableModal.Hints()
}

func (m *overlayModal) SetContent(content tview.Primitive) *overlayModal {
	if m != nil && m.resizableModal != nil {
		m.resizableModal.SetContent(content)
	}
	return m
}

func (a *App) PushModal(modal nav.Component) {
	if a == nil || a.app == nil {
		return
	}
	pushOverlayModal(a.app.Pages(), modal)
}

func pushOverlayModal(pages *nav.Pages, modal nav.Component) {
	if pages == nil || modal == nil {
		return
	}
	current := pages.Current()
	holdStartData(current)
	bg := modalBackgroundOf(current)
	// A modal over a view sees that view drawn by the page stack. Over another
	// modal it repaints the base view itself, which hides the lower modal.
	below := current != nil && bg == tview.Primitive(current)
	if setter, ok := modal.(interface{ setModalBackground(tview.Primitive) }); ok {
		setter.setModalBackground(bg)
		if b, ok := modal.(interface{ setBackgroundBelow(bool) }); ok {
			b.setBackgroundBelow(below)
		}
		pages.Push(modal)
		return
	}
	if inner, ok := modal.(nav.Modal); ok && bg != nil {
		pages.Push(&overlayModalPage{Modal: inner, background: bg, backgroundBelow: below})
		return
	}
	pages.Push(modal)
}

type overlayModalPage struct {
	nav.Modal
	background      tview.Primitive
	backgroundBelow bool
}

func (o *overlayModalPage) Draw(screen tcell.Screen) {
	if o != nil && o.background != nil && !o.backgroundBelow {
		x, y, w, h := o.GetRect()
		o.background.SetRect(x, y, w, h)
		o.background.Draw(screen)
	}
	if g, ok := o.Modal.(interface{ GetPanel() *components.Panel }); ok {
		if panel := g.GetPanel(); panel != nil {
			panel.SetFocused(true)
		}
	}
	o.Modal.Draw(screen)
}

func modalBackgroundOf(current tview.Primitive) tview.Primitive {
	if current == nil {
		return nil
	}
	if getter, ok := current.(interface{ modalBackground() tview.Primitive }); ok {
		if bg := getter.modalBackground(); bg != nil {
			return bg
		}
	}
	return current
}

type startDataHolder interface {
	HoldStartData()
}

func holdStartData(p tview.Primitive) {
	if h, ok := p.(startDataHolder); ok {
		h.HoldStartData()
	}
}
