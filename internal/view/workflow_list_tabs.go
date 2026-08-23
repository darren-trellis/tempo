package view

import (
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

type listKind int

const (
	listWorkflows listKind = iota
	listTaskQueues
)

func workflowTabName(title string) string {
	title = strings.TrimSpace(title)
	if icon := theme.IconWorkflow; icon != "" && strings.HasPrefix(title, icon) {
		return strings.TrimSpace(strings.TrimPrefix(title, icon))
	}
	return title
}

func (wl *WorkflowList) setupListTabs() {
	wl.ensureTaskQueues()
	wl.listTabs = components.NewTabs().
		SetShowIcons(true).
		SetShowBadges(false).
		AddTabWithIcon("Workflows (List)", theme.IconWorkflow, wl.tableScroll).
		AddTabWithIcon("Task Queues", theme.IconTaskQueue, wl.taskQueues.queueTable).
		SetOnChange(func(index int, name string) {
			if index == int(listTaskQueues) {
				wl.setListKind(listTaskQueues)
				return
			}
			wl.setListKind(listWorkflows)
		})
	wl.listTabs.SetActive(int(listWorkflows))
	wl.workflowTab = wl.listTabs.GetActiveTab()
	wl.listTabs.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handleFocusCycleKey(event) {
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

func (wl *WorkflowList) bindTaskQueueKeys() {
	if wl.taskQueues == nil {
		return
	}
	tq := wl.taskQueues
	tq.queueTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handleFocusCycleKey(event) || wl.handleListTabKey(event) {
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
			tq.refreshCurrentQueue()
			return nil
		}
		return event
	})
	tq.pollerTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handleFocusCycleKey(event) {
			return nil
		}
		if event.Rune() == 'r' {
			tq.refreshCurrentQueue()
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

func (wl *WorkflowList) taskQueuesActive() bool {
	return wl != nil && wl.listKind == listTaskQueues
}

func (wl *WorkflowList) setListKind(kind listKind) {
	if kind != listWorkflows && kind != listTaskQueues {
		return
	}
	changing := wl.listKind != kind
	wl.listKind = kind
	if wl.listTabs != nil && wl.listTabs.GetActive() != int(kind) {
		wl.listTabs.SetActive(int(kind))
	}
	if !changing {
		return
	}
	if kind == listTaskQueues {
		wl.ensureTaskQueues()
		wl.focusPane = focusWorkflows
	} else if wl.focusPane == focusPollers {
		wl.focusPane = focusWorkflows
	}
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.app.updateCrumbs()
	}
	wl.applyPreviewLayout()
}

func (wl *WorkflowList) cycleListKind(delta int) {
	next := (int(wl.listKind) + delta) % 2
	if next < 0 {
		next += 2
	}
	wl.setListKind(listKind(next))
}

func (wl *WorkflowList) handleListTabKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	if wl.focusPane != focusWorkflows {
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
	names := [2]string{"Workflows (List)", "Task Queues"}
	if wl.workflowTab != nil && wl.workflowTab.Name != "" {
		names[0] = wl.workflowTab.Name
	}
	icons := [2]string{theme.IconWorkflow, theme.IconTaskQueue}
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
