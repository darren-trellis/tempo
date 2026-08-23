package view

import (
	"fmt"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type workflowFocusPane int

const (
	focusWorkflows workflowFocusPane = iota
	focusEvents
	focusEventDetail
)

// WorkflowList displays a list of workflows.
type WorkflowList struct {
	*tview.Flex
	app               *App
	namespace         string
	table             *components.Table
	workflowsPanel    *components.Panel
	previewPanel      *components.Panel
	previewTabs       *components.Tabs
	eventTable        *components.Table
	eventDetail       *tview.TextView
	eventsPanel       *components.Panel
	eventDetailPanel  *components.Panel
	workflowDetail    *tview.TextView
	focusPane         workflowFocusPane
	previewKind       previewKind
	previewEvents     []temporal.EnhancedHistoryEvent
	previewActivities []previewActivity
	previewWorkflowID string
	previewRunID      string
	previewGen        uint64
	previewTimer      *time.Timer
	previewMode       bool
	previewCache      *previewCache
	emptyState        *components.EmptyState
	noResultsState    *components.EmptyState
	allWorkflows      []temporal.Workflow // Full unfiltered list
	workflows         []temporal.Workflow // Filtered list for display
	filterText        string
	visibilityQuery   string // Temporal visibility query
	loading           bool
	autoRefresh       bool
	refreshTicker     *time.Ticker
	stopRefresh       chan struct{}
	selectionMode     bool     // Multi-select mode active
	searchHistory     []string // History of visibility queries
	historyIndex      int      // Current position in history (-1 = not browsing)
	maxHistorySize    int      // Maximum number of history entries
	// Server-side completion support
	serverCompletions   []string            // Cached completions from server query
	lastCompletionQuery string              // Last query sent to server (to avoid duplicates)
	originalWorkflows   []temporal.Workflow // Original workflows before server search
	preloaded           bool                // True if workflows were provided at construction time
}

// NewWorkflowList creates a new workflow list view.
func NewWorkflowList(app *App, namespace string) *WorkflowList {
	wl := &WorkflowList{
		Flex:           tview.NewFlex().SetDirection(tview.FlexColumn),
		app:            app,
		namespace:      namespace,
		table:          components.NewTable(),
		workflows:      []temporal.Workflow{},
		stopRefresh:    make(chan struct{}, 1), // Buffered to ensure stop signal isn't lost
		searchHistory:  make([]string, 0, 50),
		historyIndex:   -1,
		maxHistorySize: 50,
		previewKind:    previewActivities,
		previewCache:   newPreviewCache(previewCacheLimit(app)),
	}
	wl.setup()

	// Register for automatic theme refresh
	theme.RegisterRefreshable(wl)

	return wl
}

// NewWorkflowListWithData creates a workflow list pre-populated with data (no server fetch).
func NewWorkflowListWithData(app *App, namespace string, workflows []temporal.Workflow) *WorkflowList {
	wl := &WorkflowList{
		Flex:           tview.NewFlex().SetDirection(tview.FlexColumn),
		app:            app,
		namespace:      namespace,
		table:          components.NewTable(),
		allWorkflows:   workflows,
		workflows:      workflows,
		stopRefresh:    make(chan struct{}, 1),
		searchHistory:  make([]string, 0, 50),
		historyIndex:   -1,
		maxHistorySize: 50,
		previewKind:    previewActivities,
		preloaded:      true,
	}
	wl.setup()

	theme.RegisterRefreshable(wl)
	return wl
}

// CommandContext returns the workflow ID, run ID, and type of the currently selected row.
func (wl *WorkflowList) CommandContext() (workflowID, runID, workflowType string) {
	row := wl.table.SelectedRow()
	if row >= 0 && row < len(wl.workflows) {
		return wl.workflows[row].ID, wl.workflows[row].RunID, wl.workflows[row].Type
	}
	return "", "", ""
}

func (wl *WorkflowList) setup() {
	wl.SetBackgroundColor(theme.Bg())
	wl.table.SetEvaluateAllRows(true)
	wl.table.SetBorder(false)
	wl.table.SetBackgroundColor(theme.Bg())
	applyWorkflowColumnHeaders(wl.table, wl.columnLayout())
	wl.setupPreview()

	wl.workflowsPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Workflows", theme.IconWorkflow))
	wl.workflowsPanel.SetContent(wl.table)

	wl.applyPreviewLayout()

	emptyInputCapture := func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'W':
			wl.showSignalWithStart()
			return nil
		case 'r':
			wl.loadData()
			return nil
		case 't':
			wl.app.NavigateToTaskQueues()
			return nil
		case 's':
			wl.app.NavigateToSchedules()
			return nil
		case 'a':
			wl.toggleAutoRefresh()
			return nil
		case '|':
			wl.showColumnEditor()
			return nil
		case 'p':
			wl.togglePreviewMode()
			return nil
		case '[':
			if wl.previewModeEnabled() {
				wl.cyclePreviewKind(-1)
				return nil
			}
		case ']':
			if wl.previewModeEnabled() {
				wl.cyclePreviewKind(1)
				return nil
			}
		}
		return event
	}

	wl.emptyState = components.NewEmptyState().
		SetIcon(theme.IconInfo).
		SetTitle("No Workflows").
		SetMessage("No workflows found in this namespace")
	wl.emptyState.SetInputCapture(emptyInputCapture)

	wl.noResultsState = components.NewEmptyState().
		SetIcon(theme.IconSearch).
		SetTitle("No Results").
		SetMessage("No workflows match the current filter")
	wl.noResultsState.SetInputCapture(emptyInputCapture)

	wl.clearPreview()

	wl.table.SetSelectionChangedFunc(func(row, col int) {
		if wl.previewModeEnabled() && row > 0 && row-1 < len(wl.workflows) {
			wl.schedulePreview(wl.workflows[row-1], false)
		}
	})

	wl.table.SetOnSelect(func(row int) {
		wl.activateSelectedWorkflow()
	})
}

