package view

import (
	"time"
)

const loadingFrameInterval = 120 * time.Millisecond

var loadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func loadingSpinner(frame int) string {
	if frame < 0 {
		frame = -frame
	}
	return loadingFrames[frame%len(loadingFrames)]
}

func loadingLabel(frame int) string {
	return loadingSpinner(frame) + " Loading"
}

func (a *App) SetViewLoading(key string, loading bool) {
	a.setViewLoad(key, loading, false)
}

func (a *App) SetViewRefreshing(key string, loading bool) {
	a.setViewLoad(key, loading, true)
}

func (a *App) setViewLoad(key string, loading, quiet bool) {
	if a == nil {
		return
	}

	a.loadMu.Lock()
	if a.loadingViews == nil {
		a.loadingViews = make(map[string]bool)
	}
	if a.loadingQuiet == nil {
		a.loadingQuiet = make(map[string]bool)
	}
	if loading {
		a.loadingViews[key] = true
		if quiet {
			a.loadingQuiet[key] = true
		} else {
			delete(a.loadingQuiet, key)
		}
	} else {
		delete(a.loadingViews, key)
		delete(a.loadingQuiet, key)
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
	// QueueUpdateDraw waits for the event loop. After the splash screen, Start()
	// runs on the main thread before Run(), so a direct call would deadlock and
	// look like tempo exited. The same call from a key handler (e.g. refresh)
	// would also stall the event loop. Always dispatch from a goroutine.
	go a.app.QueueUpdateDraw(func() {
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
	if a.quietOnlyLocked() {
		return loadingSpinner(a.loadingFrame)
	}
	return loadingLabel(a.loadingFrame)
}

func (a *App) quietOnlyLocked() bool {
	for key := range a.loadingViews {
		if !a.loadingQuiet[key] {
			return false
		}
	}
	return true
}

// renderLoading puts the spinner on the hint bar. An empty label clears it.
func (a *App) renderLoading(label string) {
	a.paintConnectionSection(label)
}

func (a *App) paintConnectionSection(label string) {
	if a == nil {
		return
	}
	a.connectionLabel = label
}
