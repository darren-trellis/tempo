package view

import (
	"time"

	"github.com/atterpac/jig/layout"
	"github.com/atterpac/jig/theme"
)

// loadingSectionIndex is where the spinner sits in the status bar: right after
// the fixed profile, namespace, connection and codec sections. It is only drawn
// once those are in place, so it can never take one of their slots.
const loadingSectionIndex = 4

const loadingFrameInterval = 120 * time.Millisecond

var loadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// loadingLabel renders one frame of the spinner shown while a fetch is running.
func loadingLabel(frame int) string {
	if frame < 0 {
		frame = -frame
	}
	return loadingFrames[frame%len(loadingFrames)] + " loading"
}

// SetViewLoading records whether a view has a fetch in flight. Keys are per
// view so one view's flag cannot pin the spinner on for another, and the status
// bar shows the spinner for as long as any view is still loading.
func (a *App) SetViewLoading(key string, loading bool) {
	if a == nil {
		return
	}

	a.loadMu.Lock()
	if a.loadingViews == nil {
		a.loadingViews = make(map[string]bool)
	}
	if loading {
		a.loadingViews[key] = true
	} else {
		delete(a.loadingViews, key)
	}
	busy := len(a.loadingViews) > 0
	running := a.loadingStop != nil
	var start, stop chan struct{}
	switch {
	case busy && !running && a.app != nil:
		start = make(chan struct{})
		a.loadingStop = start
	case !busy && running:
		stop = a.loadingStop
		a.loadingStop = nil
		a.loadingFrame = 0
	}
	a.loadMu.Unlock()

	if stop != nil {
		close(stop)
	}
	if start != nil {
		go a.animateLoading(start)
	}
	a.drawLoading()
}

// animateLoading advances the spinner until the last fetch finishes.
func (a *App) animateLoading(stop chan struct{}) {
	ticker := time.NewTicker(loadingFrameInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			a.loadMu.Lock()
			a.loadingFrame++
			a.loadMu.Unlock()
			a.drawLoading()
		}
	}
}

// drawLoading repaints the spinner on the UI thread. The label is resolved
// inside the queued update so a frame queued just before the last fetch
// finished cannot repaint a spinner that is already gone.
func (a *App) drawLoading() {
	if a == nil || a.app == nil {
		return
	}
	a.app.QueueUpdateDraw(func() {
		a.renderLoading(a.loadingText())
	})
}

// loadingText is the spinner label right now, empty when nothing is loading.
func (a *App) loadingText() string {
	a.loadMu.Lock()
	defer a.loadMu.Unlock()
	if len(a.loadingViews) == 0 {
		return ""
	}
	return loadingLabel(a.loadingFrame)
}

// renderLoading writes the spinner section into the status bar. An empty label
// removes it again.
func (a *App) renderLoading(label string) {
	if a == nil || a.statusBar == nil {
		return
	}
	count := a.statusBar.SectionCount()
	if label == "" {
		if count > loadingSectionIndex {
			keep := make([]layout.StatusSection, 0, loadingSectionIndex)
			for i := 0; i < loadingSectionIndex; i++ {
				keep = append(keep, a.statusBar.GetSection(i))
			}
			a.statusBar.SetSections(keep)
		}
		return
	}
	section := layout.StatusSection{Text: label, ColorFunc: theme.Accent}
	switch {
	case count > loadingSectionIndex:
		a.statusBar.UpdateSection(loadingSectionIndex, section)
	case count == loadingSectionIndex:
		a.statusBar.AddSection(section)
	}
}