// RefreshTheme updates all component colors after a theme change.
func (wl *WorkflowList) RefreshTheme() {
	bg := theme.Bg()
	wl.SetBackgroundColor(bg)
	wl.table.SetBackgroundColor(bg)
	wl.eventTable.SetBackgroundColor(bg)
	wl.eventDetail.SetBackgroundColor(bg)
	wl.eventDetail.SetTextColor(theme.Fg())
	if wl.workflowDetail != nil {
		wl.workflowDetail.SetBackgroundColor(bg)
		wl.workflowDetail.SetTextColor(theme.Fg())
	}
	if wl.previewPanel != nil {
		wl.previewPanel.SetBackgroundColor(bg)
	}
	wl.populateTable()
	wl.applyFocusStyles()
}

func (wl *WorkflowList) SetMasterTitle(title string) {
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetTitle(title)
	}
}

func (wl *WorkflowList) SetMasterContent(content tview.Primitive) {
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetContent(content)
	}
}

// Name returns the view name.
func (wl *WorkflowList) Name() string {
	return "workflows"
}

// Start is called when the view becomes active.
func (wl *WorkflowList) Start() {
	bindings := input.NewKeyBindings().
		OnRune(' ', func(e *tcell.EventKey) bool {
			if wl.selectionMode {
				wl.table.ToggleSelection()
				wl.updateSelectionPreview()
				return true
			}
			return false
		}).
		OnRune('/', func(e *tcell.EventKey) bool {
			wl.showFilter()
			return true
		}).
		OnRune('F', func(e *tcell.EventKey) bool {
			wl.showVisibilityQuery()
			return true
		}).
		OnRune('f', func(e *tcell.EventKey) bool {
			wl.showQueryTemplates()
			return true
		}).
		OnRune('D', func(e *tcell.EventKey) bool {
			wl.showDateRangePicker()
			return true
		}).
		OnRune('t', func(e *tcell.EventKey) bool {
			wl.app.NavigateToTaskQueues()
			return true
		}).
		OnRune('s', func(e *tcell.EventKey) bool {
			wl.app.NavigateToSchedules()
			return true
		}).
		OnRune('a', func(e *tcell.EventKey) bool {
			wl.toggleAutoRefresh()
			return true
		}).
		OnRune('r', func(e *tcell.EventKey) bool {
			wl.loadData()
			return true
		}).
		OnRune('y', func(e *tcell.EventKey) bool {
			wl.copyWorkflowID()
			return true
		}).
		OnRune('v', func(e *tcell.EventKey) bool {
			wl.toggleSelectionMode()
			return true
		}).
		OnRune('c', func(e *tcell.EventKey) bool {
			if wl.selectionMode && len(wl.table.GetSelectedRows()) > 0 {
				wl.showBatchCancelConfirm()
				return true
			}
			return false
		}).
		OnRune('X', func(e *tcell.EventKey) bool {
			if wl.selectionMode && len(wl.table.GetSelectedRows()) > 0 {
				wl.showBatchTerminateConfirm()
				return true
			}
			return false
		}).
		OnRune('C', func(e *tcell.EventKey) bool {
			if wl.visibilityQuery != "" {
				wl.clearVisibilityQuery()
				return true
			}
			return false
		}).
		OnRune('L', func(e *tcell.EventKey) bool {
			wl.showSavedFilters()
			return true
		}).
		OnRune('S', func(e *tcell.EventKey) bool {
			if wl.visibilityQuery != "" {
				wl.showSaveFilter()
				return true
			}
			return false
		}).
		OnRune('N', func(e *tcell.EventKey) bool {
			wl.showStartWorkflow()
			return true
		}).
		OnRune('W', func(e *tcell.EventKey) bool {
			wl.showSignalWithStart()
			return true
		}).
		OnRune('d', func(e *tcell.EventKey) bool {
			wl.startDiff()
			return true
		}).
		OnCtrlRune('a', func(e *tcell.EventKey) bool {
			if wl.selectionMode {
				wl.table.SelectAll()
				wl.updateSelectionPreview()
				return true
			}
			return false
		}).
		OnRune('o', func(e *tcell.EventKey) bool {
			wl.showWorkflowGraph()
			return true
		}).
		OnRune('|', func(e *tcell.EventKey) bool {
			wl.showColumnEditor()
			return true
		}).
		OnRune('p', func(e *tcell.EventKey) bool {
			wl.togglePreviewMode()
			return true
		}).
		OnRune('[', func(e *tcell.EventKey) bool {
			if !wl.previewModeEnabled() {
				return false
			}
			wl.cyclePreviewKind(-1)
			return true
		}).
		OnRune(']', func(e *tcell.EventKey) bool {
			if !wl.previewModeEnabled() {
				return false
			}
			wl.cyclePreviewKind(1)
			return true
		}).
		OnRune('i', func(e *tcell.EventKey) bool {
			return wl.showPreviewIO()
		}).
		OnRune('e', func(e *tcell.EventKey) bool {
			row := wl.table.SelectedRow()
			if row >= 0 && row < len(wl.workflows) {
				wf := wl.workflows[row]
				wl.app.NavigateToEvents(wf.ID, wf.RunID)
				return true
			}
			return false
		}).
		On(tcell.KeyTab, func(e *tcell.EventKey) bool {
			if !wl.previewModeEnabled() {
				return false
			}
			wl.cycleFocus(1)
			return true
		}).
		On(tcell.KeyBacktab, func(e *tcell.EventKey) bool {
			if !wl.previewModeEnabled() {
				return false
			}
			wl.cycleFocus(-1)
			return true
		})

	wl.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if bindings.Handle(event) {
			return nil
		}
		return event
	})

	wl.loadData()
}

