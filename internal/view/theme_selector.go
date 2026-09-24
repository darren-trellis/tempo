package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/theme/themes"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) closeThemeSelector() {
	a.app.Pages().DismissModal()
}

// applyTheme switches the live theme, remembers it in the running config and
// persists it. Without the in-memory update, a later cancel would restore the
// theme the app started with rather than the one on screen.
func (a *App) applyThemeLive(name string) {
	if selected := themes.Get(name); selected != nil {
		theme.SetProvider(selected)
	}
	if a.config != nil {
		a.config.Theme = name
	}
}

func (a *App) applyTheme(name string) {
	a.applyThemeLive(name)
	if a.configUnreadable {
		// The file could not be parsed; writing would replace it with defaults.
		a.ToastError("Theme applied, but not saved: " + config.ConfigPath() + " could not be read")
		return
	}
	if a.config == nil || !a.config.ShouldAutosave() {
		return
	}
	go func() {
		// Re-read so a concurrent edit is not clobbered, but never fall back to
		// defaults: that used to overwrite the whole config with a fresh one.
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			return
		}
		cfg.Theme = name
		_ = config.Save(cfg)
	}()
}

func (a *App) showThemeSelector() {
	// Get current theme name from config
	currentTheme := "tokyonight-night"
	if a.config != nil && a.config.Theme != "" {
		currentTheme = a.config.Theme
	}
	originalTheme := currentTheme
	originalProvider := theme.Get()
	committed := false

	// restorePreview undoes whatever browsing the list previewed.
	restorePreview := func() {
		if committed {
			return
		}
		if originalProvider != nil {
			theme.SetProvider(originalProvider) // Auto-refreshes all registered views
			return
		}
		if origTheme := themes.Get(originalTheme); origTheme != nil {
			theme.SetProvider(origTheme)
		}
	}

	selectTheme := func(name string) {
		a.applyTheme(name)
		committed = true
		a.closeThemeSelector()
	}

	// Separate themes into dark and light categories
	var darkThemes, lightThemes []string
	for _, name := range themes.Names() {
		if relativeLuminance(themes.Get(name).Bg()) > 0.5 {
			lightThemes = append(lightThemes, name)
		} else {
			darkThemes = append(darkThemes, name)
		}
	}

	// Create modal with backdrop disabled so dashboard is visible for live preview
	modal := newModal(components.ModalConfig{
		Title:    "Select Theme",
		Width:    30,
		Height:   22,
		Backdrop: false,
	})

	// Create a list for theme selection
	list := tview.NewList()
	bg := theme.Bg()
	list.SetBackgroundColor(bg)
	list.SetMainTextColor(theme.Fg())
	list.SetMainTextStyle(tcell.StyleDefault.Background(bg).Foreground(theme.Fg()))
	list.SetSelectedBackgroundColor(theme.Accent())
	list.SetSelectedTextColor(bg)
	list.SetSelectedStyle(tcell.StyleDefault.Background(theme.Accent()).Foreground(bg))
	list.SetHighlightFullLine(true)
	list.ShowSecondaryText(false)

	// Track mapping from list index to theme name (for headers)
	listToTheme := make(map[int]string)
	listIdx := 0

	// Add dark themes header
	list.AddItem("[::d]─── Dark ───[-::-]", "", 0, nil)
	listIdx++

	// Add dark themes
	for _, themeName := range darkThemes {
		name := themeName // capture for closure
		prefix := "  "
		if name == currentTheme {
			prefix = "● "
		}
		listToTheme[listIdx] = name
		list.AddItem(prefix+name, "", 0, func() {
			selectTheme(name)
		})
		listIdx++
	}

	// Add light themes header
	list.AddItem("[::d]─── Light ───[-::-]", "", 0, nil)
	lightHeaderIdx := listIdx
	listIdx++

	// Add light themes
	for _, themeName := range lightThemes {
		name := themeName // capture for closure
		prefix := "  "
		if name == currentTheme {
			prefix = "● "
		}
		listToTheme[listIdx] = name
		list.AddItem(prefix+name, "", 0, func() {
			selectTheme(name)
		})
		listIdx++
	}

	// Find list index for current theme
	currentListIdx := 1 // Start after dark header
	for idx, themeName := range listToTheme {
		if themeName == currentTheme {
			currentListIdx = idx
			break
		}
	}
	list.SetCurrentItem(currentListIdx)

	// Live preview on navigation
	list.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		if themeName, ok := listToTheme[index]; ok {
			newTheme := themes.Get(themeName)
			if newTheme != nil {
				theme.SetProvider(newTheme) // Auto-refreshes all registered views
				// Update list colors for new theme
				newBg := theme.Bg()
				list.SetBackgroundColor(newBg)
				list.SetMainTextColor(theme.Fg())
				list.SetMainTextStyle(tcell.StyleDefault.Background(newBg).Foreground(theme.Fg()))
				list.SetSelectedBackgroundColor(theme.Accent())
				list.SetSelectedTextColor(newBg)
				list.SetSelectedStyle(tcell.StyleDefault.Background(theme.Accent()).Foreground(newBg))
			}
		}
	})

	modal.SetContent(list).
		SetHints([]components.KeyHint{
			{Key: "Enter", Description: "Select"},
			{Key: "Esc", Description: "Cancel"},
		}).
		SetOnCancel(func() {
			restorePreview()
			a.closeThemeSelector()
		})

	// The app dismisses a modal on escape before this capture ever runs, so the
	// revert hangs off OnDismiss and covers both routes.
	modal.SetOnDismiss(func() bool {
		restorePreview()
		return true
	})

	// Handle vim navigation and escape in the list
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		current := list.GetCurrentItem()

		// Handle Escape and q to cancel
		if event.Key() == tcell.KeyEscape || event.Rune() == 'q' {
			restorePreview()
			a.closeThemeSelector()
			return nil
		}

		switch event.Rune() {
		case 'j':
			next := current + 1
			// Skip light header
			if next == lightHeaderIdx {
				next++
			}
			if next < list.GetItemCount() {
				list.SetCurrentItem(next)
			}
			return nil
		case 'k':
			prev := current - 1
			// Skip headers
			if prev == lightHeaderIdx {
				prev--
			}
			if prev == 0 { // dark header
				prev-- // Will be -1, handled below
			}
			if prev >= 1 { // Don't go above first theme (index 1)
				list.SetCurrentItem(prev)
			}
			return nil
		}
		return event
	})

	a.PushModal(modal)
	a.app.SetFocus(list)
}

// Profile management methods
