package view

import (
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type workflowFocusPane int

const (
	focusWorkflows workflowFocusPane = iota
	focusEvents
	focusEventDetail
	focusTimeline
	focusPollers
	focusScheduleDetail
	focusScheduleRuns
	focusWorkerDetail
	focusFilters
)

// WorkflowList displays a list of workflows.
type WorkflowList struct {
	*tview.Flex
	mainFlex               *tview.Flex
	primaryStack           *tview.Flex
	primaryWidth           int
	tertiaryHeight         int
	timelineHeight         int
	app                    *App
	namespace              string
	table                  *components.Table
	tableScroll            *charScrollView
	listTabs               *components.Tabs
	workflowTab            *components.Tab
	taskQueues             *TaskQueueView
	schedules              *ScheduleList
	workers                *WorkerView
	listKind               listKind
	pollersVisible         bool
	scheduleDetailVisible  bool
	workerDetailVisible    bool
	filterBar              *filterChipBar
	workflowStack          *tview.Flex
	workflowBody           tview.Primitive
	filterBarRows          int
	filterBarSide          bool
	workflowsPanel         *chromePanel
	previewPanel           *components.Panel
	previewTabs            *components.Tabs
	rightFlex              *tview.Flex
	eventTable             *components.Table
	eventTableScroll       *charScrollView
	eventTreeView          *EventTreeView
	eventTreeMode          bool
	eventTab               *components.Tab
	eventDetail            *tview.TextView
	eventDetailPanel       *components.Panel
	eventsPanel            *components.Panel
	workflowDetail         *components.Table
	workflowDetailScroll   *charScrollView
	previewDetailRows      []workflowInfoRow
	timelineView           *TimelineView
	timelinePanel          *timelineFrame
	timelineVisible        bool
	timelineNarrow         bool
	highlightedActivityID  int64
	highlightedWorkflowID  string
	highlightedRunID       string
	timelineSyncing        bool
	workflowTreeMode       bool
	workflowDepths         []int
	workflowTreePrefixes   []string
	workflowHasChildren    []bool
	workflowCollapsed      map[string]bool
	focusPane              workflowFocusPane
	previewKind            previewKind
	activityDetailKind     activityDetailKind
	activityDetailTabs     *components.Tabs
	activityDetail         *components.Table
	activityDetailScroll   *charScrollView
	activityDetailRows     []workflowInfoRow
	workflowIOKind         workflowIOKind
	workflowIOTabs         *components.Tabs
	workflowIOView         *tview.TextView
	hierarchyView          *WorkflowGraphView
	hierarchyGraphPanel    *components.Panel
	previewEvents          []temporal.EnhancedHistoryEvent
	previewEventSearch     string
	previewActivities      []previewActivity
	previewActivitySearch  string
	previewWorkflowID      string
	previewRunID           string
	previewGen             uint64
	previewTimer           *time.Timer
	previewPending         bool
	previewMode            bool
	previewCache           *previewCache
	allWorkflows           []temporal.Workflow // Full unfiltered list
	workflows              []temporal.Workflow // Filtered list for display
	filterText             string
	visibilityQuery        string // Temporal visibility query
	activeFilterName       string
	filterClauses          []config.FilterClause
	adHoc                  adHocFilter
	filterTestPending      bool
	filterTestSavedName    string
	filterTestSavedQuery   string
	filterTestSavedClauses []config.FilterClause
	filterTestSavedAdHoc   adHocFilter
	pager                  workflowPager
	pageBusy               bool
	pageGen                uint64
	listAnchorIndex        int
	hasListAnchor          bool
	listEdgePin            listEdgePin
	serverStats            WorkflowStats
	serverStatsOK          bool
	loading                bool
	autoRefresh            bool
	liveBusy               bool
	refreshTicker          *time.Ticker
	stopRefresh            chan struct{}
	selectionMode          bool     // Multi-select mode active
	searchHistory          []string // History of visibility queries
	historyIndex           int      // Current position in history (-1 = not browsing)
	maxHistorySize         int      // Maximum number of history entries
	// Server-side completion support
	serverCompletions   []string            // Cached completions from server query
	lastCompletionQuery string              // Last query sent to server (to avoid duplicates)
	originalWorkflows   []temporal.Workflow // Original workflows before server search
	preloaded           bool                // True if workflows were provided at construction time
	keepDataOnStart     bool
}

func (wl *WorkflowList) HoldStartData() {
	if wl != nil {
		wl.keepDataOnStart = true
	}
}

// NewWorkflowList creates a new workflow list view.
func NewWorkflowList(app *App, namespace string) *WorkflowList {
	wl := &WorkflowList{
		Flex:                  tview.NewFlex().SetDirection(tview.FlexRow),
		app:                   app,
		namespace:             namespace,
		table:                 components.NewTable(),
		workflows:             []temporal.Workflow{},
		stopRefresh:           make(chan struct{}, 1), // Buffered to ensure stop signal isn't lost
		searchHistory:         make([]string, 0, 50),
		historyIndex:          -1,
		maxHistorySize:        50,
		previewKind:           previewActivities,
		previewCache:          newPreviewCache(previewCacheLimit(app)),
		pollersVisible:        true,
		scheduleDetailVisible: true,
		workerDetailVisible:   true,
		workflowTreeMode:      true,
		eventTreeMode:         true,
	}
	wl.setup()

	// Register for automatic theme refresh
	theme.RegisterRefreshable(wl)

	return wl
}

// NewWorkflowListWithData creates a workflow list pre-populated with data (no server fetch).
func NewWorkflowListWithData(app *App, namespace string, workflows []temporal.Workflow) *WorkflowList {
	wl := &WorkflowList{
		Flex:             tview.NewFlex().SetDirection(tview.FlexRow),
		app:              app,
		namespace:        namespace,
		table:            components.NewTable(),
		allWorkflows:     workflows,
		workflows:        workflows,
		stopRefresh:      make(chan struct{}, 1),
		searchHistory:    make([]string, 0, 50),
		historyIndex:     -1,
		maxHistorySize:   50,
		previewKind:      previewActivities,
		preloaded:        true,
		workflowTreeMode: true,
		eventTreeMode:    true,
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
	wl.mainFlex = tview.NewFlex().SetDirection(tview.FlexColumn)
	wl.mainFlex.SetBackgroundColor(theme.Bg())
	wl.table.SetEvaluateAllRows(true)
	wl.table.SetBorder(false)
	wl.table.SetBackgroundColor(theme.Bg())
	applyWorkflowColumnHeaders(wl.table, wl.columnLayout())
	wl.tableScroll = newCharScrollView(wl.table, func() int {
		return workflowTableContentWidth(wl.columnLayout())
	}).withApp(wl.app)
	bindTableCharScroll(wl.table, wl.tableScroll, func() int {
		return mouseScrollStepFromApp(wl.app)
	})
	wl.setupPreview()

	wl.workflowsPanel = newChromePanel(wl.app)
	wl.applyProfileTitle()
	wl.setupListTabs()
	wl.updatePanelTitle()

	wl.applyPreviewLayout()

	wl.table.ConfigureEmpty(theme.IconInfo, "No Workflows", "No workflows found in this namespace")

	wl.clearPreview()

	wl.table.SetSelectionChangedFunc(func(row, col int) {
		wl.syncHistoryForSelectedRow()
	})

	wl.table.SetOnSelect(func(row int) {
		wl.activateSelectedWorkflow()
	})
}

// RefreshTheme updates all component colors after a theme change.
func (wl *WorkflowList) RefreshTheme() {
	bg := theme.Bg()
	wl.SetBackgroundColor(bg)
	if wl.mainFlex != nil {
		wl.mainFlex.SetBackgroundColor(bg)
	}
	if wl.primaryStack != nil {
		wl.primaryStack.SetBackgroundColor(bg)
	}
	if wl.listTabs != nil {
		wl.listTabs.SetBackgroundColor(bg)
	}
	if wl.workflowStack != nil {
		wl.workflowStack.SetBackgroundColor(bg)
	}
	if wl.filterBar != nil {
		wl.filterBar.SetBackgroundColor(bg)
	}
	if wl.previewTabs != nil {
		wl.previewTabs.SetBackgroundColor(bg)
	}
	if wl.activityDetailTabs != nil {
		wl.activityDetailTabs.SetBackgroundColor(bg)
	}
	if wl.tableScroll != nil {
		wl.tableScroll.SetBackgroundColor(bg)
	}
	if wl.eventTableScroll != nil {
		wl.eventTableScroll.SetBackgroundColor(bg)
	}
	if wl.activityDetailScroll != nil {
		wl.activityDetailScroll.SetBackgroundColor(bg)
	}
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetBackgroundColor(bg)
	}
	wl.table.SetBackgroundColor(bg)
	wl.eventTable.SetBackgroundColor(bg)
	if wl.eventTreeView != nil {
		wl.eventTreeView.SetBackgroundColor(bg)
	}
	wl.eventDetail.SetBackgroundColor(bg)
	wl.eventDetail.SetTextColor(theme.Fg())
	if wl.workflowIOView != nil {
		wl.workflowIOView.SetBackgroundColor(bg)
		wl.workflowIOView.SetTextColor(theme.Fg())
	}
	if wl.rightFlex != nil {
		wl.rightFlex.SetBackgroundColor(bg)
	}
	if wl.workflowDetail != nil {
		wl.workflowDetail.SetBackgroundColor(bg)
	}
	if wl.activityDetail != nil {
		wl.activityDetail.SetBackgroundColor(bg)
	}
	if wl.previewPanel != nil {
		wl.previewPanel.SetBackgroundColor(bg)
	}
	if wl.timelineView != nil {
		wl.timelineView.SetBackgroundColor(bg)
	}
	if wl.taskQueues != nil {
		wl.taskQueues.RefreshTheme()
	}
	if wl.schedules != nil {
		wl.schedules.RefreshTheme()
	}
	if wl.workers != nil {
		wl.workers.RefreshTheme()
	}
	if wl.hierarchyView != nil {
		wl.hierarchyView.RefreshTheme()
	}
	wl.populateTable()
	wl.applyFocusStyles()
}

func (wl *WorkflowList) displayedWorkflowCount() int {
	if wl == nil {
		return 0
	}
	return len(wl.workflows)
}

func (wl *WorkflowList) loadedWorkflowCount() int {
	if wl == nil {
		return 0
	}
	return len(wl.allWorkflows)
}

func (wl *WorkflowList) totalWorkflowCount() int {
	if wl == nil {
		return 0
	}
	if wl.serverStatsOK {
		if wl.serverStats.Total > 0 {
			return wl.serverStats.Total
		}
		return wl.serverStats.Running + wl.serverStats.Completed + wl.serverStats.Failed +
			wl.serverStats.Canceled + wl.serverStats.Terminated + wl.serverStats.TimedOut +
			wl.serverStats.ContinuedAsNew
	}
	return len(wl.allWorkflows)
}

func (wl *WorkflowList) applyProfileTitle() {
	if wl == nil || wl.workflowsPanel == nil {
		return
	}
	wl.workflowsPanel.SetTitle("")
}

func (wl *WorkflowList) SetMasterTitle(title string) {
	name := workflowTabName(title)
	if wl.workflowTab != nil {
		wl.workflowTab.Name = name
	}
}

func (wl *WorkflowList) SetMasterContent(content tview.Primitive) {
	if content == wl.table && wl.tableScroll != nil {
		content = wl.tableScroll
	}
	wl.workflowBody = content
	wl.mountWorkflowContent()
}

func (wl *WorkflowList) mountWorkflowContent() {
	if wl.workflowStack == nil {
		if wl.workflowTab != nil && wl.workflowBody != nil {
			wl.workflowTab.Content = wl.workflowBody
			return
		}
		if wl.workflowsPanel != nil && wl.workflowBody != nil {
			wl.workflowsPanel.SetContent(wl.workflowBody)
		}
		return
	}
	body := wl.workflowBody
	if body == nil {
		body = wl.tableScroll
	}
	side := wl.filtersOnSide()
	wl.filterBarSide = side
	if side {
		wl.workflowStack.SetDirection(tview.FlexColumn)
		width := wl.filterSidebarWidth()
		wl.filterBarRows = width
	} else {
		wl.workflowStack.SetDirection(tview.FlexRow)
		rows := wl.filterBarHeight()
		wl.filterBarRows = rows
	}
	wl.workflowStack.Clear()
	if wl.filterBar != nil {
		wl.workflowStack.AddItem(wl.filterBar, wl.filterBarRows, 0, false)
	}
	if body != nil {
		wl.workflowStack.AddItem(body, 0, 1, true)
	}
	if wl.workflowTab != nil {
		wl.workflowTab.Content = wl.workflowStack
	}
}

// Name returns the view name.
func (wl *WorkflowList) Name() string {
	switch {
	case wl.taskQueuesActive():
		return "task-queues"
	case wl.schedulesActive():
		return "schedules"
	case wl.workersActive():
		return "workers"
	default:
		return "workflows"
	}
}

// Start is called when the view becomes active.
func (wl *WorkflowList) Start() {
	bindings := input.NewKeyBindings().
		OnRune(' ', func(e *tcell.EventKey) bool {
			if workflowTreeFoldAllKey(e) {
				return wl.toggleWorkflowTreeFoldAll()
			}
			if wl.selectionMode {
				wl.toggleRowSelection()
				if next := wl.table.SelectedRow() + 1; next < wl.table.RowCount() {
					wl.table.SelectRow(next)
				}
				wl.updateSelectionPreview()
				return true
			}
			return wl.toggleWorkflowTreeFold()
		}).
		On(tcell.KeyCtrlSpace, func(e *tcell.EventKey) bool {
			return wl.toggleWorkflowTreeFoldAll()
		}).
		OnRune('/', func(e *tcell.EventKey) bool {
			wl.showFilter()
			return true
		}).
		OnRune('f', func(e *tcell.EventKey) bool {
			wl.showAdHocClauseEditor()
			return true
		}).
		OnRune('F', func(e *tcell.EventKey) bool {
			wl.showFilterManager()
			return true
		}).
		OnRune('d', func(e *tcell.EventKey) bool {
			if wl.selectionMode {
				if len(wl.table.GetSelectedRows()) > 0 {
					wl.showBatchDeleteConfirm()
				}
				return true
			}
			return false
		}).
		OnRune('D', func(e *tcell.EventKey) bool {
			if wl.selectionMode && len(wl.table.GetSelectedRows()) > 0 {
				wl.showBatchDeleteConfirm()
				return true
			}
			wl.showDeleteSelected()
			return true
		}).
		OnRune('s', func(e *tcell.EventKey) bool {
			wl.showSignalSelected()
			return true
		}).
		OnRune('a', func(e *tcell.EventKey) bool {
			if wl.selectionMode {
				wl.table.SelectAll()
				wl.updateSelectionPreview()
				return true
			}
			wl.toggleAutoRefresh()
			return true
		}).
		OnRune('r', func(e *tcell.EventKey) bool {
			wl.refresh()
			return true
		}).
		OnRune('y', func(e *tcell.EventKey) bool {
			wl.copyWorkflowID()
			return true
		}).
		OnRune('u', func(e *tcell.EventKey) bool {
			return wl.openSelectedWorkflowUI()
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
			wl.showCancelSelected()
			return true
		}).
		OnRune('X', func(e *tcell.EventKey) bool {
			if wl.selectionMode && len(wl.table.GetSelectedRows()) > 0 {
				wl.showBatchTerminateConfirm()
				return true
			}
			wl.showTerminateSelected()
			return true
		}).
		OnRune('Q', func(e *tcell.EventKey) bool {
			wl.showQuerySelected()
			return true
		}).
		OnRune('R', func(e *tcell.EventKey) bool {
			wl.showResetSelected()
			return true
		}).
		OnRune('N', func(e *tcell.EventKey) bool {
			wl.showStartWorkflow()
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
		OnRune('z', func(e *tcell.EventKey) bool {
			wl.toggleTimeline()
			return true
		}).
		OnRune('b', func(e *tcell.EventKey) bool {
			wl.toggleWorkflowTree()
			return true
		}).
		OnRune('i', func(e *tcell.EventKey) bool {
			return wl.showPreviewIO()
		}).
		On(tcell.KeyTab, func(e *tcell.EventKey) bool {
			wl.cycleFocus(1)
			return true
		}).
		On(tcell.KeyBacktab, func(e *tcell.EventKey) bool {
			wl.cycleFocus(-1)
			return true
		})

	wl.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) {
			return nil
		}
		if wl.handleListTabKey(event) || wl.handlePreviewTabKey(event) {
			return nil
		}
		if wl.handleWorkflowScroll(event) {
			return nil
		}
		if bindings.Handle(event) {
			return nil
		}
		return event
	})

	if !wl.workflowsActive() {
		switch {
		case wl.taskQueuesActive():
			wl.ensureTaskQueues()
		case wl.schedulesActive():
			wl.ensureSchedules()
		case wl.workersActive():
			wl.ensureWorkers()
		}
		wl.restoreFocus()
		wl.updateStats()
		return
	}
	if wl.keepDataOnStart {
		wl.keepDataOnStart = false
		wl.restoreFocus()
		wl.updateStats()
		return
	}
	wl.loadData()
}

