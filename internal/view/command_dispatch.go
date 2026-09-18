package view

import (
	"strings"

	"github.com/atterpac/jig/components"
)

func (a *App) executeBuiltinCommand(fields []string) bool {
	if a == nil || len(fields) == 0 {
		return false
	}
	verb := strings.ToLower(fields[0])
	args := fields[1:]
	switch verb {
	case "quit":
		a.Stop()
	case "help":
		a.showHelp()
	case "command":
		a.showCommandBar()
	case "debug":
		a.showDebugScreen()
	case "refresh":
		a.execRefresh()
	case "auto":
		a.execAuto(args)
	case "profile":
		a.handleProfileCommand(strings.Join(args, " "))
	case "config":
		a.execConfig(args)
	case "nav":
		a.execNav(args)
	case "focus":
		a.execFocus(args)
	case "tab":
		a.execTab(args)
	case "preview":
		a.execPreview(args)
	case "timeline":
		a.execTimeline(args)
	case "tree":
		a.execTree(args)
	case "fold":
		a.execFold(args)
	case "columns":
		a.execColumns()
	case "legend":
		a.showTimelineLegend()
	case "io":
		a.execIO()
	case "editor":
		a.execEditor()
	case "ui":
		a.execUI()
	case "search":
		a.execSearch()
	case "filter":
		a.execFilter(args)
	case "query":
		a.execQuery(args)
	case "date":
		a.execDate()
	case "workflow":
		a.execWorkflow(args)
	case "namespace":
		a.execNamespace(args)
	case "schedule":
		a.execSchedule(args)
	case "worker":
		a.execWorker(args)
	case "poller":
		a.execPoller(args)
	case "yank":
		a.execYank()
	case "delete":
		a.execDelete()
	case "back":
		a.execBack()
	default:
		return false
	}
	return true
}

func (a *App) workflowList() (*WorkflowList, bool) {
	if a == nil {
		return nil, false
	}
	wl, ok := a.currentContent().(*WorkflowList)
	return wl, ok
}

func (a *App) namespaceListView() (*NamespaceList, bool) {
	if a == nil {
		return nil, false
	}
	nl, ok := a.currentContent().(*NamespaceList)
	return nl, ok
}

func (a *App) requireWorkflowList(usage string) (*WorkflowList, bool) {
	wl, ok := a.workflowList()
	if !ok {
		a.ToastWarning(usage)
		return nil, false
	}
	return wl, true
}

func parseOnOffToggle(args []string, current bool) (bool, bool) {
	if len(args) == 0 {
		return !current, true
	}
	switch strings.ToLower(args[0]) {
	case "on":
		return true, true
	case "off":
		return false, true
	case "toggle":
		return !current, true
	default:
		return false, false
	}
}

func (a *App) execRefresh() {
	if wl, ok := a.workflowList(); ok {
		switch {
		case wl.taskQueuesActive() && wl.taskQueues != nil:
			wl.taskQueues.refresh()
		case wl.schedulesActive() && wl.schedules != nil:
			wl.schedules.loadData()
		case wl.workersActive() && wl.workers != nil:
			wl.workers.loadData()
		default:
			wl.refresh()
		}
		return
	}
	if nl, ok := a.namespaceListView(); ok {
		nl.loadData()
		return
	}
	a.ToastWarning("Nothing to refresh")
}

func (a *App) execAuto(args []string) {
	if wl, ok := a.workflowList(); ok {
		if wl.taskQueuesActive() && wl.taskQueues != nil {
			on, ok := parseOnOffToggle(args, wl.taskQueues.autoRefresh)
			if !ok {
				a.ToastWarning("usage: auto on|off|toggle")
				return
			}
			if wl.taskQueues.autoRefresh != on {
				wl.taskQueues.toggleAutoRefresh()
			}
			return
		}
		on, ok := parseOnOffToggle(args, wl.autoRefresh)
		if !ok {
			a.ToastWarning("usage: auto on|off|toggle")
			return
		}
		if wl.autoRefresh != on {
			wl.toggleAutoRefresh()
		}
		return
	}
	if nl, ok := a.namespaceListView(); ok {
		on, ok := parseOnOffToggle(args, nl.autoRefresh)
		if !ok {
			a.ToastWarning("usage: auto on|off|toggle")
			return
		}
		if nl.autoRefresh != on {
			nl.toggleAutoRefresh()
		}
		return
	}
	a.ToastWarning("usage: auto on|off|toggle")
}

