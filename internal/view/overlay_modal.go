package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type overlayModal struct {
	*resizableModal
	background      tview.Primitive
	interceptEscape func() bool
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

func (m *overlayModal) InterceptEscape() bool {
	return m != nil && m.interceptEscape != nil && m.interceptEscape()
}

func (m *overlayModal) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
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

func (a *App) PushModal(modal nav.Component) {
	if a == nil || a.app == nil {
		return
	}
	pushOverlayModal(a.app.Pages(), modal)
	if a.app.Menu() != nil && modal != nil {
		a.app.Menu().SetHints(modal.Hints())
	}
}

func pushOverlayModal(pages *nav.Pages, modal nav.Component) {
	if pages == nil || modal == nil {
		return
	}
	current := pages.Current()
	holdStartData(current)
	bg := modalBackgroundOf(current)
	if setter, ok := modal.(interface{ setModalBackground(tview.Primitive) }); ok {
		setter.setModalBackground(bg)
		pages.Push(modal)
		return
	}
	if inner, ok := modal.(nav.Modal); ok && bg != nil {
		pages.Push(&overlayModalPage{Modal: inner, background: bg})
		return
	}
	pages.Push(modal)
}

type overlayModalPage struct {
	nav.Modal
	background tview.Primitive
}

func (o *overlayModalPage) Draw(screen tcell.Screen) {
	if o != nil && o.background != nil {
		x, y, w, h := o.GetRect()
		o.background.SetRect(x, y, w, h)
		o.background.Draw(screen)
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