func (wl *WorkflowList) restoreFocus() {
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
		return
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) shouldFocusWorkflowTable() bool {
	if wl == nil || !wl.workflowsActive() || wl.focusPane != focusWorkflows {
		return false
	}
	if wl.app != nil && wl.app.modalHasFocus() {
		return false
	}
	return true
}

func (wl *WorkflowList) handleWorkflowScroll(event *tcell.EventKey) bool {
	if event == nil || event.Modifiers() != tcell.ModNone {
		return false
	}
	switch event.Rune() {
	case 'g':
		wl.jumpWorkflowListEdge(false)
		return true
	case 'G':
		wl.jumpWorkflowListEdge(true)
		return true
	}
	if wl.tableScroll == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyLeft:
		wl.tableScroll.scrollChars(-1)
		return true
	case tcell.KeyRight:
		wl.tableScroll.scrollChars(1)
		return true
	case tcell.KeyHome:
		wl.tableScroll.scrollTo(scrollOffsetByColumn(wl.tableScroll.offset, wl.columnLayout(), -1))
		return true
	case tcell.KeyEnd:
		wl.tableScroll.scrollTo(scrollOffsetByColumn(wl.tableScroll.offset, wl.columnLayout(), 1))
		return true
	}
	switch event.Rune() {
	case 'h':
		wl.tableScroll.scrollChars(-1)
		return true
	case 'l':
		wl.tableScroll.scrollChars(1)
		return true
	}
	return false
}