func (a *App) execNav(args []string) {
	if len(args) == 0 {
		a.ToastWarning("usage: nav up|down|top|bottom|page up|page down|left|right")
		return
	}
	cmd := strings.ToLower(args[0])
	if cmd == "page" && len(args) > 1 {
		cmd = "page " + strings.ToLower(args[1])
	}
	if wl, ok := a.workflowList(); ok && wl.workflowsActive() && wl.focusPane == focusWorkflows {
		switch cmd {
		case "top":
			wl.jumpWorkflowListEdge(false)
			return
		case "bottom":
			wl.jumpWorkflowListEdge(true)
			return
		case "left":
			if wl.tableScroll != nil {
				wl.tableScroll.scrollChars(-1)
			}
			return
		case "right":
			if wl.tableScroll != nil {
				wl.tableScroll.scrollChars(1)
			}
			return
		}
	}
	table := a.focusedTable()
	if table == nil {
		a.ToastWarning("Nothing to navigate")
		return
	}
	row := table.SelectedRow()
	count := table.RowCount()
	switch cmd {
	case "up":
		if row > 0 {
			table.SelectRow(row - 1)
		}
	case "down":
		if row+1 < count {
			table.SelectRow(row + 1)
		}
	case "top":
		table.SelectRow(0)
	case "bottom":
		if count > 0 {
			table.SelectRow(count - 1)
		}
	case "page up":
		next := row - 10
		if next < 0 {
			next = 0
		}
		table.SelectRow(next)
	case "page down":
		next := row + 10
		if count > 0 && next >= count {
			next = count - 1
		}
		table.SelectRow(next)
	case "left", "right":
		a.ToastWarning("usage: nav left|right")
	default:
		a.ToastWarning("usage: nav up|down|top|bottom|page up|page down|left|right")
	}
}

func (a *App) focusedTable() *components.Table {
	if wl, ok := a.workflowList(); ok {
		switch wl.focusPane {
		case focusEvents:
			return wl.eventTable
		case focusEventDetail:
			if wl.previewKind == previewActivities {
				return wl.activityDetail
			}
			if wl.previewKind == previewDetails {
				return wl.workflowDetail
			}
		case focusPollers:
			if wl.taskQueues != nil {
				return wl.taskQueues.pollerTable
			}
		case focusScheduleDetail:
			if wl.schedules != nil {
				return wl.schedules.detail
			}
		case focusScheduleRuns:
			if wl.schedules != nil {
				return wl.schedules.runsTable
			}
		case focusWorkerDetail:
			if wl.workers != nil {
				return wl.workers.detail
			}
		default:
			switch {
			case wl.taskQueuesActive() && wl.taskQueues != nil:
				return wl.taskQueues.queueTable
			case wl.schedulesActive() && wl.schedules != nil:
				return wl.schedules.table
			case wl.workersActive() && wl.workers != nil:
				return wl.workers.table
			default:
				return wl.table
			}
		}
	}
	if nl, ok := a.namespaceListView(); ok {
		return nl.table
	}
	return nil
}

func (a *App) execFocus(args []string) {
	wl, ok := a.requireWorkflowList("usage: focus cycle|prev|list|preview|timeline|detail|pollers")
	if !ok {
		return
	}
	if len(args) == 0 {
		wl.cycleFocus(1)
		return
	}
	switch strings.ToLower(args[0]) {
	case "cycle":
		wl.cycleFocus(1)
	case "prev":
		wl.cycleFocus(-1)
	case "list":
		wl.setFocusPane(focusWorkflows)
	case "preview":
		if !wl.workflowsActive() || !wl.previewModeEnabled() {
			a.ToastWarning("Preview is hidden")
			return
		}
		wl.setFocusPane(focusEventDetail)
	case "timeline":
		if !wl.timelineVisible {
			a.ToastWarning("Timeline is hidden")
			return
		}
		wl.setFocusPane(focusTimeline)
	case "detail":
		switch {
		case wl.schedulesActive():
			wl.setScheduleDetailVisible(true)
			wl.setFocusPane(focusScheduleDetail)
		case wl.workersActive():
			wl.setWorkerDetailVisible(true)
			wl.setFocusPane(focusWorkerDetail)
		case wl.workflowsActive() && wl.previewModeEnabled():
			wl.setFocusPane(focusEventDetail)
		default:
			a.ToastWarning("No detail pane")
		}
	case "pollers":
		if !wl.taskQueuesActive() {
			a.ToastWarning("usage: focus pollers")
			return
		}
		wl.setPollersVisible(true)
		wl.setFocusPane(focusPollers)
	default:
		a.ToastWarning("usage: focus cycle|prev|list|preview|timeline|detail|pollers")
	}
}

