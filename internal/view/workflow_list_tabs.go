package view

import (
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type listKind int

const (
	listWorkflows listKind = iota
	listTaskQueues
	listSchedules
	listWorkers
	listKindCount
)

func workflowListTabName() string {
	return "Workflows"
}

func workflowTabName(title string) string {
	title = strings.TrimSpace(title)
	if icon := theme.IconWorkflow; icon != "" && strings.HasPrefix(title, icon) {
		return strings.TrimSpace(strings.TrimPrefix(title, icon))
	}
	return title
}

func (wl *WorkflowList) setupListTabs() {
	wl.ensureTaskQueues()
	wl.ensureSchedules()
	wl.ensureWorkers()
	wl.filterBar = newFilterChipBar(wl)
	wl.workflowStack = tview.NewFlex().SetDirection(tview.FlexRow)
	wl.workflowStack.SetBackgroundColor(theme.Bg())
	wl.workflowBody = wl.tableScroll
	wl.mountWorkflowContent()
	wl.listTabs = components.NewTabs().
		SetShowIcons(true).
		SetShowBadges(false).
		AddTabWithIcon(workflowListTabName(), theme.IconWorkflow, wl.workflowStack).
		AddTabWithIcon("Task Queues", theme.IconTaskQueue, wl.taskQueues.queueScroll).
		AddTabWithIcon("Schedules", theme.IconSchedule, wl.schedules.tableScroll).
		AddTabWithIcon("Workers", theme.IconUsers, wl.workers.tableScroll).
		SetOnChange(func(index int, name string) {
			wl.setListKind(listKind(index))
		})
	wl.listTabs.SetActive(int(listWorkflows))
	wl.workflowTab = wl.listTabs.GetActiveTab()
	wl.listTabs.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) {
			return nil
		}
		if wl.handleListTabKey(event) {
			return nil
		}
		if isJigTabsNavKey(event) {
			return nil
		}
		return event
	})
	wl.workflowsPanel.SetContent(wl.listTabs)
}

func (wl *WorkflowList) handleFocusCycleKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyTab:
		wl.cycleFocus(1)
		return true
	case tcell.KeyBacktab:
		wl.cycleFocus(-1)
		return true
	}
	return false
}

func isJigTabsNavKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyTab, tcell.KeyBacktab:
		return true
	case tcell.KeyRune:
		switch event.Rune() {
		case '1', '2', '3', '4', '5', '6', '7', '8', '9', 'H', 'L':
			return true
		}
	}
	return false
}

func (wl *WorkflowList) ensureTaskQueues() {
	if wl.taskQueues == nil {
		wl.taskQueues = NewTaskQueueView(wl.app)
	}
	if wl.taskQueuesActive() {
		wl.taskQueues.Start()
		wl.bindTaskQueueKeys()
	}
}

func (wl *WorkflowList) ensureSchedules() {
	if wl.schedules == nil {
		wl.schedules = NewScheduleList(wl.app, wl.namespace)
	}
	if wl.schedulesActive() {
		if len(wl.schedules.allSchedules) == 0 {
			wl.schedules.loadData()
		}
		wl.bindScheduleKeys()
	}
}

func (wl *WorkflowList) ensureWorkers() {
	if wl.workers == nil {
		wl.workers = NewWorkerView(wl.app)
	}
	if wl.workersActive() {
		wl.workers.Start()
		wl.bindWorkerKeys()
	}
}