// Stop is called when the view is deactivated.
func (wl *WorkflowList) Stop() {
	wl.table.SetInputCapture(nil)
	if wl.previewTimer != nil {
		wl.previewTimer.Stop()
	}
	wl.stopAutoRefresh()
	if wl.taskQueues != nil {
		wl.taskQueues.Stop()
	}
	if wl.schedules != nil {
		wl.schedules.Stop()
	}
	if wl.workers != nil {
		wl.workers.Stop()
	}
}

// Hints returns keybinding hints for this view.
func (wl *WorkflowList) Hints() []KeyHint {
	if wl.taskQueuesActive() {
		if wl.focusPane == focusPollers {
			return []KeyHint{
				{Key: "Enter", Description: "Show Worker"},
				{Key: "r", Description: "Refresh"},
				{Key: "a", Description: "Auto-refresh"},
			}
		}
		return []KeyHint{
			{Key: "/", Description: "Search"},
			{Key: "r", Description: "Refresh"},
			{Key: "a", Description: "Auto-refresh"},
		}
	}
	if wl.workersActive() {
		if wl.focusPane == focusWorkerDetail {
			return []KeyHint{
				{Key: "y", Description: "Yank"},
				{Key: "r", Description: "Refresh"},
			}
		}
		return []KeyHint{
			{Key: "Enter", Description: "Detail"},
			{Key: "/", Description: "Search"},
			{Key: "r", Description: "Refresh"},
		}
	}
	if wl.schedulesActive() {
		switch wl.focusPane {
		case focusScheduleDetail:
			return []KeyHint{
				{Key: "y", Description: "Yank"},
				{Key: "r", Description: "Refresh"},
			}
		case focusScheduleRuns:
			return []KeyHint{
				{Key: "Enter", Description: "Open Run"},
				{Key: "y", Description: "Copy ID"},
				{Key: "r", Description: "Refresh"},
			}
		}
		return []KeyHint{
			{Key: "/", Description: "Search"},
			{Key: "r", Description: "Refresh"},
			{Key: "P", Description: "Pause/Unpause"},
			{Key: "t", Description: "Trigger"},
			{Key: "v", Description: "View runs"},
			{Key: "D", Description: "Delete"},
		}
	}

	if wl.selectionMode && wl.focusPane == focusWorkflows {
		hints := []KeyHint{
			{Key: "space", Description: "Select"},
			{Key: "a", Description: "Select All"},
		}
		if len(wl.table.GetSelectedRows()) > 0 {
			hints = append(hints,
				KeyHint{Key: "c", Description: "Cancel"},
				KeyHint{Key: "X", Description: "Terminate"},
				KeyHint{Key: "d", Description: "Delete"},
			)
		}
		return hints
	}

	switch wl.focusPane {
	case focusEvents:
		return wl.previewListHints()
	case focusEventDetail:
		return wl.previewSideHints()
	case focusTimeline:
		return wl.timelineHints()
	}
	return wl.workflowPaneHints()
}