func (a *App) execTab(args []string) {
	if len(args) == 0 {
		a.ToastWarning("usage: tab workflows|queues|schedules|workers")
		return
	}
	var kind listKind
	switch strings.ToLower(args[0]) {
	case "workflows":
		kind = listWorkflows
	case "queues":
		kind = listTaskQueues
	case "schedules":
		kind = listSchedules
	case "workers":
		kind = listWorkers
	default:
		a.ToastWarning("usage: tab workflows|queues|schedules|workers")
		return
	}
	if wl, ok := a.workflowList(); ok {
		wl.setListKind(kind)
		return
	}
	switch kind {
	case listTaskQueues:
		a.NavigateToTaskQueues()
	case listSchedules:
		a.NavigateToSchedules()
	case listWorkers:
		a.NavigateToWorkers()
	default:
		a.NavigateToWorkflows(a.CurrentNamespace())
	}
}

func (a *App) execPreview(args []string) {
	if nl, ok := a.namespaceListView(); ok {
		if len(args) == 0 {
			nl.togglePreview()
			return
		}
		a.ToastWarning("usage: preview")
		return
	}
	wl, ok := a.requireWorkflowList("usage: preview on|off|toggle|details|activities|events|hierarchy")
	if !ok {
		return
	}
	if !wl.workflowsActive() {
		a.ToastWarning("usage: preview on|off|toggle|details|activities|events|hierarchy")
		return
	}
	if len(args) == 0 {
		wl.togglePreviewMode()
		return
	}
	switch strings.ToLower(args[0]) {
	case "on", "off", "toggle":
		on, ok := parseOnOffToggle(args, wl.previewMode)
		if !ok {
			a.ToastWarning("usage: preview on|off|toggle")
			return
		}
		wl.setPreviewVisible(on)
	case "details":
		wl.setPreviewVisible(true)
		wl.setPreviewKind(previewDetails)
	case "activities":
		wl.setPreviewVisible(true)
		wl.setPreviewKind(previewActivities)
	case "events":
		wl.setPreviewVisible(true)
		wl.setPreviewKind(previewEvents)
	case "hierarchy":
		wl.setPreviewVisible(true)
		wl.setPreviewKind(previewHierarchy)
	default:
		a.ToastWarning("usage: preview on|off|toggle|details|activities|events|hierarchy")
	}
}

func (a *App) execTimeline(args []string) {
	wl, ok := a.requireWorkflowList("usage: timeline on|off|toggle|wide|narrow")
	if !ok {
		return
	}
	if len(args) == 0 {
		wl.toggleTimeline()
		return
	}
	switch strings.ToLower(args[0]) {
	case "on", "off", "toggle":
		on, ok := parseOnOffToggle(args, wl.timelineVisible)
		if !ok {
			a.ToastWarning("usage: timeline on|off|toggle")
			return
		}
		if wl.timelineVisible != on {
			wl.toggleTimeline()
		}
	case "wide":
		if wl.timelineNarrow {
			wl.toggleTimelineSize()
		}
	case "narrow":
		if !wl.timelineNarrow {
			wl.toggleTimelineSize()
		}
	default:
		a.ToastWarning("usage: timeline on|off|toggle|wide|narrow")
	}
}

func (a *App) execTree(args []string) {
	wl, ok := a.requireWorkflowList("usage: tree on|off|toggle")
	if !ok {
		return
	}
	on, ok := parseOnOffToggle(args, wl.workflowTreeMode)
	if !ok {
		a.ToastWarning("usage: tree on|off|toggle")
		return
	}
	if wl.workflowTreeMode != on {
		wl.toggleWorkflowTree()
	}
}