func (wl *WorkflowList) bindTaskQueueKeys() {
	if wl.taskQueues == nil {
		return
	}
	tq := wl.taskQueues
	tq.queueTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) || wl.handleListTabKey(event) {
			return nil
		}
		if handleTableCharScroll(tq.queueScroll, tq.queueTable, event) {
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			wl.setPollersVisible(true)
			wl.setFocusPane(focusPollers)
			return nil
		}
		switch event.Rune() {
		case '/':
			tq.showSearch()
			return nil
		case 'r':
			tq.refresh()
			return nil
		case 'a':
			tq.toggleAutoRefresh()
			return nil
		}
		return event
	})
	tq.pollerTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) {
			return nil
		}
		if handleTableCharScroll(tq.pollerScroll, tq.pollerTable, event) {
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			wl.showPollerWorker()
			return nil
		}
		switch event.Rune() {
		case 'r':
			tq.refreshCurrentQueue()
			return nil
		case 'a':
			tq.toggleAutoRefresh()
			return nil
		}
		return event
	})
	bindTableDoubleClick(tq.pollerTable)
}

// showPollerWorker opens the workers tab on the instance behind the selected
// poller.
func (wl *WorkflowList) showPollerWorker() {
	if wl.taskQueues == nil {
		return
	}
	poller, ok := wl.taskQueues.selectedPoller()
	if !ok || poller.Identity == "" {
		return
	}
	queue := wl.taskQueues.selectedQueue
	wl.setListKind(listWorkers)
	if wl.workers == nil {
		return
	}
	found := wl.workers.RevealInstance(poller.Identity, queue)
	wl.setFocusPane(focusWorkflows)
	if found || wl.app == nil {
		return
	}
	if wl.workers.loading || len(wl.workers.allWorkers) == 0 {
		// The pending request is applied once the worker list lands.
		return
	}
	wl.app.ToastWarning("No worker instance is reporting as " + poller.Identity)
}

func (wl *WorkflowList) bindScheduleKeys() {
	if wl.schedules == nil {
		return
	}
	sl := wl.schedules
	sl.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) || wl.handleListTabKey(event) {
			return nil
		}
		if handleTableCharScroll(sl.tableScroll, sl.table, event) {
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			wl.setScheduleDetailVisible(true)
			wl.setFocusPane(focusScheduleDetail)
			return nil
		}
		switch event.Rune() {
		case '/':
			sl.MasterDetailView.ShowSearch()
			return nil
		case 'r':
			sl.loadData()
			return nil
		case 'P':
			sl.showPauseConfirm()
			return nil
		case 't':
			sl.showTriggerConfirm()
			return nil
		case 'v':
			sl.viewRecentRuns()
			return nil
		case 'D':
			sl.showDeleteConfirm()
			return nil
		}
		return event
	})
	sl.detail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) {
			return nil
		}
		return sl.handleDetailKeys(event)
	})
	sl.runsTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) {
			return nil
		}
		return sl.handleRunsKeys(event)
	})
}

func (wl *WorkflowList) bindWorkerKeys() {
	if wl.workers == nil {
		return
	}
	wv := wl.workers
	wv.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) || wl.handleListTabKey(event) {
			return nil
		}
		if handleTableCharScroll(wv.tableScroll, wv.table, event) {
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			wl.setWorkerDetailVisible(true)
			wl.setFocusPane(focusWorkerDetail)
			return nil
		}
		switch event.Rune() {
		case '/':
			wv.showSearch()
			return nil
		case 'r':
			wv.loadData()
			return nil
		}
		return event
	})
	wv.detail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) || wl.handleFocusCycleKey(event) {
			return nil
		}
		if handleTableCharScroll(wv.detailScroll, wv.detail, event) {
			return nil
		}
		switch event.Rune() {
		case 'r':
			wv.loadData()
			return nil
		case 'y':
			wv.yankDetailRow()
			return nil
		}
		return event
	})
}

func (wl *WorkflowList) setPollersVisible(on bool) {
	if wl.pollersVisible == on {
		return
	}
	wl.pollersVisible = on
	if !on && wl.focusPane == focusPollers {
		wl.focusPane = focusWorkflows
	}
	wl.applyMainLayout()
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
		return
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) setScheduleDetailVisible(on bool) {
	if wl.scheduleDetailVisible == on {
		return
	}
	wl.scheduleDetailVisible = on
	if !on && (wl.focusPane == focusScheduleDetail || wl.focusPane == focusScheduleRuns) {
		wl.focusPane = focusWorkflows
	}
	wl.applyMainLayout()
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
		return
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) setWorkerDetailVisible(on bool) {
	if wl.workerDetailVisible == on {
		return
	}
	wl.workerDetailVisible = on
	if !on && wl.focusPane == focusWorkerDetail {
		wl.focusPane = focusWorkflows
	}
	wl.applyMainLayout()
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
		return
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) workflowsActive() bool {
	return wl != nil && wl.listKind == listWorkflows
}