func (wl *WorkflowList) previewIOHint() []KeyHint {
	if !wl.previewModeEnabled() || !wl.previewShowsIO() {
		return nil
	}
	return []KeyHint{{Key: "i", Description: "Input/Output"}}
}

func (wl *WorkflowList) previewShowsSidePane() bool {
	return wl.previewKind == previewActivities || wl.previewKind == previewEvents || wl.previewKind == previewDetails
}

func (wl *WorkflowList) previewShowsIO() bool {
	return wl.previewKind == previewActivities || wl.previewKind == previewEvents
}

func (wl *WorkflowList) previewListHints() []KeyHint {
	if wl.previewKind == previewHierarchy {
		return []KeyHint{
			{Key: "space", Description: "Collapse/Expand"},
			{Key: "+/-", Description: "Depth"},
			{Key: "p", Description: "Preview"},
		}
	}
	hints := wl.previewIOHint()
	if wl.previewKind == previewDetails && wl.focusPane == focusEvents {
		hints = append(hints, KeyHint{Key: "y", Description: "Yank"})
	}
	if wl.previewKind == previewEvents || wl.previewKind == previewActivities {
		hints = append(hints, KeyHint{Key: "/", Description: "Search"})
	}
	if wl.previewKind == previewActivities {
		hints = append(hints, KeyHint{Key: "|", Description: "Columns"})
	}
	if wl.previewKind == previewEvents {
		if wl.eventTreeMode {
			hints = append(hints, KeyHint{Key: "space", Description: "Collapse/Expand"})
		}
		hints = append(hints, KeyHint{Key: "b", Description: treeModeHint(wl.eventTreeMode)})
		if ev, ok := wl.selectedPreviewEvent(); ok && ev.ChildWorkflowID != "" && ev.ChildRunID != "" {
			hints = append(hints, KeyHint{Key: "g", Description: "Go to Child"})
		}
	}
	return append(hints,
		KeyHint{Key: "r", Description: "Refresh"},
		KeyHint{Key: "p", Description: "Preview"},
	)
}