func (a *App) execFold(args []string) {
	wl, ok := a.requireWorkflowList("usage: fold on|off|toggle|all")
	if !ok {
		return
	}
	if !wl.workflowTreeMode {
		a.ToastWarning("Tree view is off")
		return
	}
	all := false
	rest := args
	if len(args) > 0 && strings.EqualFold(args[0], "all") {
		all = true
		rest = args[1:]
	}
	if all {
		folded := wl.anyWorkflowFolded()
		on, ok := parseOnOffToggle(rest, folded)
		if !ok {
			a.ToastWarning("usage: fold all on|off|toggle")
			return
		}
		if folded != on {
			wl.toggleWorkflowTreeFoldAll()
		}
		return
	}
	row := -1
	if wl.table != nil {
		row = wl.table.SelectedRow()
	}
	collapsed := false
	if row >= 0 && row < len(wl.workflows) {
		collapsed = wl.workflowCollapsed[workflowIdentityKey(wl.workflows[row])]
	}
	on, ok := parseOnOffToggle(rest, collapsed)
	if !ok {
		a.ToastWarning("usage: fold on|off|toggle")
		return
	}
	if collapsed != on {
		wl.toggleWorkflowTreeFold()
	}
}

func (a *App) execColumns() {
	wl, ok := a.requireWorkflowList("usage: columns")
	if !ok {
		return
	}
	wl.showColumnEditor()
}

func (a *App) execIO() {
	wl, ok := a.requireWorkflowList("usage: io")
	if !ok {
		return
	}
	if !wl.showPreviewIO() {
		a.ToastWarning("No input/output")
	}
}

func (a *App) execEditor() {
	wl, ok := a.requireWorkflowList("usage: editor")
	if !ok {
		return
	}
	if !wl.openPreviewIOInEditor() {
		a.ToastWarning("No payload to edit")
	}
}

func (a *App) execUI() {
	wl, ok := a.requireWorkflowList("usage: ui")
	if !ok {
		return
	}
	wl.openSelectedWorkflowUI()
}

func (a *App) execSearch() {
	if wl, ok := a.workflowList(); ok {
		switch {
		case wl.taskQueuesActive() && wl.taskQueues != nil:
			wl.taskQueues.showSearch()
		case wl.schedulesActive() && wl.schedules != nil:
			wl.schedules.ShowSearch()
		case wl.workersActive() && wl.workers != nil:
			wl.workers.showSearch()
		default:
			wl.showFilter()
		}
		return
	}
	if nl, ok := a.namespaceListView(); ok {
		nl.ShowSearch()
		return
	}
	a.ToastWarning("Nothing to search")
}

func (a *App) execFilter(args []string) {
	wl, ok := a.requireWorkflowList("usage: filter save|load")
	if !ok {
		return
	}
	if !wl.workflowsActive() {
		a.ToastWarning("usage: filter save|load")
		return
	}
	if len(args) == 0 {
		wl.showFilter()
		return
	}
	switch strings.ToLower(args[0]) {
	case "save":
		if wl.visibilityQuery == "" {
			a.ToastWarning("No query to save")
			return
		}
		wl.showSaveFilter()
	case "load":
		if len(args) > 1 {
			a.loadSavedFilter(wl, strings.Join(args[1:], " "))
			return
		}
		wl.showSavedFilters()
	default:
		a.ToastWarning("usage: filter save|load")
	}
}

func (a *App) loadSavedFilter(wl *WorkflowList, name string) {
	if a.config == nil {
		a.ToastWarning("No saved filters")
		return
	}
	for _, f := range a.config.GetSavedFilters() {
		if strings.EqualFold(strings.TrimSpace(f.Name), name) {
			wl.applyVisibilityQuery(f.Query)
			return
		}
	}
	a.ToastWarning("Unknown filter " + name)
}

func (a *App) execQuery(args []string) {
	wl, ok := a.requireWorkflowList("usage: query templates|clear")
	if !ok {
		return
	}
	if !wl.workflowsActive() {
		a.ToastWarning("usage: query templates|clear")
		return
	}
	if len(args) == 0 {
		wl.showVisibilityQuery()
		return
	}
	switch strings.ToLower(args[0]) {
	case "templates":
		wl.showQueryTemplates()
	case "clear":
		wl.clearVisibilityQuery()
	default:
		a.ToastWarning("usage: query templates|clear")
	}
}

func (a *App) execDate() {
	wl, ok := a.requireWorkflowList("usage: date")
	if !ok {
		return
	}
	if !wl.workflowsActive() {
		a.ToastWarning("usage: date")
		return
	}
	wl.showDateRangePicker()
}

