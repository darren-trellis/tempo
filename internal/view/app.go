package view

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/layout"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/theme/themes"
	"github.com/galaxy-io/tempo/internal/command"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/galaxy-io/tempo/internal/update"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	connectionCheckInterval = 10 * time.Second
	reconnectInitialBackoff = 2 * time.Second
	reconnectMaxBackoff     = 30 * time.Second
	connectionCheckTimeout  = 5 * time.Second
)

// App is the main application controller.
type App struct {
	app              *layout.App
	menu             *layout.Menu
	hintPrompt       *hintPrompt
	chromeProfile    string
	chromeCodecIcon  string
	chromeCodecColor func() tcell.Color
	chromeStats      WorkflowStats
	chromeStatsOn    bool
	modalHintsOn     bool
	connectionLabel  string
	namespaceList    *NamespaceList

	statusMu    sync.Mutex
	statusText  string
	statusClear *time.Timer

	// Protected by mu - accessed from multiple goroutines
	mu            sync.RWMutex
	provider      temporal.Provider
	currentNS     string
	activeProfile string
	reconnecting  bool
	connected     bool

	// Connection monitor
	stopMonitor     chan struct{}
	stopConfigWatch chan struct{}

	// Profile management
	config *config.Config
	// configUnreadable records that the config file on disk could not be parsed,
	// so what is in memory is a fallback and must not be written over it.
	configUnreadable bool

	// Dev mode
	devMode bool

	// mouseEnabled tracks terminal mouse reporting so ctrl+o can toggle it.
	mouseEnabled bool

	// Loading indicator - which views have a fetch in flight
	loadMu       sync.Mutex
	loadingViews map[string]bool
	loadingQuiet map[string]bool
	loadingFrame int
	loadingStop  chan struct{}

	catalog namespaceCatalogStore

	filterDebounce     *time.Timer
	filterDebounceText string
}

// NewApp creates a new application controller with no provider (uses mock data).
func NewApp() *App {
	a := &App{
		currentNS: "default",
	}
	a.buildApp()
	a.setup()
	return a
}

// NewAppWithProvider creates a new application controller with a Temporal provider.
func NewAppWithProvider(provider temporal.Provider, defaultNamespace string, cfg *config.Config, activeProfile string) *App {
	a := &App{
		provider:      provider,
		currentNS:     defaultNamespace,
		stopMonitor:   make(chan struct{}),
		config:        cfg,
		activeProfile: activeProfile,
	}
	a.buildApp()
	a.setup()

	// Set initial profile name in stats bar (must be first - clears sections)
	a.setProfile(activeProfile)
	// Set initial connection status based on provider (adds section 2)
	if provider != nil {
		a.setConnected(provider.IsConnected())
	}
	a.refreshNamespaceCatalog()
	return a
}

func (a *App) buildApp() {
	a.menu = layout.NewMenu()
	a.hintPrompt = newHintPrompt()
	a.hintPrompt.history = loadCommandHistory(config.HistoryPath())
	a.hintPrompt.searchHistory = loadCommandHistory(config.SearchHistoryPath())

	a.app = layout.NewApp(layout.AppConfig{
		ShowCrumbs: false,
		BottomBar:  a.menu,
		OnComponentChange: func(c nav.Component) {
			a.syncModalHints(c)
			a.updateCrumbs()
			a.syncWorkflowStats(c)
		},
	})

	tviewApp := a.app.GetApplication()
	enableAppMouse(tviewApp, a.menu)
	a.mouseEnabled = true
	bindModalMouse(tviewApp, func() tview.Primitive {
		if a.app == nil || a.app.Pages() == nil {
			return nil
		}
		return a.app.Pages().Current()
	})
	tviewApp.SetAfterDrawFunc(func(screen tcell.Screen) {
		a.drawHintStatus(screen)
		a.drawHintLoading(screen)
		a.drawHintPrompt(screen)
		a.drawBottomChrome(screen)
	})
}

func (a *App) setup() {
	a.restoreDefaultCommandCallbacks()

	// Global key handler
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if a.handlePromptKey(event) {
			return nil
		}

		if event != nil && event.Key() == tcell.KeyEscape {
			if current := a.app.Pages().Current(); current != nil {
				if interceptor, ok := current.(interface{ InterceptEscape() bool }); ok && interceptor.InterceptEscape() {
					return nil
				}
			}
		}

		// Mouse toggle (ctrl+o) - works everywhere, modals included, so the
		// terminal's own selection can be reached from any view.
		if event.Key() == tcell.KeyCtrlO {
			a.toggleMouse()
			return nil
		}

		// Check if we're on a modal page that should handle its own escape
		isModalPage := a.app.Pages().CurrentIsModal()
		// Fallback for views using AddPage that don't implement nav.Component
		if !isModalPage {
			if frontPage, _ := a.app.Pages().GetFrontPage(); frontPage == "splash-test" {
				isModalPage = true
			}
		}

		// Global quit (only on root view, not in modals)
		if event.Rune() == 'q' && !isModalPage {
			if a.app.Pages().StackDepth() <= 1 {
				a.Stop()
				return nil
			}
		}

		// Global back navigation (skip for modals - they handle their own escape)
		if !isModalPage {
			if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyBackspace || event.Key() == tcell.KeyBackspace2 {
				// Check if current view wants to handle escape first
				if current := a.app.Pages().Current(); current != nil {
					if handler, ok := current.(EscapeHandler); ok {
						if handler.HandleEscape() {
							return nil
						}
					}
				}
				if a.app.Pages().CanPop() {
					a.app.Pages().Pop()
					if current := a.app.Pages().Current(); current != nil {
						a.app.SetFocus(current)
					}
					return nil
				}
			}
		}

		// Help: pane views open a hint modal; open modals put hints on the bottom bar.
		if event.Rune() == '?' {
			a.handleQuestionMark()
			return nil
		}

		// Theme selector (capital T) - works everywhere except modals
		if event.Rune() == 'T' && !isModalPage {
			a.showThemeSelector()
			return nil
		}

		// Profile selector (capital P) - works everywhere except modals
		if event.Rune() == 'P' && !isModalPage {
			a.ShowProfileSelector()
			return nil
		}

		// Command bar (: key) - works everywhere except modals
		if event.Rune() == ':' && !isModalPage {
			a.showCommandBar()
			return nil
		}

		// Dev mode: splash screen test (capital S)
		if a.devMode && event.Rune() == 'S' {
			a.showSplashTest()
			return nil
		}

		// Debug screen (!) - works everywhere except modals
		if event.Rune() == '!' && !isModalPage {
			a.showDebugScreen()
			return nil
		}

		return event
	})

	// Create and push the home view
	// If a namespace is defined in the connection, skip namespace list and go directly to workflows
	if a.provider != nil && a.provider.Config().Namespace != "" {
		wl := NewWorkflowList(a, a.currentNS)
		a.app.Pages().Push(wl)
	} else {
		a.namespaceList = NewNamespaceList(a)
		a.app.Pages().Push(a.namespaceList)
	}
}