func (wl *WorkflowList) previewSideHints() []KeyHint {
	if wl.previewKind == previewEvents {
		hints := []KeyHint{{Key: "y", Description: "Yank"}}
		return append(hints, wl.previewListHints()...)
	}
	if wl.activityDetailTableFocused() {
		return append([]KeyHint{
			{Key: "y", Description: "Yank"},
			{Key: "/", Description: "Search"},
			{Key: "|", Description: "Columns"},
		},
			KeyHint{Key: "r", Description: "Refresh"},
			KeyHint{Key: "p", Description: "Preview"},
		)
	}
	if wl.previewKind == previewHierarchy {
		return []KeyHint{
			{Key: "c", Description: "Center Graph"},
			{Key: "+/-", Description: "Depth"},
			{Key: "p", Description: "Preview"},
		}
	}
	if wl.previewIOViewFocused() {
		hints := []KeyHint{
			{Key: "e", Description: "Editor"},
			{Key: "y", Description: "Yank"},
		}
		return append(hints, wl.previewListHints()...)
	}
	return wl.previewListHints()
}

func (wl *WorkflowList) timelineHints() []KeyHint {
	return []KeyHint{
		{Key: "+/-", Description: "Zoom"},
		{Key: "m", Description: timelineSizeHint(wl.timelineNarrow)},
		{Key: "p", Description: "Preview"},
		{Key: "L", Description: "Legend"},
	}
}

