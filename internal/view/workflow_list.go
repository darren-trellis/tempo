package view

import (
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
	focusTimeline
	focusPollers
	focusScheduleDetail
	focusScheduleRuns
	focusWorkerDetail
)

// WorkflowList displays a list of workflows.
type WorkflowList struct {
	*tview.Flex
	mainFlex              *tview.Flex
	app                   *App
	namespace             string
	table                 *components.Table
	tableScroll           *charScrollView
	listTabs              *components.Tabs
	workflowTab           *components.Tab
	taskQueues            *TaskQueueView
	schedules             *ScheduleList
	workers               *WorkerView
	listKind              listKind
	pollersVisible        bool
	scheduleDetailVisible bool
	workerDetailVisible   bool
	workflowsPanel        *components.Panel
	previewPanel          *components.Panel
	previewTabs           *components.Tabs
	rightFlex             *tview.Flex
	eventTable            *components.Table
	eventTableScroll      *charScrollView
	eventTreeView         *EventTreeView
	eventTreeMode         bool
	eventTab              *components.Tab
	eventDetail           *tview.TextView
	eventDetailPanel      *components.Panel
	eventsPanel           *components.Panel
	workflowDetail        *components.Table
	workflowDetailScroll  *charScrollView
	previewDetailRows     []workflowInfoRow
	timelineView          *TimelineView
	timelinePanel         *timelineFrame
	timelineVisible       bool
	timelineNarrow        bool
	highlightedActivityID int64
	highlightedWorkflowID string
	highlightedRunID      string
	timelineSyncing       bool
	workflowTreeMode      bool
	workflowDepths        []int
	focusPane             workflowFocusPane
	previewKind           previewKind
	activityDetailKind    activityDetailKind
	activityDetailTabs    *components.Tabs
	activityDetail        *components.Table
	activityDetailScroll  *charScrollView
	activityDetailRows    []workflowInfoRow
	hierarchyView         *WorkflowGraphView
	hierarchyGraphPanel   *components.Panel
	previewEvents         []temporal.EnhancedHistoryEvent
	previewActivities     []previewActivity
	previewWorkflowID     string
	previewRunID          string
	previewGen            uint64
	previewTimer          *time.Timer
	previewMode           bool
	previewCache          *previewCache
	allWorkflows          []temporal.Workflow // Full unfiltered list
	workflows             []temporal.Workflow // Filtered list for display
	filterText            string
	visibilityQuery       string // Temporal visibility query
	pager                 workflowPager
	pageBusy              bool
	pageGen               uint64
	serverStats           WorkflowStats
	serverStatsOK         bool
	loading               bool
	autoRefresh           bool
	liveBusy              bool
	refreshTicker         *time.Ticker
	stopRefresh           chan struct{}
	selectionMode         bool     // Multi-select mode active
	searchHistory         []string // History of visibility queries
	historyIndex          int      // Current position in history (-1 = not browsing)
	maxHistorySize        int      // Maximum number of history entries
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
	}
	wl.setup()

	// Register for automatic theme refresh
	theme.RegisterRefreshable(wl)

	return wl
}