func (a *App) currentContent() nav.Component {
	if a == nil || a.app == nil || a.app.Pages() == nil {
		return nil
	}
	stack := a.app.Pages().GetStack()
	for i := len(stack) - 1; i >= 0; i-- {
		if !nav.IsModal(stack[i]) {
			return stack[i]
		}
	}
	return a.app.Pages().Current()
}

func (a *App) syncWorkflowStats(c nav.Component) {
	if a == nil || c == nil || nav.IsModal(c) {
		return
	}
	if wl, ok := c.(*WorkflowList); ok {
		if !wl.workflowsActive() {
			a.ClearWorkflowStats()
		}
		return
	}
	a.ClearWorkflowStats()
}

func (a *App) updateCrumbs() {
	if a == nil || a.app == nil || a.app.Crumbs() == nil {
		return
	}
	a.app.Crumbs().Clear()
}

// Status chrome helpers.

func (a *App) setConnected(connected bool) {
	if a != nil {
		a.connected = connected
	}
	a.paintConnectionSection(a.loadingText())
	a.refreshCodecStatus()
}

func (a *App) setCodecStatus(icon string, colorFunc func() tcell.Color) {
	if a == nil {
		return
	}
	a.chromeCodecIcon = icon
	a.chromeCodecColor = colorFunc
}

func (a *App) refreshCodecStatus() {
	var endpoint, namespace string
	if a.provider != nil {
		cfg := a.provider.Config()
		endpoint = cfg.CodecEndpoint
		namespace = cfg.Namespace
	}
	if namespace == "" {
		namespace = a.currentNS
	}
	if endpoint == "" {
		a.setCodecStatus("", theme.FgDim)
		return
	}
	if a.chromeCodecIcon == "" {
		a.setCodecStatus(theme.IconCloud, theme.FgDim)
	}
	go func() {
		err := temporal.ProbeCodec(endpoint, namespace)
		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.setCodecStatus(theme.IconCloud, theme.Error)
				return
			}
			a.setCodecStatus(theme.IconCloud, theme.Success)
		})
	}()
}

func (a *App) profileTitle() string {
	if a == nil {
		return ""
	}
	if a.chromeProfile != "" {
		return a.chromeProfile
	}
	return a.activeProfile
}

func (a *App) setProfile(name string) {
	if a == nil {
		return
	}
	a.chromeProfile = name
	if wl, ok := a.currentContent().(*WorkflowList); ok {
		wl.applyProfileTitle()
	}
}

func (a *App) setNamespace(ns string) {
}

// WorkflowStats holds workflow count statistics.
type WorkflowStats struct {
	Running        int
	Completed      int
	Failed         int
	Canceled       int
	Terminated     int
	TimedOut       int
	ContinuedAsNew int
	Total          int
}

// SetWorkflowStats updates the workflow count badges on the bottom bar.
func (a *App) SetWorkflowStats(stats WorkflowStats) {
	if a == nil {
		return
	}
	a.chromeStats = stats
	a.chromeStatsOn = true
}

// ClearWorkflowStats removes workflow count badges from the bottom bar.
func (a *App) ClearWorkflowStats() {
	if a == nil {
		return
	}
	a.chromeStats = WorkflowStats{}
	a.chromeStatsOn = false
}

// App returns the underlying jig layout.App.
func (a *App) JigApp() *layout.App {
	return a.app
}

// Provider returns the Temporal provider.
// Thread-safe: can be called from any goroutine.
func (a *App) Provider() temporal.Provider {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.provider
}

// SetNamespace sets the current namespace context.
// Thread-safe: can be called from any goroutine.
func (a *App) SetNamespace(ns string) {
	a.mu.Lock()
	prev := a.currentNS
	a.currentNS = ns
	a.mu.Unlock()
	a.setNamespace(ns)
	if ns != prev || !a.catalog.has(ns) {
		a.refreshNamespaceCatalogFor(ns)
	}
}

// CurrentNamespace returns the current namespace.
// Thread-safe: can be called from any goroutine.
func (a *App) CurrentNamespace() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentNS
}

// NavigateToWorkflows pushes the workflow list view.
func (a *App) NavigateToWorkflows(namespace string) {
	a.SetNamespace(namespace)
	wl := NewWorkflowList(a, namespace)
	a.app.Pages().Push(wl)
}

// OpenWorkflowPreview shows the selected workflow in the Workflows preview pane.
func (a *App) OpenWorkflowPreview(workflowID, runID string) {
	if a == nil {
		return
	}
	wl, ok := a.currentContent().(*WorkflowList)
	if !ok {
		if a.app == nil || a.app.Pages() == nil {
			return
		}
		wl = NewWorkflowList(a, a.CurrentNamespace())
		a.app.Pages().Push(wl)
	}
	wl.setListKind(listWorkflows)
	wl.revealWorkflow(workflowID, runID)
}