func (wl *WorkflowList) workflowPaneHints() []KeyHint {
	hints := []KeyHint{
		{Key: "Enter", Description: "Preview"},
		{Key: "i", Description: "Input/Output"},
	}
	if wl.previewModeEnabled() {
		hints = append(hints,
			KeyHint{Key: "z", Description: "Timeline"},
			KeyHint{Key: "b", Description: treeModeHint(wl.workflowTreeMode)},
			KeyHint{Key: "p", Description: "Preview"},
		)
	} else {
		hints = append(hints,
			KeyHint{Key: "p", Description: "Preview"},
			KeyHint{Key: "z", Description: "Timeline"},
			KeyHint{Key: "b", Description: treeModeHint(wl.workflowTreeMode)},
		)
	}
	if wl.workflowTreeMode {
		hints = append(hints,
			KeyHint{Key: "space", Description: "Fold/Unfold"},
			KeyHint{Key: "Ctrl+Space", Description: workflowTreeFoldAllHint(wl.anyWorkflowFolded())},
		)
	}
	hints = append(hints,
		KeyHint{Key: "|", Description: "Columns"},
		KeyHint{Key: "/", Description: "Search"},
		KeyHint{Key: "f", Description: "Add Filter"},
		KeyHint{Key: "F", Description: "Filters"},
	)
	hints = append(hints,
		KeyHint{Key: "v", Description: "Select Mode"},
		KeyHint{Key: "N", Description: "Start"},
	)
	if w, ok := wl.selectedWorkflow(); ok {
		hints = append(hints, wl.listActionHints(w)...)
	}
	return append(hints,
		KeyHint{Key: "y", Description: "Copy ID"},
		KeyHint{Key: "u", Description: "Web UI"},
		KeyHint{Key: "r", Description: "Refresh"},
		KeyHint{Key: "a", Description: "Auto-refresh"},
		KeyHint{Key: "T", Description: "Theme"},
		KeyHint{Key: "?", Description: "Help"},
	)
}

