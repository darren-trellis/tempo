package view

import (
	"time"

	"github.com/galaxy-io/tempo/internal/config"
)

// FilterModeCallbacks holds callbacks for filter mode.
type FilterModeCallbacks struct {
	OnSubmit func(text string)
	OnCancel func()
	OnChange func(text string)
}

// filterModeActive tracks if we're in filter mode with custom callbacks.
var filterModeCallbacks *FilterModeCallbacks

// ShowFilterMode enters filter mode with custom callbacks.
// The filter input replaces the hint bar with a "/" prompt.
func (a *App) ShowFilterMode(initialText string, callbacks FilterModeCallbacks) {
	if a == nil {
		return
	}
	filterModeCallbacks = &callbacks
	p := a.prompt()
	if p.searchHistory == nil {
		p.searchHistory = loadCommandHistory(config.SearchHistoryPath())
	}
	p.onComplete = nil
	p.suggestFn = nil
	p.suggestions.clear()
	p.onSubmit = func(text string) {
		a.stopFilterDebounce()
		p.input.SetChangedFunc(nil)
		filterModeCallbacks = nil
		a.restoreDefaultCommandCallbacks()
		a.exitPrompt()
		a.recordSearch(text)
		if callbacks.OnSubmit != nil {
			callbacks.OnSubmit(text)
		}
	}
	p.onCancel = func() {
		a.stopFilterDebounce()
		filterModeCallbacks = nil
		a.restoreDefaultCommandCallbacks()
		a.exitPrompt()
		if callbacks.OnCancel != nil {
			callbacks.OnCancel()
		}
	}
	a.enterPrompt("/ ", "Search...")
	if initialText != "" {
		p.input.SetText(initialText)
	}
	if callbacks.OnChange != nil {
		p.input.SetChangedFunc(func(text string) {
			if !p.applying {
				p.searchHistory.resetBrowse()
			}
			a.scheduleFilterChange(text, callbacks.OnChange)
		})
	}
}

// ShowSearchPrompt opens an empty "/" prompt the way vim does: typing previews
// the search, and Esc or an empty Enter puts the previous search back.
func (a *App) ShowSearchPrompt(previous string, onChange, onSubmit func(string)) {
	if onSubmit == nil {
		onSubmit = onChange
	}
	current := previous
	restore := func() {
		if current == previous {
			return
		}
		current = previous
		onChange(previous)
	}
	a.ShowFilterMode("", FilterModeCallbacks{
		OnChange: func(text string) {
			current = text
			onChange(text)
		},
		OnSubmit: func(text string) {
			if text == "" {
				restore()
				return
			}
			current = text
			onSubmit(text)
		},
		OnCancel: restore,
	})
}

// ExitFilterMode exits filter mode and restores default command bar behavior.
func (a *App) ExitFilterMode() {
	a.stopFilterDebounce()
	filterModeCallbacks = nil
	a.restoreDefaultCommandCallbacks()
	a.exitPrompt()
}

const defaultFilterChangeDelay = 80 * time.Millisecond

var filterChangeDelay = defaultFilterChangeDelay

func (a *App) stopFilterDebounce() {
	if a == nil || a.filterDebounce == nil {
		return
	}
	a.filterDebounce.Stop()
	a.filterDebounce = nil
}

func (a *App) scheduleFilterChange(text string, onChange func(string)) {
	if a == nil || onChange == nil {
		return
	}
	a.stopFilterDebounce()
	if text == "" || filterChangeDelay <= 0 {
		onChange(text)
		return
	}
	a.filterDebounceText = text
	a.filterDebounce = time.AfterFunc(filterChangeDelay, func() {
		apply := func() {
			if !a.IsFilterMode() || a.filterDebounceText != text {
				return
			}
			a.filterDebounce = nil
			onChange(text)
		}
		if a.app == nil {
			apply()
			return
		}
		a.app.QueueUpdateDraw(apply)
	})
}

// SetFilterSuggestion sets the inline ghost text suggestion for the filter input.
// The suggestion should be the full text (what the user typed + completion).
func (a *App) SetFilterSuggestion(suggestion string) {
	if a == nil {
		return
	}
	a.prompt().suggestion = suggestion
}

// IsFilterMode returns whether filter mode is active.
func (a *App) IsFilterMode() bool {
	return filterModeCallbacks != nil && a.promptActive()
}