// NavigateToTaskQueues opens the task queues tab on the workflows view.
func (a *App) NavigateToTaskQueues() {
	if current, ok := a.app.Pages().Current().(*WorkflowList); ok {
		current.setListKind(listTaskQueues)
		return
	}
	wl := NewWorkflowList(a, a.CurrentNamespace())
	a.app.Pages().Push(wl)
	wl.setListKind(listTaskQueues)
}

// NavigateToSchedules opens the schedules tab on the workflows view.
func (a *App) NavigateToSchedules() {
	if current, ok := a.app.Pages().Current().(*WorkflowList); ok {
		current.setListKind(listSchedules)
		return
	}
	wl := NewWorkflowList(a, a.CurrentNamespace())
	a.app.Pages().Push(wl)
	wl.setListKind(listSchedules)
}

// NavigateToWorkers opens the workers tab on the workflows view.
func (a *App) NavigateToWorkers() {
	if current, ok := a.app.Pages().Current().(*WorkflowList); ok {
		current.setListKind(listWorkers)
		return
	}
	wl := NewWorkflowList(a, a.CurrentNamespace())
	a.app.Pages().Push(wl)
	wl.setListKind(listWorkers)
}

// NavigateToNamespaceDetail pushes the namespace detail view.
func (a *App) NavigateToNamespaceDetail(namespace string) {
	nd := NewNamespaceDetail(a, namespace)
	a.app.Pages().Push(nd)
}

// Run starts the application.
func (a *App) Run() error {
	// Start connection monitor if we have a provider
	a.mu.RLock()
	hasProvider := a.provider != nil
	a.mu.RUnlock()

	if hasProvider && a.stopMonitor != nil {
		go a.connectionMonitor()
	}

	a.startConfigWatch()

	// Check for updates if enabled
	if a.config != nil && a.config.ShouldCheckUpdates() {
		go a.checkForUpdates()
	}

	return a.app.Run()
}

// checkForUpdates checks for updates and automatically applies them.
func (a *App) checkForUpdates() {
	defer func() { _ = recover() }()
	// Skip auto-update for Homebrew installs - use `brew upgrade` instead
	if update.IsHomebrewInstall() {
		return
	}

	updater := update.NewUpdater()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := updater.CheckForUpdate(ctx)
	if err != nil {
		// Silent failure - don't bother user with update check errors
		return
	}

	if !info.NeedsUpdate {
		return
	}

	// Apply update automatically
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer updateCancel()

	if err := updater.ApplyUpdate(updateCtx, info); err != nil {
		return
	}

	a.app.QueueUpdateDraw(func() {
		a.ToastSuccess("Updated, restart plz " + theme.IconHeart)
	})
}

const statusMessageHold = 3 * time.Second

func (a *App) queueStatusMessage(message string) {
	if a == nil || a.app == nil {
		a.setStatusMessage(message)
		return
	}
	go a.app.QueueUpdateDraw(func() {
		a.setStatusMessage(message)
	})
}

func (a *App) setStatusMessage(message string) {
	if a == nil {
		return
	}
	a.statusMu.Lock()
	a.statusText = message
	if a.statusClear != nil {
		a.statusClear.Stop()
		a.statusClear = nil
	}
	if message != "" {
		a.statusClear = time.AfterFunc(statusMessageHold, func() {
			a.statusMu.Lock()
			a.statusText = ""
			a.statusClear = nil
			a.statusMu.Unlock()
			if a.app == nil {
				a.paintHintStatus()
				return
			}
			go a.app.QueueUpdateDraw(func() {
				a.paintHintStatus()
			})
		})
	}
	a.statusMu.Unlock()
	a.paintHintStatus()
}

func (a *App) paintHintStatus() {}