func (a *App) execWorkflow(args []string) {
	if len(args) == 0 {
		a.ToastWarning("usage: workflow start|cancel|terminate|signal|query|reset|delete|yank|select")
		return
	}
	cmd := strings.ToLower(args[0])
	if cmd == "start" {
		if wl, ok := a.workflowList(); ok {
			wl.showStartWorkflow()
			return
		}
		if nl, ok := a.namespaceListView(); ok {
			if ns := nl.getSelectedNamespace(); ns != nil {
				showStartWorkflowModal(a, startWorkflowPrefill{Namespace: ns.Name})
				return
			}
		}
		showStartWorkflowModal(a, startWorkflowPrefill{Namespace: a.CurrentNamespace()})
		return
	}
	wl, ok := a.requireWorkflowList("usage: workflow " + cmd)
	if !ok {
		return
	}
	if !wl.workflowsActive() {
		a.ToastWarning("usage: workflow " + cmd)
		return
	}
	switch cmd {
	case "cancel":
		if wl.selectionMode && len(wl.table.GetSelectedRows()) > 0 {
			wl.showBatchCancelConfirm()
			return
		}
		wl.showCancelSelected()
	case "terminate":
		if wl.selectionMode && len(wl.table.GetSelectedRows()) > 0 {
			wl.showBatchTerminateConfirm()
			return
		}
		wl.showTerminateSelected()
	case "signal":
		wl.showSignalSelected()
	case "query":
		wl.showQuerySelected()
	case "reset":
		wl.showResetSelected()
	case "delete":
		if wl.selectionMode && len(wl.table.GetSelectedRows()) > 0 {
			wl.showBatchDeleteConfirm()
			return
		}
		wl.showDeleteSelected()
	case "yank":
		wl.copyWorkflowID()
	case "select":
		a.execWorkflowSelect(wl, args[1:])
	default:
		a.ToastWarning("usage: workflow start|cancel|terminate|signal|query|reset|delete|yank|select")
	}
}

func (a *App) execWorkflowSelect(wl *WorkflowList, args []string) {
	if len(args) > 0 && strings.EqualFold(args[0], "all") {
		if !wl.selectionMode {
			wl.toggleSelectionMode()
		}
		wl.table.SelectAll()
		wl.updateSelectionPreview()
		return
	}
	on, ok := parseOnOffToggle(args, wl.selectionMode)
	if !ok {
		a.ToastWarning("usage: workflow select on|off|toggle|all")
		return
	}
	if wl.selectionMode != on {
		wl.toggleSelectionMode()
	}
}

func (a *App) execNamespace(args []string) {
	nl, ok := a.namespaceListView()
	if !ok {
		a.ToastWarning("usage: namespace create|edit|deprecate|delete|info|preview|workflows|start")
		return
	}
	if len(args) == 0 {
		a.ToastWarning("usage: namespace create|edit|deprecate|delete|info|preview|workflows|start")
		return
	}
	switch strings.ToLower(args[0]) {
	case "create":
		nl.showCreateNamespaceForm()
	case "edit":
		nl.showEditNamespaceForm()
	case "deprecate":
		nl.showDeprecateConfirm()
	case "delete":
		nl.showDeleteConfirm()
	case "info":
		if ns := nl.getSelectedNamespace(); ns != nil {
			a.NavigateToNamespaceDetail(ns.Name)
			return
		}
		a.ToastWarning("No namespace selected")
	case "preview":
		nl.togglePreview()
	case "workflows":
		if ns := nl.getSelectedNamespace(); ns != nil {
			a.NavigateToWorkflows(ns.Name)
			return
		}
		a.ToastWarning("No namespace selected")
	case "start":
		if ns := nl.getSelectedNamespace(); ns != nil {
			showStartWorkflowModal(a, startWorkflowPrefill{Namespace: ns.Name})
			return
		}
		a.ToastWarning("No namespace selected")
	default:
		a.ToastWarning("usage: namespace create|edit|deprecate|delete|info|preview|workflows|start")
	}
}