// Stop is called when the view is deactivated.
func (wl *WorkflowList) Stop() {
	wl.table.SetInputCapture(nil)
	if wl.previewTimer != nil {
		wl.previewTimer.Stop()
	}
	wl.stopAutoRefresh()
	wl.app.ClearWorkflowStats()
}

// Hints returns keybinding hints for this view.
func (wl *WorkflowList) Hints() []KeyHint {
	if wl.selectionMode {
		hints := []KeyHint{
			{Key: "space", Description: "Select"},
			{Key: "Ctrl+A", Description: "Select All"},
			{Key: "v", Description: "Exit Select"},
		}
		if len(wl.table.GetSelectedRows()) > 0 {
			hints = append(hints,
				KeyHint{Key: "c", Description: "Cancel"},
				KeyHint{Key: "X", Description: "Terminate"},
			)
		}
		hints = append(hints, KeyHint{Key: "esc", Description: "Back"})
		return hints
	}

	if wl.previewModeEnabled() {
		switch wl.focusPane {
		case focusEvents:
			return []KeyHint{
				{Key: "j/k", Description: wl.previewKind.title()},
				{Key: "tab", Description: "Details"},
				{Key: "[/]", Description: "View"},
				{Key: "i", Description: "Input/Output"},
				{Key: "p", Description: "Preview"},
				{Key: "e", Description: "Event Graph"},
				{Key: "esc", Description: "Workflows"},
			}
		case focusEventDetail:
			return []KeyHint{
				{Key: "j/k", Description: "Scroll"},
				{Key: "tab", Description: "Workflows"},
				{Key: "[/]", Description: "View"},
				{Key: "i", Description: "Input/Output"},
				{Key: "p", Description: "Preview"},
				{Key: "esc", Description: "Workflows"},
			}
		}
	}

	hints := []KeyHint{
		{Key: "enter", Description: "Detail"},
		{Key: "p", Description: "Preview"},
	}
	if wl.previewModeEnabled() {
		hints = []KeyHint{
			{Key: "enter", Description: wl.previewKind.title()},
			{Key: "tab", Description: wl.previewKind.title()},
			{Key: "[/]", Description: "View"},
			{Key: "i", Description: "Input/Output"},
			{Key: "p", Description: "Preview"},
		}
	}
	hints = append(hints,
		KeyHint{Key: "e", Description: "Event Graph"},
		KeyHint{Key: "h/l", Description: "Scroll"},
		KeyHint{Key: "|", Description: "Columns"},
		KeyHint{Key: "/", Description: "Filter"},
		KeyHint{Key: "F", Description: "Query"},
		KeyHint{Key: "f", Description: "Templates"},
		KeyHint{Key: "D", Description: "Date Range"},
	)
	if wl.visibilityQuery != "" {
		hints = append(hints,
			KeyHint{Key: "C", Description: "Clear Query"},
			KeyHint{Key: "S", Description: "Save Filter"},
		)
	}
	hints = append(hints,
		KeyHint{Key: "L", Description: "Load Filter"},
		KeyHint{Key: "d", Description: "Diff"},
		KeyHint{Key: "o", Description: "Overview"},
		KeyHint{Key: "v", Description: "Select Mode"},
		KeyHint{Key: "N", Description: "Start"},
		KeyHint{Key: "W", Description: "Signal+Start"},
		KeyHint{Key: "y", Description: "Copy ID"},
		KeyHint{Key: "r", Description: "Refresh"},
		KeyHint{Key: "a", Description: "Auto-refresh"},
		KeyHint{Key: "t", Description: "Task Queues"},
		KeyHint{Key: "s", Description: "Schedules"},
		KeyHint{Key: "T", Description: "Theme"},
		KeyHint{Key: "?", Description: "Help"},
		KeyHint{Key: "esc", Description: "Back"},
	)
	return hints
}