func (a *App) drawHintLoading(screen tcell.Screen) {
	if a == nil || a.menu == nil || screen == nil || a.promptActive() {
		return
	}
	if a.hintBarMessage() != "" {
		return
	}
	label := a.connectionLabel
	if label == "" {
		return
	}
	x, y, width, height := a.menu.GetInnerRect()
	if width < 1 || height < 1 {
		return
	}
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	for col := x; col < x+width; col++ {
		screen.SetContent(col, y, ' ', nil, style)
	}
	col := x + 1
	for _, r := range label {
		if col >= x+width-1 {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col++
	}
}

func (a *App) drawHintStatus(screen tcell.Screen) {
	if a == nil || a.menu == nil || screen == nil {
		return
	}
	a.statusMu.Lock()
	text := a.statusText
	a.statusMu.Unlock()
	if text == "" {
		return
	}
	x, y, width, height := a.menu.GetInnerRect()
	if width < 1 || height < 1 {
		return
	}
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	for col := x; col < x+width; col++ {
		screen.SetContent(col, y, ' ', nil, style)
	}
	col := x + 1
	for _, r := range text {
		if col >= x+width-1 {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col++
	}
}

func (a *App) hintBarMessage() string {
	if a == nil {
		return ""
	}
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	return a.statusText
}

func (a *App) ShowToastError(message string) {
	a.queueStatusMessage(message)
}

func (a *App) ShowToastWarning(message string) {
	a.queueStatusMessage(message)
}

func (a *App) ShowToastSuccess(message string) {
	a.queueStatusMessage(message)
}

func (a *App) ToastSuccess(message string) {
	a.setStatusMessage(message)
}

func (a *App) ToastError(message string) {
	a.setStatusMessage(message)
}

func (a *App) ToastWarning(message string) {
	a.setStatusMessage(message)
}

func (a *App) ToastInfo(message string) {
	a.setStatusMessage(message)
}

// toggleMouse flips terminal mouse reporting. Turning it off hands clicks and
// drags back to the terminal, so text can be selected and copied natively.
func (a *App) toggleMouse() bool {
	a.mouseEnabled = !a.mouseEnabled
	if a.app != nil {
		setAppMouse(a.app.GetApplication(), a.mouseEnabled)
	}
	a.ToastInfo(mouseToggleMessage(a.mouseEnabled))
	return a.mouseEnabled
}

// connectionMonitor periodically checks the connection and attempts reconnection if needed.
func (a *App) connectionMonitor() {
	ticker := time.NewTicker(connectionCheckInterval)
	defer ticker.Stop()

	backoff := reconnectInitialBackoff

	for {
		select {
		case <-a.stopMonitor:
			return
		case <-ticker.C:
			// Get provider with lock
			a.mu.RLock()
			provider := a.provider
			a.mu.RUnlock()

			if provider == nil {
				continue
			}

			// Check connection
			ctx, cancel := context.WithTimeout(context.Background(), connectionCheckTimeout)
			err := provider.CheckConnection(ctx)
			cancel()

			if err != nil {
				// Connection lost - update UI
				a.app.QueueUpdateDraw(func() {
					a.setConnected(false)
				})

				// Attempt reconnection with backoff
				a.mu.Lock()
				shouldReconnect := !a.reconnecting
				if shouldReconnect {
					a.reconnecting = true
				}
				a.mu.Unlock()

				if shouldReconnect {
					go a.attemptReconnect(backoff)
					backoff = backoff * 2
					if backoff > reconnectMaxBackoff {
						backoff = reconnectMaxBackoff
					}
				}
			} else {
				// Connection is good - reset backoff
				backoff = reconnectInitialBackoff
				a.mu.Lock()
				a.reconnecting = false
				a.mu.Unlock()

				// Ensure UI shows connected
				a.app.QueueUpdateDraw(func() {
					a.setConnected(true)
				})
			}
		}
	}
}

// attemptReconnect tries to reconnect to the Temporal server.
func (a *App) attemptReconnect(backoff time.Duration) {
	select {
	case <-a.stopMonitor:
		return
	case <-time.After(backoff):
	}

	// Get provider with lock
	a.mu.RLock()
	provider := a.provider
	a.mu.RUnlock()

	if provider == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := provider.Reconnect(ctx)
	cancel()

	// Update reconnecting state before QueueUpdateDraw to avoid deadlock
	if err == nil {
		a.mu.Lock()
		a.reconnecting = false
		a.mu.Unlock()
	}

	a.app.QueueUpdateDraw(func() {
		if err == nil {
			a.setConnected(true)
			a.refreshNamespaceCatalog()
		}
	})
}

// Stop stops the application and connection monitor.
func (a *App) Stop() {
	if a.stopMonitor != nil {
		select {
		case <-a.stopMonitor:
		default:
			close(a.stopMonitor)
		}
	}
	if a.stopConfigWatch != nil {
		select {
		case <-a.stopConfigWatch:
		default:
			close(a.stopConfigWatch)
		}
	}
	a.app.Stop()
}

// SetDevMode enables or disables development mode.
func (a *App) SetDevMode(enabled bool) {
	a.devMode = enabled
}

// showSplashTest shows the splash screen for testing gradients and themes.
func (a *App) showSplashTest() {
	currentTheme := "tokyonight-night"
	if a.config != nil && a.config.Theme != "" {
		currentTheme = a.config.Theme
	}

	splash := NewSplashTestView(currentTheme)
	splash.SetOnClose(func() {
		a.closeSplashTest()
	})
	splash.SetOnThemeChange(func(themeName string) {
		// Update config with new theme
		if a.config != nil {
			a.config.Theme = themeName
		}
		// Refresh theme colors across the app
		a.app.RefreshTheme()
	})

	a.app.Pages().AddPage("splash-test", splash, true, true)
	a.app.SetFocus(splash)
}

func (a *App) closeSplashTest() {
	a.app.Pages().RemovePage("splash-test")
	if current := a.app.Pages().Current(); current != nil {
		a.app.SetFocus(current)
	}
}

func (a *App) timelineHasFocus() bool {
	if a == nil || a.app == nil {
		return false
	}
	if current := a.app.Pages().Current(); current != nil {
		if v, ok := current.(*WorkflowList); ok {
			return v.focusPane == focusTimeline
		}
	}
	if tviewApp := a.app.GetApplication(); tviewApp != nil {
		_, ok := tviewApp.GetFocus().(*TimelineView)
		return ok
	}
	return false
}

func (a *App) modalHasFocus() bool {
	return a != nil && a.app != nil && a.app.Pages() != nil && a.app.Pages().CurrentIsModal()
}

func (a *App) handleQuestionMark() {
	if a == nil || a.app == nil || a.app.Pages() == nil {
		return
	}
	current := a.app.Pages().Current()
	if helpModalOf(current) != nil {
		a.closeHelp()
		return
	}
	if a.app.Pages().CurrentIsModal() {
		return
	}
	a.showHelp()
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

func (a *App) showHintSheet() {
	// Gather hints: global + current view
	allHints := []components.KeyHint{
		{Key: "?", Description: "Help"},
		{Key: "T", Description: "Theme"},
		{Key: "P", Description: "Profile"},
		{Key: "Ctrl+O", Description: "Mouse on/off"},
		{Key: "Esc", Description: "Back"},
		{Key: "q", Description: "Quit"},
	}

	current := a.app.Pages().Current()
	if current != nil {
		viewHints := current.Hints()
		if len(viewHints) > 0 {
			allHints = append(allHints, viewHints...)
		}
	}

	// Create hint grid and calculate height
	grid := components.NewHintGrid()
	grid.SetHints(allHints)

	// Estimate width for height calculation (use a reasonable default).
	// The actual width will be available at draw time, but we need an estimate
	// for the sheet height. Use 80 as a conservative estimate; the grid reflows on draw.
	estimatedWidth := 80
	gridHeight := grid.GetPreferredHeight(estimatedWidth)
	// Panel border (2) + hint bar (1) = 3 lines of overhead
	sheetHeight := gridHeight + 3

	sheet := components.NewBottomSheet(components.BottomSheetConfig{
		Title:    "Keybindings",
		Height:   sheetHeight,
		Backdrop: false,
	})

	sheet.SetContent(grid)
	sheet.SetHints([]components.KeyHint{
		{Key: "Esc/?", Description: "Close"},
	})

	sheet.SetOnClose(func() {
		a.app.Pages().DismissModal()
	})

	// Wrap input to also dismiss on '?' (toggle behavior)
	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == '?' {
			a.app.Pages().DismissModal()
			return nil
		}
		return event
	})

	a.PushModal(sheet)
	a.app.SetFocus(sheet)
}

func (a *App) closeThemeSelector() {
	a.app.Pages().DismissModal()
}

func (a *App) showDebugScreen() {
	// Build debug data from current app state
	data := DebugData{
		Version:     update.Version,
		Commit:      update.Commit,
		BuildDate:   update.BuildDate,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		GoVersion:   runtime.Version(),
		Term:        os.Getenv("TERM"),
		ColorTerm:   os.Getenv("COLORTERM"),
		TermProgram: os.Getenv("TERM_PROGRAM"),
		ConfigPath:  config.ConfigPath(),
		ThemeName:   a.config.Theme,
		ProfileName: a.activeProfile,
	}

	// Get profile connection details
	if profile, ok := a.config.GetProfile(a.activeProfile); ok {
		data.ServerAddress = profile.Address
		data.Namespace = profile.Namespace
		data.TLSEnabled = profile.TLS.Cert != "" || profile.TLS.CA != ""
		data.TLSCertPath = profile.TLS.Cert
		data.TLSKeyPath = profile.TLS.Key
		data.TLSCAPath = profile.TLS.CA
	}

	// Detect color space from environment
	colorTerm := os.Getenv("COLORTERM")
	term := os.Getenv("TERM")
	switch {
	case colorTerm == "truecolor" || colorTerm == "24bit":
		data.ColorSpace = "truecolor (24-bit)"
	case strings.Contains(term, "256color"):
		data.ColorSpace = "256 colors"
	default:
		data.ColorSpace = "unknown"
	}

	// Create and push debug screen
	debugScreen := NewDebugScreen(data)

	// Wire up yank keybindings (in standalone mode these live on DebugApp)
	debugScreen.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'y':
			report := debugScreen.GeneratePlainReport()
			if err := copyToClipboard(report); err != nil {
				a.ToastError("Failed to copy: " + err.Error())
			} else {
				a.ToastSuccess("Report copied to clipboard!")
			}
			return nil
		case 'Y':
			tmpl := debugScreen.GenerateIssueTemplate()
			if err := copyToClipboard(tmpl); err != nil {
				a.ToastError("Failed to copy: " + err.Error())
			} else {
				a.ToastSuccess("Issue template copied to clipboard!")
			}
			return nil
		}
		return event
	})

	a.app.Pages().Push(debugScreen)
}