func (wl *WorkflowList) taskQueuesActive() bool {
	return wl != nil && wl.listKind == listTaskQueues
}

func (wl *WorkflowList) schedulesActive() bool {
	return wl != nil && wl.listKind == listSchedules
}

func (wl *WorkflowList) workersActive() bool {
	return wl != nil && wl.listKind == listWorkers
}

func (wl *WorkflowList) setListKind(kind listKind) {
	if kind < listWorkflows || kind >= listKindCount {
		return
	}
	changing := wl.listKind != kind
	prev := wl.listKind
	wl.listKind = kind
	if wl.listTabs != nil && wl.listTabs.GetActive() != int(kind) {
		wl.listTabs.SetActive(int(kind))
	}
	if !changing {
		return
	}
	if prev == listTaskQueues && wl.taskQueues != nil {
		wl.taskQueues.Stop()
	}
	switch kind {
	case listTaskQueues:
		wl.ensureTaskQueues()
		wl.focusPane = focusWorkflows
	case listSchedules:
		wl.ensureSchedules()
		wl.focusPane = focusWorkflows
	case listWorkers:
		wl.ensureWorkers()
		wl.focusPane = focusWorkflows
	default:
		if wl.focusPane == focusPollers || wl.focusPane == focusScheduleDetail ||
			wl.focusPane == focusScheduleRuns || wl.focusPane == focusWorkerDetail {
			wl.focusPane = focusWorkflows
		}
	}
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.app.updateCrumbs()
	}
	wl.updateStats()
	wl.applyPreviewLayout()
}

func (wl *WorkflowList) cycleListKind(delta int) {
	next := (int(wl.listKind) + delta) % int(listKindCount)
	if next < 0 {
		next += int(listKindCount)
	}
	wl.setListKind(listKind(next))
}

func (wl *WorkflowList) handleListTabKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	if wl.focusPane != focusWorkflows && wl.focusPane != focusFilters {
		return false
	}
	switch event.Rune() {
	case '[':
		wl.cycleListKind(-1)
		return true
	case ']':
		wl.cycleListKind(1)
		return true
	case '1':
		wl.setListKind(listWorkflows)
		return true
	case '2':
		wl.setListKind(listTaskQueues)
		return true
	case '3':
		wl.setListKind(listSchedules)
		return true
	case '4':
		wl.setListKind(listWorkers)
		return true
	}
	return false
}

func listTabWidth(name, icon string) int {
	width := 2 + len(name)
	if icon != "" {
		width += len(icon) + 1
	}
	return width
}

func (wl *WorkflowList) listTabAt(x, y int) (listKind, bool) {
	if wl == nil || wl.listTabs == nil {
		return 0, false
	}
	tx, ty, tw, _ := wl.listTabs.GetInnerRect()
	if tw <= 0 || y != ty || x < tx || x >= tx+tw {
		return 0, false
	}
	names := [listKindCount]string{"Workflows", "Task Queues", "Schedules", "Workers"}
	if wl.workflowTab != nil && wl.workflowTab.Name != "" {
		names[0] = wl.workflowTab.Name
	}
	icons := [listKindCount]string{theme.IconWorkflow, theme.IconTaskQueue, theme.IconSchedule, theme.IconUsers}
	col := tx
	for i, name := range names {
		width := listTabWidth(name, icons[i])
		if x >= col && x < col+width {
			return listKind(i), true
		}
		col += width + 1
	}
	return 0, false
}