// HandleEscape implements EscapeHandler to leave select mode or clear filters before navigation.
func (wl *WorkflowList) HandleEscape() bool {
	if wl.taskQueuesActive() && wl.focusPane == focusPollers {
		wl.setPollersVisible(false)
		return true
	}
	if wl.schedulesActive() && (wl.focusPane == focusScheduleDetail || wl.focusPane == focusScheduleRuns) {
		wl.setScheduleDetailVisible(false)
		return true
	}
	if wl.workersActive() && wl.focusPane == focusWorkerDetail {
		wl.setWorkerDetailVisible(false)
		return true
	}
	if wl.selectionMode {
		wl.toggleSelectionMode()
		return true
	}
	if wl.escapeFromPreview() {
		return true
	}
	if wl.focusPane != focusWorkflows {
		wl.setFocusPane(focusWorkflows)
		return true
	}
	if wl.taskQueuesActive() && wl.taskQueues != nil && wl.taskQueues.searchText != "" {
		wl.taskQueues.applyFilter("")
		return true
	}
	if wl.schedulesActive() && wl.schedules != nil && wl.schedules.GetSearchText() != "" {
		wl.schedules.ClearSearch()
		wl.schedules.applyFilter("")
		return true
	}
	if wl.workersActive() && wl.workers != nil && wl.workers.searchText != "" {
		wl.workers.applyFilter("")
		return true
	}
	if wl.workflowsActive() && (wl.filterText != "" || wl.visibilityQuery != "" || wl.originalWorkflows != nil) {
		wl.clearAllFilters()
		return true
	}
	return false
}