func (a *App) showCommandBar() {
	p := a.prompt()
	p.suggestFn = func(input string) []commandSuggestion {
		return suggestionsFor(input, a.commandCatalog(), a.commandExtras())
	}
	p.input.SetChangedFunc(func(string) {
		if p.applying {
			return
		}
		p.history.resetBrowse()
		p.refreshSuggestions()
	})
	a.enterPrompt(": ", "command...")
	p.refreshSuggestions()
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
	allThemes := config.ThemeNames()
	var darkThemes, lightThemes []string
	for _, name := range allThemes {
		if t, ok := config.BuiltinThemes[name]; ok {
			if t.Type == "light" {
				lightThemes = append(lightThemes, name)
			} else {
				darkThemes = append(darkThemes, name)
			}
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

	// Find current theme index for marker
	currentIdx := -1
	for i, name := range allThemes {
		if name == currentTheme {
			currentIdx = i
			break
		}
	}

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
	if currentIdx >= 0 {
		// Find it in the correct section
		for idx, themeName := range listToTheme {
			if themeName == currentTheme {
				currentListIdx = idx
				break
			}
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

// refreshCurrentView calls RefreshTheme on the current view if it supports it.
//
// Deprecated: As of jig v0.0.6, theme.SetProvider() automatically calls RefreshTheme()
// on all registered Refreshable components and triggers a redraw. This method is no
// longer needed for theme switching. Kept for backwards compatibility.
func (a *App) refreshCurrentView() {
	if current := a.app.Pages().Current(); current != nil {
		if refreshable, ok := current.(interface{ RefreshTheme() }); ok {
			refreshable.RefreshTheme()
		}
	}
}

// Profile management methods

// ShowProfileSelector opens the profile selector modal.
func (a *App) ShowProfileSelector() {
	if a.config == nil {
		return
	}

	modal := NewProfileModal()
	modal.SetProfiles(a.config.ListProfiles(), a.activeProfile, a.config)
	modal.SetOnSelect(func(name string) {
		a.closeProfileSelector()
		a.SwitchProfile(name)
	})
	modal.SetOnNew(func() {
		a.closeProfileSelector()
		a.showProfileForm("")
	})
	modal.SetOnEdit(func(name string) {
		a.closeProfileSelector()
		a.showProfileForm(name)
	})
	modal.SetOnDelete(func(name string) {
		a.deleteProfile(name)
		modal.SetProfiles(a.config.ListProfiles(), a.activeProfile, a.config)
	})
	modal.SetOnClose(func() {
		a.closeProfileSelector()
	})

	a.PushModal(modal)
	a.app.SetFocus(modal)
}

func (a *App) closeProfileSelector() {
	a.app.Pages().DismissModal()
}

func (a *App) showProfileForm(editName string) {
	form := NewProfileForm()

	if editName != "" {
		if cfg, ok := a.config.GetProfile(editName); ok {
			form.SetProfile(editName, cfg)
		}
	}

	form.SetOnSave(func(name string, cfg config.ConnectionConfig) {
		a.closeProfileForm()
		if err := a.config.SaveProfile(name, cfg); err != nil {
			// Log error but continue
			return
		}
		if err := a.SaveConfig(); err != nil {
			// Log error but continue
		}
		a.SwitchProfile(name)
	})
	form.SetOnCancel(func() {
		a.closeProfileForm()
	})

	a.PushModal(form)
	a.app.SetFocus(form)
}

func (a *App) closeProfileForm() {
	a.app.Pages().DismissModal()
}

func (a *App) deleteProfile(name string) {
	if a.config == nil {
		return
	}
	if err := a.config.DeleteProfile(name); err != nil {
		return
	}
	_ = a.SaveConfig()
}

// SaveConfig persists the running config when autosave is on, unless the file
// on disk could not be read: overwriting it then would replace the user's
// settings with defaults.
func (a *App) SaveConfig() error {
	if a == nil || a.config == nil {
		return nil
	}
	if !a.config.ShouldAutosave() {
		return nil
	}
	if a.configUnreadable {
		a.ToastError("Not saving: " + config.ConfigPath() + " could not be read")
		return nil
	}
	return a.config.Save()
}

// MarkConfigUnreadable records that the config file could not be parsed.
func (a *App) MarkConfigUnreadable() {
	a.configUnreadable = true
}

// SwitchProfile switches to a different connection profile.
func (a *App) SwitchProfile(name string) {
	a.applyProfile(name, true)
}

func (a *App) applyProfile(name string, persist bool) {
	a.mu.RLock()
	provider := a.provider
	currentProfile := a.activeProfile
	a.mu.RUnlock()

	if a.config == nil || provider == nil {
		return
	}

	profileCfg, ok := a.config.GetProfile(name)
	if !ok {
		return
	}
	if err := profileCfg.CloudAPIKeyError(); err != nil {
		a.ToastError(err.Error())
		return
	}
	profileCfg = profileCfg.ExpandEnv()

	connConfig := temporal.ConnectionConfig{
		Address:       profileCfg.Address,
		Namespace:     profileCfg.Namespace,
		TLSCertPath:   profileCfg.TLS.Cert,
		TLSKeyPath:    profileCfg.TLS.Key,
		TLSCAPath:     profileCfg.TLS.CA,
		TLSServerName: profileCfg.TLS.ServerName,
		TLSSkipVerify: profileCfg.TLS.SkipVerify,
		APIKey:        profileCfg.APIKey,
		GRPCMeta:      profileCfg.GRPCMeta,
		CodecEndpoint: profileCfg.CodecEndpoint,
	}

	// QueueUpdateDraw from the UI thread deadlocks (Enter in the profile modal).
	go func() {
		a.app.QueueUpdateDraw(func() {
			if current := a.app.Pages().Current(); current != nil {
				current.Stop()
			}
			a.setProfile(name + " (connecting...)")
			a.setConnected(false)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := provider.ReconnectWithConfig(ctx, connConfig)
		cancel()

		if err == nil {
			a.mu.Lock()
			a.activeProfile = name
			a.currentNS = connConfig.Namespace
			a.mu.Unlock()

			a.config.SetActiveProfile(name)
			if persist {
				_ = a.SaveConfig()
			}
		}

		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.setProfile(currentProfile + " (failed)")
				a.setConnected(false)
				return
			}

			a.setProfile(name)
			a.setConnected(true)
			a.setNamespace(connConfig.Namespace)
			a.resetNamespaceCatalog()

			a.reinitializeViews()
		})
	}()
}

// reinitializeViews resets the view stack after a profile switch.
func (a *App) reinitializeViews() {
	a.app.Pages().Clear()

	// If a namespace is defined in the connection, skip namespace list and go directly to workflows
	if a.provider != nil && a.provider.Config().Namespace != "" {
		wl := NewWorkflowList(a, a.currentNS)
		a.app.Pages().Push(wl)
		a.app.SetFocus(wl)
	} else {
		a.namespaceList = NewNamespaceList(a)
		a.app.Pages().Push(a.namespaceList)
		a.app.SetFocus(a.namespaceList)
	}
}

func (a *App) handleProfileCommand(args string) {
	args = strings.TrimSpace(args)

	if args == "" {
		a.ShowProfileSelector()
		return
	}

	parts := strings.Fields(args)
	cmd := parts[0]

	switch cmd {
	case "new":
		a.showProfileForm("")
	case "edit":
		if len(parts) > 1 {
			a.showProfileForm(parts[1])
		} else {
			a.showProfileForm(a.activeProfile)
		}
	case "delete":
		if len(parts) > 1 {
			a.deleteProfile(parts[1])
		}
	case "save":
		a.showProfileForm("")
	default:
		if a.config != nil && a.config.ProfileExists(cmd) {
			a.SwitchProfile(cmd)
		}
	}
}

// ActiveProfile returns the currently active profile name.
func (a *App) ActiveProfile() string {
	return a.activeProfile
}

// Config returns the app configuration.
func (a *App) Config() *config.Config {
	return a.config
}

// OpenWorkflowInBrowser opens the workflow run in the Temporal Web UI.
func (a *App) OpenWorkflowInBrowser(workflowID, runID string) {
	if a == nil {
		return
	}
	link, err := a.workflowUILink(workflowID, runID)
	if err != nil {
		a.ToastError(err.Error())
		return
	}
	if err := openBrowser(link); err != nil {
		a.ToastError("Failed to open browser: " + err.Error())
		return
	}
	a.ToastSuccess("Opened in browser")
}

func (a *App) workflowUILink(workflowID, runID string) (string, error) {
	if workflowID == "" {
		return "", fmt.Errorf("No workflow selected")
	}
	profile := a.uiProfile()
	base := profile.ResolveUIURL()
	if base == "" {
		return "", fmt.Errorf("Set ui_url on this profile to open the Web UI")
	}
	namespace := a.CurrentNamespace()
	if namespace == "" {
		namespace = profile.Namespace
	}
	link := config.WorkflowUIURL(base, namespace, workflowID, runID)
	if link == "" {
		return "", fmt.Errorf("Could not build the Web UI URL")
	}
	return link, nil
}

func (a *App) uiProfile() config.ConnectionConfig {
	if a == nil {
		return config.ConnectionConfig{}
	}
	if a.config != nil {
		if a.activeProfile != "" {
			if profile, ok := a.config.GetProfile(a.activeProfile); ok {
				return profile
			}
		}
		_, profile := a.config.GetActiveProfile()
		return profile
	}
	if a.provider != nil {
		tc := a.provider.Config()
		return config.ConnectionConfig{Address: tc.Address, Namespace: tc.Namespace}
	}
	return config.ConnectionConfig{}
}

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

// restoreDefaultCommandCallbacks restores the default command bar callbacks.
func (a *App) restoreDefaultCommandCallbacks() {
	p := a.prompt()
	p.input.SetChangedFunc(nil)
	p.onComplete = nil
	p.suggestFn = nil
	p.suggestions.clear()
	p.onSubmit = func(text string) {
		a.recordCommand(text)
		a.exitPrompt()
		a.handleCommandInput(text)
	}
	p.onCancel = func() {
		a.exitPrompt()
	}
}

// CommandContextProvider is implemented by views that can provide workflow context for commands.
type CommandContextProvider interface {
	CommandContext() (workflowID, runID, workflowType string)
}

// handleCommandInput processes command bar input, dispatching to built-in or user commands.
func (a *App) handleCommandInput(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		if current := a.app.Pages().Current(); current != nil {
			a.app.SetFocus(current)
		}
		return
	}

	fields := strings.Fields(text)
	if a.executeBuiltinCommand(fields) {
		if a.app != nil && a.app.Pages() != nil && !a.app.Pages().CurrentIsModal() {
			a.refocusCurrent()
		}
		return
	}

	cmdName := fields[0]
	args := fields[1:]
	if a.config != nil {
		commands := a.config.GetMergedCommands(a.activeProfile)
		if cfg, ok := commands[cmdName]; ok {
			a.executeUserCommand(cmdName, cfg, args)
			return
		}
	}

	a.ToastWarning(fmt.Sprintf("Unknown command: %s", cmdName))
	a.refocusCurrent()
}

// buildCommandContext resolves all context variables from current app state.
func (a *App) buildCommandContext(args []string) command.Context {
	ctx := command.Context{
		Namespace: a.CurrentNamespace(),
		Profile:   a.activeProfile,
		Args:      args,
	}

	// Get connection details from active profile config
	if a.config != nil {
		if profile, ok := a.config.GetProfile(a.activeProfile); ok {
			expanded := profile.ExpandEnv()
			ctx.Address = expanded.Address
			ctx.TLSCertPath = expanded.TLS.Cert
			ctx.TLSKeyPath = expanded.TLS.Key
			ctx.TLSCAPath = expanded.TLS.CA
			ctx.TLSServerName = expanded.TLS.ServerName
			ctx.TLSSkipVerify = expanded.TLS.SkipVerify
			ctx.APIKey = expanded.APIKey
			ctx.CodecEndpoint = expanded.CodecEndpoint
		}
	}

	// Get workflow info from current view if it implements CommandContextProvider.
	// Walk the stack from top to bottom to find the first provider with data.
	stack := a.app.Pages().GetStack()
	for i := len(stack) - 1; i >= 0; i-- {
		if provider, ok := stack[i].(CommandContextProvider); ok {
			wfID, rID, wfType := provider.CommandContext()
			if wfID != "" {
				ctx.WorkflowID = wfID
				ctx.RunID = rID
				ctx.WorkflowType = wfType
				break
			}
		}
	}

	return ctx
}

// refocusCurrent restores focus to the current view.
func (a *App) refocusCurrent() {
	if current := a.app.Pages().Current(); current != nil {
		a.app.SetFocus(current)
	}
}

// executeUserCommand expands the command template, optionally confirms, then runs it.
func (a *App) executeUserCommand(name string, cfg config.CommandConfig, args []string) {
	cmdCtx := a.buildCommandContext(args)

	expandedCmd, err := command.ExpandCmd(cfg.Cmd, cmdCtx)
	if err != nil {
		a.ToastError(fmt.Sprintf("Command %q: %s", name, err))
		a.refocusCurrent()
		return
	}

	// Inject connection flags for temporal CLI commands
	expandedCmd = command.InjectConnectionFlags(expandedCmd, cmdCtx)

	if cfg.Confirm {
		a.showCommandConfirm(name, cfg, expandedCmd)
		return
	}

	a.runCommand(name, expandedCmd, cfg)
}

// showCommandConfirm shows a confirmation modal before executing a command.
func (a *App) showCommandConfirm(name string, cfg config.CommandConfig, expandedCmd string) {
	form := components.NewFormBuilder().
		OnSubmit(func(values map[string]any) {
			a.app.Pages().DismissModal()
			a.runCommand(name, expandedCmd, cfg)
		}).
		OnCancel(func() {
			a.app.Pages().DismissModal()
			if current := a.app.Pages().Current(); current != nil {
				a.app.SetFocus(current)
			}
		}).
		Build()

	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.SetBackgroundColor(theme.Bg())

	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	infoText.SetBackgroundColor(theme.Bg())
	infoText.SetText(fmt.Sprintf("[%s]Command:[-] [%s]%s[-]\n\n[%s]%s[-]",
		theme.TagFgDim(), theme.TagAccent(), name,
		theme.TagFg(), expandedCmd))

	contentFlex.AddItem(infoText, 4, 0, false)
	contentFlex.AddItem(form, 0, 1, true)

	title := fmt.Sprintf("Confirm: %s", name)
	if cfg.Description != "" {
		title = fmt.Sprintf("Confirm: %s", cfg.Description)
	}

	modal := newModal(components.ModalConfig{
		Title:    title,
		Width:    70,
		Height:   12,
		Backdrop: true,
	})
	modal.SetContent(contentFlex)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
	})

	a.PushModal(modal)
	a.app.SetFocus(form)
}

// runCommand dispatches to the appropriate output handler based on config.
func (a *App) runCommand(name, expandedCmd string, cfg config.CommandConfig) {
	outputType := cfg.Output
	if outputType == "" {
		outputType = config.OutputLog
	}

	switch outputType {
	case config.OutputLog:
		a.runCommandLog(name, expandedCmd, cfg)
	case config.OutputJSON:
		a.runCommandJSON(name, expandedCmd, cfg)
	case config.OutputWorkflows:
		a.runCommandWorkflows(name, expandedCmd, cfg)
	case config.OutputWorkflow:
		a.runCommandWorkflow(name, expandedCmd, cfg)
	default:
		a.runCommandLog(name, expandedCmd, cfg)
	}
}

// runCommandLog creates a CommandOutputView with LogViewer, pushes it, and streams output.
func (a *App) runCommandLog(name, expandedCmd string, cfg config.CommandConfig) {
	ctx, cancel := context.WithCancel(context.Background())

	lv := components.NewLogViewer()
	description := cfg.Description
	if description == "" {
		description = name
	}
	view := NewCommandOutputView(a, name, description, lv, cancel)

	a.app.Pages().Push(view)
	a.app.SetFocus(lv)

	go func() {
		lv.AddEntry(components.LogEntry{
			Level:   components.LogLevelInfo,
			Message: "$ " + expandedCmd,
		})

		err := command.RunStreaming(ctx, expandedCmd, func(line string) {
			a.app.QueueUpdateDraw(func() {
				lv.AddEntry(components.LogEntry{
					Level:   components.LogLevelInfo,
					Message: line,
				})
			})
		})

		a.app.QueueUpdateDraw(func() {
			if err != nil && ctx.Err() == nil {
				lv.AddEntry(components.LogEntry{
					Level:   components.LogLevelError,
					Message: fmt.Sprintf("Error: %s", err),
				})
			} else {
				lv.AddEntry(components.LogEntry{
					Level:   components.LogLevelInfo,
					Message: "--- Done ---",
				})
			}
		})
	}()
}

// runCommandJSON runs a command and displays the result in a CodeView with JSON highlighting.
func (a *App) runCommandJSON(name, expandedCmd string, cfg config.CommandConfig) {
	ctx, cancel := context.WithCancel(context.Background())

	cv := components.NewCodeView().SetLanguage(components.LangJSON)
	cv.SetCode("Running...")

	description := cfg.Description
	if description == "" {
		description = name
	}
	view := NewCommandOutputView(a, name, description, cv, cancel)

	a.app.Pages().Push(view)
	a.app.SetFocus(cv)

	go func() {
		output, err := command.Run(ctx, expandedCmd)
		a.app.QueueUpdateDraw(func() {
			if err != nil && ctx.Err() == nil {
				cv.SetCode(fmt.Sprintf("Error: %s\n\n%s", err, output))
			} else {
				// Try to pretty-print JSON
				formatted := formatJSONPretty(output)
				cv.SetCode(formatted)
			}
		})
	}()
}

// runCommandWorkflows runs a command, parses JSONL output, and pushes a WorkflowList.
func (a *App) runCommandWorkflows(name, expandedCmd string, _ config.CommandConfig) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, runErr := command.Run(ctx, expandedCmd)

		// Try to parse output even on non-zero exit (CLI may still produce valid JSONL)
		workflows, parseErr := command.ParseWorkflowsOutput(output)
		if parseErr != nil {
			// If both run and parse failed, show the run error (more useful)
			errMsg := parseErr.Error()
			if runErr != nil {
				errMsg = runErr.Error()
			}
			a.app.QueueUpdateDraw(func() {
				a.ToastError(fmt.Sprintf("Command %q: %s", name, errMsg))
				a.refocusCurrent()
			})
			return
		}

		a.app.QueueUpdateDraw(func() {
			wl := NewWorkflowListWithData(a, a.CurrentNamespace(), workflows)
			a.app.Pages().Push(wl)
			a.app.SetFocus(wl)
		})
	}()
}

// runCommandWorkflow runs a command, parses workflow ID/run ID, and opens Preview.
func (a *App) runCommandWorkflow(name, expandedCmd string, _ config.CommandConfig) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, runErr := command.Run(ctx, expandedCmd)

		workflowID, runID, parseErr := command.ParseWorkflowOutput(output)
		if parseErr != nil {
			errMsg := parseErr.Error()
			if runErr != nil {
				errMsg = runErr.Error()
			}
			a.app.QueueUpdateDraw(func() {
				a.ToastError(fmt.Sprintf("Command %q: %s", name, errMsg))
				a.refocusCurrent()
			})
			return
		}

		a.app.QueueUpdateDraw(func() {
			a.OpenWorkflowPreview(workflowID, runID)
		})
	}()
}

// EscapeHandler is implemented by views that want to handle escape key.
type EscapeHandler interface {
	HandleEscape() bool
}

// KeyHint re-exports jig's KeyHint for convenience.
type KeyHint = components.KeyHint
