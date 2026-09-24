package view

import (
	"context"
	"sync"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/layout"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
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
	searchStatus     string
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

// NewAppWithProvider creates a new application controller with a Temporal provider.
func NewAppWithProvider(provider temporal.Provider, defaultNamespace string, cfg *config.Config, activeProfile string) *App {
	a := &App{
		provider:      provider,
		currentNS:     defaultNamespace,
		stopMonitor:   make(chan struct{}),
		config:        cfg,
		activeProfile: activeProfile,
	}
	migrateSavedFilters(cfg)
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

		// Help: pane views open a hint modal; open modals put hints on the
		// bottom bar and leave the key alone so it can be typed into a field.
		if event.Rune() == '?' && a.handleQuestionMark() {
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

	a.installPasteScreen()

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

func (a *App) setIOWrap(on bool) {
	if a == nil {
		return
	}
	if a.config == nil {
		a.config = &config.Config{}
	}
	a.config.IOWrap = &on
	if a.config.ShouldAutosave() {
		_ = a.persistConfig()
	}
	if wl, ok := a.workflowList(); ok {
		wl.applyIOWrap(on)
	}
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

// ActiveProfile returns the currently active profile name.
func (a *App) ActiveProfile() string {
	return a.activeProfile
}

// Config returns the app configuration.
func (a *App) Config() *config.Config {
	return a.config
}

// EscapeHandler is implemented by views that want to handle escape key.
type EscapeHandler interface {
	HandleEscape() bool
}

// KeyHint re-exports jig's KeyHint for convenience.
type KeyHint = components.KeyHint
