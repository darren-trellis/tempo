package view

import "github.com/atterpac/jig/nav"

func (a *App) modalHasFocus() bool {
	return a != nil && a.app != nil && a.app.Pages() != nil && a.app.Pages().CurrentIsModal()
}

func (a *App) handleQuestionMark() bool {
	if a == nil || a.app == nil || a.app.Pages() == nil {
		return false
	}
	current := a.app.Pages().Current()
	if helpModalOf(current) != nil {
		a.closeHelp()
		return true
	}
	if a.app.Pages().CurrentIsModal() {
		return false
	}
	a.showHelp()
	return true
}

func (a *App) syncModalHints(c nav.Component) {
	if a == nil || a.menu == nil {
		return
	}
	if nav.IsModal(c) {
		a.modalHintsOn = true
		a.menu.SetHints(c.Hints())
		return
	}
	a.modalHintsOn = false
	a.menu.SetHints(nil)
}

func helpModalOf(c nav.Component) *HelpModal {
	switch v := c.(type) {
	case *HelpModal:
		return v
	case *overlayModalPage:
		if h, ok := v.Modal.(*HelpModal); ok {
			return h
		}
	}
	return nil
}

func (a *App) showTimelineLegend() {
	modal := NewTimelineLegendModal()
	modal.SetOnClose(func() {
		a.app.Pages().DismissModal()
	})
	a.PushModal(modal)
	a.app.SetFocus(modal)
}

func (a *App) showHelp() {
	helpModal := NewHelpModal()

	// Get current view's hints
	current := a.app.Pages().Current()
	if current != nil {
		if named, ok := current.(interface{ Name() string }); ok {
			helpModal.SetViewHints(named.Name(), current.Hints())
		}
	}

	helpModal.SetOnClose(func() {
		a.closeHelp()
	})

	a.PushModal(helpModal)
	a.app.SetFocus(helpModal)
}

func (a *App) closeHelp() {
	a.app.Pages().DismissModal()
}
