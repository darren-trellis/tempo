package view

import "github.com/rivo/tview"

// previewPane names a preview pane that can fill the workflows view.
type previewPane int

const (
	previewPaneNone previewPane = iota
	previewPaneSecondary
	previewPaneTertiary
)

// focus is the focus pane that a preview pane holds.
func (p previewPane) focus() workflowFocusPane {
	if p == previewPaneTertiary {
		return focusEventDetail
	}
	return focusEvents
}

// previewPaneFor names the preview pane that holds focus, if one does.
func previewPaneFor(focus workflowFocusPane) previewPane {
	switch focus {
	case focusEvents:
		return previewPaneSecondary
	case focusEventDetail:
		return previewPaneTertiary
	}
	return previewPaneNone
}

// paneSizeHintFor is the m hint for a preview pane.
func (wl *WorkflowList) paneSizeHintFor(pane previewPane) KeyHint {
	return KeyHint{Key: "m", Description: paneSizeHint(wl.maximizedPane == pane)}
}

// togglePaneMaximized makes pane fill the workflows view, hiding the list,
// the other preview pane and the timeline, or puts the layout back when it
// already does. The pane keeps focus either way.
func (wl *WorkflowList) togglePaneMaximized(pane previewPane) {
	if wl == nil || pane == previewPaneNone {
		return
	}
	if wl.maximizedPane == pane {
		wl.maximizedPane = previewPaneNone
	} else {
		wl.maximizedPane = pane
	}
	wl.applyMainLayout()
	wl.setFocusPane(pane.focus())
}

// restorePaneSize puts the layout back if a preview pane fills the view, and
// reports whether one did.
func (wl *WorkflowList) restorePaneSize() bool {
	if wl == nil || wl.maximizedFrame() == nil {
		return false
	}
	wl.maximizedPane = previewPaneNone
	wl.applyMainLayout()
	return true
}

// maximizedFrame is the pane that fills the view, or nil. A maximized pane
// that cannot show, because the preview closed or another list tab is up,
// is forgotten, so the layout does not jump back to it later.
func (wl *WorkflowList) maximizedFrame() *paneFrame {
	if wl == nil || wl.maximizedPane == previewPaneNone {
		return nil
	}
	if wl.workflowsActive() && wl.previewModeEnabled() {
		switch {
		case wl.maximizedPane == previewPaneSecondary && wl.previewPanel != nil:
			return wl.previewPanel
		case wl.maximizedPane == previewPaneTertiary && wl.previewShowsSidePane() && wl.eventDetailPanel != nil:
			return wl.eventDetailPanel
		}
	}
	wl.maximizedPane = previewPaneNone
	return nil
}

// sizeButtonFrames lists the panes whose size button is on screen.
func (wl *WorkflowList) sizeButtonFrames() []*paneFrame {
	if frame := wl.maximizedFrame(); frame != nil {
		return []*paneFrame{frame}
	}
	var frames []*paneFrame
	if wl.workflowsActive() && wl.previewModeEnabled() {
		frames = append(frames, wl.previewPanel)
		if wl.previewShowsSidePane() {
			frames = append(frames, wl.eventDetailPanel)
		}
	}
	if wl.timelineVisible {
		frames = append(frames, wl.timelinePanel)
	}
	return frames
}

// sizeButtonAt toggles the pane whose size button is at x, y, and reports
// whether there was one.
func (wl *WorkflowList) sizeButtonAt(x, y int) bool {
	for _, frame := range wl.sizeButtonFrames() {
		if frame != nil && frame.toggle != nil && paneSizeButtonHit(frame, x, y) {
			frame.toggle()
			return true
		}
	}
	return false
}

// maximizedMouse handles the mouse while a pane fills the view. The hidden
// panes keep the places they last drew at, so only the maximized pane is
// asked where a click landed.
func (wl *WorkflowList) maximizedMouse(action tview.MouseAction, x, y int) (handled bool) {
	if action != tview.MouseLeftDown && action != tview.MouseLeftClick {
		return false
	}
	if wl.sizeButtonAt(x, y) {
		return true
	}
	if wl.maximizedPane == previewPaneSecondary {
		if kind, ok := wl.previewTabAt(x, y); ok {
			wl.setPreviewKind(kind)
			return true
		}
	}
	return false
}