// HandleEscape implements EscapeHandler to clear filter state before navigation.
func (wl *WorkflowList) HandleEscape() bool {
	if wl.focusPane != focusWorkflows {
		wl.setFocusPane(focusWorkflows)
		return true
	}
	if wl.filterText != "" || wl.visibilityQuery != "" || wl.originalWorkflows != nil {
		wl.clearAllFilters()
		return true
	}
	return false
}

// Focus sets focus to the table.
func (wl *WorkflowList) Focus(delegate func(p tview.Primitive)) {
	if len(wl.workflows) == 0 && len(wl.allWorkflows) == 0 {
		delegate(wl.workflowsPanel)
		return
	}
	switch wl.focusPane {
	case focusEvents:
		delegate(wl.eventTable)
	case focusEventDetail:
		if wl.previewKind == previewDetails && wl.workflowDetail != nil {
			delegate(wl.workflowDetail)
			return
		}
		delegate(wl.eventDetail)
	default:
		delegate(wl.table)
	}
}

// Draw draws the workflow list.
func (wl *WorkflowList) Draw(screen tcell.Screen) {
	bg := theme.Bg()
	wl.SetBackgroundColor(bg)
	if wl.previewPanel != nil {
		wl.previewPanel.SetBackgroundColor(bg)
	}
	if wl.eventTable != nil {
		wl.eventTable.SetBackgroundColor(bg)
	}
	if wl.eventDetail != nil {
		wl.eventDetail.SetBackgroundColor(bg)
		wl.eventDetail.SetTextColor(theme.Fg())
	}
	wl.syncFocusFromPrimitives()
	wl.Flex.Draw(screen)
}