// Focus sets focus to the table.
func (wl *WorkflowList) Focus(delegate func(p tview.Primitive)) {
	if wl.taskQueuesActive() && wl.taskQueues != nil {
		if wl.focusPane == focusPollers {
			delegate(wl.taskQueues.pollerTable)
			return
		}
		delegate(wl.taskQueues.queueTable)
		return
	}
	if wl.schedulesActive() && wl.schedules != nil {
		switch wl.focusPane {
		case focusScheduleDetail:
			delegate(wl.schedules.detail)
			return
		case focusScheduleRuns:
			delegate(wl.schedules.runsTable)
			return
		}
		delegate(wl.schedules.table)
		return
	}
	if wl.workersActive() && wl.workers != nil {
		if wl.focusPane == focusWorkerDetail && wl.workers.detail != nil {
			delegate(wl.workers.detail)
			return
		}
		delegate(wl.workers.table)
		return
	}
	switch wl.focusPane {
	case focusEvents:
		if p := wl.eventsPreviewPrimitive(); p != nil {
			delegate(p)
			return
		}
		delegate(wl.eventTable)
	case focusEventDetail:
		if wl.previewKind == previewDetails && wl.workflowIOView != nil {
			delegate(wl.workflowIOView)
			return
		}
		if wl.previewKind == previewHierarchy && wl.hierarchyView != nil && wl.hierarchyView.graph != nil {
			delegate(wl.hierarchyView.graph)
			return
		}
		if p := wl.activityDetailFocusPrimitive(); p != nil {
			delegate(p)
			return
		}
		delegate(wl.eventDetail)
	case focusTimeline:
		if wl.timelineView != nil {
			delegate(wl.timelineView)
			return
		}
		delegate(wl.table)
	case focusFilters:
		if wl.filterBar != nil {
			delegate(wl.filterBar)
			return
		}
		delegate(wl.table)
	default:
		delegate(wl.table)
	}
}

// Draw draws the workflow list.
func (wl *WorkflowList) Draw(screen tcell.Screen) {
	bg := theme.Bg()
	wl.SetBackgroundColor(bg)
	if wl.mainFlex != nil {
		wl.mainFlex.SetBackgroundColor(bg)
	}
	if wl.primaryStack != nil {
		wl.primaryStack.SetBackgroundColor(bg)
	}
	if wl.listTabs != nil {
		wl.listTabs.SetBackgroundColor(bg)
	}
	if wl.workflowStack != nil {
		wl.workflowStack.SetBackgroundColor(bg)
	}
	if wl.filterBar != nil {
		wl.filterBar.SetBackgroundColor(bg)
	}
	if wl.previewTabs != nil {
		wl.previewTabs.SetBackgroundColor(bg)
	}
	if wl.activityDetailTabs != nil {
		wl.activityDetailTabs.SetBackgroundColor(bg)
	}
	if wl.tableScroll != nil {
		wl.tableScroll.SetBackgroundColor(bg)
	}
	if wl.eventTableScroll != nil {
		wl.eventTableScroll.SetBackgroundColor(bg)
	}
	if wl.activityDetailScroll != nil {
		wl.activityDetailScroll.SetBackgroundColor(bg)
	}
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetBackgroundColor(bg)
	}
	if wl.previewPanel != nil {
		wl.previewPanel.SetBackgroundColor(bg)
	}
	if wl.eventTable != nil {
		wl.eventTable.SetBackgroundColor(bg)
	}
	if wl.eventTreeView != nil {
		wl.eventTreeView.SetBackgroundColor(bg)
	}
	if wl.eventDetail != nil {
		wl.eventDetail.SetBackgroundColor(bg)
		wl.eventDetail.SetTextColor(theme.Fg())
	}
	if wl.workflowIOView != nil {
		wl.workflowIOView.SetBackgroundColor(bg)
		wl.workflowIOView.SetTextColor(theme.Fg())
	}
	if wl.activityDetail != nil {
		wl.activityDetail.SetBackgroundColor(bg)
	}
	if wl.rightFlex != nil {
		wl.rightFlex.SetBackgroundColor(bg)
	}
	if wl.timelineView != nil {
		wl.timelineView.SetBackgroundColor(bg)
	}
	wl.syncFocusFromPrimitives()
	wl.syncFilterBarLayout()
	wl.Flex.Draw(screen)
}