func (a *App) execSchedule(args []string) {
	wl, ok := a.workflowList()
	if !ok || !wl.schedulesActive() || wl.schedules == nil {
		a.ToastWarning("usage: schedule pause|trigger|runs|delete|yank")
		return
	}
	if len(args) == 0 {
		a.ToastWarning("usage: schedule pause|trigger|runs|delete|yank")
		return
	}
	sl := wl.schedules
	switch strings.ToLower(args[0]) {
	case "pause":
		s := sl.getSelectedSchedule()
		if s == nil {
			a.ToastWarning("No schedule selected")
			return
		}
		on, ok := parseOnOffToggle(args[1:], s.Paused)
		if !ok {
			a.ToastWarning("usage: schedule pause on|off|toggle")
			return
		}
		if on {
			sl.showPauseConfirm()
		} else {
			sl.showUnpauseConfirm(s)
		}
	case "trigger":
		sl.showTriggerConfirm()
	case "runs":
		sl.viewRecentRuns()
	case "delete":
		sl.showDeleteConfirm()
	case "yank":
		s := sl.getSelectedSchedule()
		if s == nil {
			a.ToastWarning("No schedule selected")
			return
		}
		if err := copyToClipboard(s.ID); err != nil {
			a.ToastError("Failed to copy: " + err.Error())
			return
		}
		a.ToastSuccess("Copied schedule ID")
	default:
		a.ToastWarning("usage: schedule pause|trigger|runs|delete|yank")
	}
}

func (a *App) execWorker(args []string) {
	wl, ok := a.workflowList()
	if !ok || !wl.workersActive() || wl.workers == nil {
		a.ToastWarning("usage: worker detail|yank")
		return
	}
	if len(args) == 0 {
		a.ToastWarning("usage: worker detail|yank")
		return
	}
	switch strings.ToLower(args[0]) {
	case "detail":
		wl.setWorkerDetailVisible(true)
		wl.setFocusPane(focusWorkerDetail)
	case "yank":
		if wl.focusPane == focusWorkerDetail {
			wl.workers.yankDetailRow()
			return
		}
		row, ok := wl.workers.selectedRow()
		if !ok {
			a.ToastWarning("No worker selected")
			return
		}
		if err := copyToClipboard(row.Worker.Identity); err != nil {
			a.ToastError("Failed to copy: " + err.Error())
			return
		}
		a.ToastSuccess("Copied worker identity")
	default:
		a.ToastWarning("usage: worker detail|yank")
	}
}

func (a *App) execPoller(args []string) {
	wl, ok := a.workflowList()
	if !ok || !wl.taskQueuesActive() {
		a.ToastWarning("usage: poller worker")
		return
	}
	if len(args) == 0 || !strings.EqualFold(args[0], "worker") {
		a.ToastWarning("usage: poller worker")
		return
	}
	wl.showPollerWorker()
}

func (a *App) execYank() {
	if wl, ok := a.workflowList(); ok {
		switch {
		case wl.schedulesActive():
			a.execSchedule([]string{"yank"})
		case wl.workersActive():
			a.execWorker([]string{"yank"})
		case wl.taskQueuesActive() && wl.taskQueues != nil:
			name := wl.taskQueues.selectedQueueName()
			if name == "" {
				a.ToastWarning("No task queue selected")
				return
			}
			if err := copyToClipboard(name); err != nil {
				a.ToastError("Failed to copy: " + err.Error())
				return
			}
			a.ToastSuccess("Copied task queue")
		default:
			wl.copyWorkflowID()
		}
		return
	}
	if nl, ok := a.namespaceListView(); ok {
		if ns := nl.getSelectedNamespace(); ns != nil {
			if err := copyToClipboard(ns.Name); err != nil {
				a.ToastError("Failed to copy: " + err.Error())
				return
			}
			a.ToastSuccess("Copied namespace")
			return
		}
	}
	a.ToastWarning("Nothing to copy")
}

func (a *App) execDelete() {
	if wl, ok := a.workflowList(); ok {
		switch {
		case wl.schedulesActive():
			a.execSchedule([]string{"delete"})
		case wl.workflowsActive():
			a.execWorkflow([]string{"delete"})
		default:
			a.ToastWarning("Nothing to delete")
		}
		return
	}
	if _, ok := a.namespaceListView(); ok {
		a.execNamespace([]string{"delete"})
		return
	}
	a.ToastWarning("Nothing to delete")
}

func (a *App) execBack() {
	if a.app == nil || a.app.Pages() == nil {
		return
	}
	if current := a.app.Pages().Current(); current != nil {
		if handler, ok := current.(EscapeHandler); ok && handler.HandleEscape() {
			return
		}
	}
	if a.app.Pages().CanPop() {
		a.app.Pages().Pop()
		a.refocusCurrent()
	}
}