// NewWorkflowListWithData creates a workflow list pre-populated with data (no server fetch).
func NewWorkflowListWithData(app *App, namespace string, workflows []temporal.Workflow) *WorkflowList {
	wl := &WorkflowList{
		Flex:           tview.NewFlex().SetDirection(tview.FlexRow),
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
	wl.mainFlex = tview.NewFlex().SetDirection(tview.FlexColumn)
	wl.mainFlex.SetBackgroundColor(theme.Bg())
	wl.table.SetEvaluateAllRows(true)
	wl.table.SetBorder(false)
	wl.table.SetBackgroundColor(theme.Bg())
	applyWorkflowColumnHeaders(wl.table, wl.columnLayout())
	wl.tableScroll = newCharScrollView(wl.table, func() int {
		return workflowTableContentWidth(wl.columnLayout())
	})
	bindTableCharScroll(wl.table, wl.tableScroll, func() int {
		return mouseScrollStepFromApp(wl.app)
	})
	wl.setupPreview()

	wl.workflowsPanel = components.NewPanel()
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
	wl.table.SetBackgroundColor(bg)
	wl.eventTable.SetBackgroundColor(bg)
	if wl.eventTreeView != nil {
		wl.eventTreeView.SetBackgroundColor(bg)
	}
	wl.eventDetail.SetBackgroundColor(bg)
	wl.eventDetail.SetTextColor(theme.Fg())
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

func (wl *WorkflowList) SetMasterTitle(title string) {
	name := workflowTabName(title)
	if wl.workflowTab != nil {
		wl.workflowTab.Name = name
	}
	if wl.workflowsPanel != nil && wl.listTabs == nil {
		wl.workflowsPanel.SetTitle(title)
	}
}

func (wl *WorkflowList) SetMasterContent(content tview.Primitive) {
	if content == wl.table && wl.tableScroll != nil {
		content = wl.tableScroll
	}
	if wl.workflowTab != nil {
		wl.workflowTab.Content = content
		return
	}
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetContent(content)
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
			if wl.selectionMode {
				wl.table.ToggleSelection()
				if next := wl.table.SelectedRow() + 1; next < wl.table.RowCount() {
					wl.table.SelectRow(next)
				}
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
		OnRune('s', func(e *tcell.EventKey) bool {
			wl.setListKind(listSchedules)
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
			if wl.selectionMode {
				if len(wl.table.GetSelectedRows()) > 0 {
					wl.showBatchDeleteConfirm()
				}
				return true
			}
			return false
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
		return
	}
	if wl.keepDataOnStart {
		wl.keepDataOnStart = false
		wl.restoreFocus()
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
	return wl.workflowsActive() && wl.focusPane == focusWorkflows
}

func (wl *WorkflowList) handleWorkflowScroll(event *tcell.EventKey) bool {
	if wl.tableScroll == nil || event == nil {
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
	wl.app.ClearWorkflowStats()
}

// Hints returns keybinding hints for this view.
func (wl *WorkflowList) Hints() []KeyHint {
	if wl.taskQueuesActive() {
		if wl.focusPane == focusPollers {
			return []KeyHint{
				{Key: "Enter", Description: "Show Worker"},
				{Key: "r", Description: "Refresh"},
			}
		}
		return []KeyHint{
			{Key: "/", Description: "Search"},
			{Key: "r", Description: "Refresh"},
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

	if wl.selectionMode {
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
	return wl.previewKind == previewActivities || wl.previewKind == previewEvents
}

func (wl *WorkflowList) previewShowsIO() bool {
	return wl.previewShowsSidePane()
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
	if wl.previewKind == previewEvents {
		if wl.eventTreeMode {
			hints = append(hints, KeyHint{Key: "space", Description: "Collapse/Expand"})
		}
		hints = append(hints, KeyHint{Key: "b", Description: treeModeHint(wl.eventTreeMode)})
	}
	return append(hints,
		KeyHint{Key: "p", Description: "Preview"},
	)
}

func (wl *WorkflowList) previewSideHints() []KeyHint {
	if wl.previewKind == previewDetails || wl.activityDetailTableFocused() {
		hints := []KeyHint{{Key: "y", Description: "Yank"}}
		return append(hints,
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
	return wl.previewListHints()
}

func (wl *WorkflowList) timelineHints() []KeyHint {
	return []KeyHint{
		{Key: "+/-", Description: "Zoom"},
		{Key: "m", Description: timelineSizeHint(wl.timelineNarrow)},
		{Key: "p", Description: "Preview"},
		{Key: "?", Description: "Legend"},
	}
}

func (wl *WorkflowList) workflowPaneHints() []KeyHint {
	hints := []KeyHint{
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
	hints = append(hints,
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
	return append(hints,
		KeyHint{Key: "L", Description: "Load Filter"},
		KeyHint{Key: "v", Description: "Select Mode"},
		KeyHint{Key: "N", Description: "Start"},
		KeyHint{Key: "W", Description: "Signal+Start"},
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
		if wl.previewKind == previewDetails && wl.workflowDetail != nil {
			delegate(wl.workflowDetail)
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
	if wl.eventTreeView != nil {
		wl.eventTreeView.SetBackgroundColor(bg)
	}
	if wl.eventDetail != nil {
		wl.eventDetail.SetBackgroundColor(bg)
		wl.eventDetail.SetTextColor(theme.Fg())
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
	wl.Flex.Draw(screen)
}
