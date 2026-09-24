package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (wl *WorkflowList) paneAt(x, y int) (workflowFocusPane, bool) {
	if wl.timelineVisible && wl.timelinePanel != nil && wl.timelinePanel.InRect(x, y) {
		return focusTimeline, true
	}
	if wl.filtersOnSide() && wl.workflowsActive() && wl.filterBar != nil && wl.filterBar.InRect(x, y) {
		return focusFilters, true
	}
	if !wl.workflowsActive() {
		if wl.taskQueuesActive() && wl.pollersVisible && wl.taskQueues != nil && wl.taskQueues.pollerPanel != nil && wl.taskQueues.pollerPanel.InRect(x, y) {
			return focusPollers, true
		}
		if wl.schedulesActive() && wl.scheduleDetailVisible && wl.schedules != nil {
			if wl.schedules.runsPanel != nil && wl.schedules.runsPanel.InRect(x, y) {
				return focusScheduleRuns, true
			}
			if wl.schedules.detailPanel != nil && wl.schedules.detailPanel.InRect(x, y) {
				return focusScheduleDetail, true
			}
		}
		if wl.workersActive() && wl.workerDetailVisible && wl.workers != nil && wl.workers.detailFlex != nil && wl.workers.detailFlex.InRect(x, y) {
			return focusWorkerDetail, true
		}
		if wl.workflowsPanel != nil && wl.workflowsPanel.InRect(x, y) {
			return focusWorkflows, true
		}
	}
	if wl.previewModeEnabled() {
		if _, ok := wl.previewTabAt(x, y); ok {
			return focusWorkflows, false
		}
		if wl.previewKind == previewDetails {
			if wl.eventDetailPanel != nil && wl.eventDetailPanel.InRect(x, y) {
				return focusEventDetail, true
			}
			if wl.workflowDetailScroll != nil && wl.workflowDetailScroll.InRect(x, y) {
				return focusEvents, true
			}
			if wl.workflowDetail != nil && wl.workflowDetail.InRect(x, y) {
				return focusEvents, true
			}
		} else if wl.previewKind == previewHierarchy {
			if wl.hierarchyGraphPanel != nil && wl.hierarchyGraphPanel.InRect(x, y) {
				return focusEventDetail, true
			}
			if wl.hierarchyView != nil && wl.hierarchyView.tree != nil && wl.hierarchyView.tree.InRect(x, y) {
				return focusEvents, true
			}
		} else {
			if wl.eventDetailPanel != nil && wl.eventDetailPanel.InRect(x, y) {
				return focusEventDetail, true
			}
			if wl.previewKind == previewEvents && wl.eventTreeMode && wl.eventTreeView != nil && wl.eventTreeView.InRect(x, y) {
				return focusEvents, true
			}
			if wl.eventTableScroll != nil && wl.eventTableScroll.InRect(x, y) {
				return focusEvents, true
			}
			if wl.eventTable != nil && wl.eventTable.InRect(x, y) {
				return focusEvents, true
			}
		}
	}
	if wl.workflowsPanel != nil && wl.workflowsPanel.InRect(x, y) {
		return focusWorkflows, true
	}
	return focusWorkflows, false
}

func (wl *WorkflowList) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return wl.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		if !wl.InRect(x, y) {
			return false, nil
		}

		if wl.timelineVisible && wl.timelinePanel != nil && timelineSizeButtonHit(wl.timelinePanel, x, y) {
			if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
				wl.toggleTimelineSize()
				return true, nil
			}
		}

		if kind, ok := wl.listTabAt(x, y); ok {
			if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
				wl.setListKind(kind)
				return true, nil
			}
		}

		if kind, ok := wl.previewTabAt(x, y); ok {
			if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
				wl.setPreviewKind(kind)
				return true, nil
			}
		}

		if pane, ok := wl.paneAt(x, y); ok {
			switch action {
			case tview.MouseLeftDown, tview.MouseLeftClick:
				if pane != wl.focusPane {
					wl.setFocusPane(pane)
				}
			}
		}

		consumed, capture := wl.Flex.MouseHandler()(action, event, setFocus)
		if action == tview.MouseLeftDoubleClick && consumed {
			if _, ok := wl.listTabAt(x, y); ok {
				return consumed, capture
			}
			if pane, ok := wl.paneAt(x, y); ok && pane == focusWorkflows {
				wl.activateSelectedWorkflow()
			}
		}
		return consumed, capture
	})
}

func (wl *WorkflowList) previewFocusOrder() []workflowFocusPane {
	if wl.taskQueuesActive() {
		if wl.pollersVisible {
			return []workflowFocusPane{focusWorkflows, focusPollers}
		}
		return []workflowFocusPane{focusWorkflows}
	}
	if wl.schedulesActive() {
		if wl.scheduleDetailVisible {
			return []workflowFocusPane{focusWorkflows, focusScheduleDetail, focusScheduleRuns}
		}
		return []workflowFocusPane{focusWorkflows}
	}
	if wl.workersActive() {
		if wl.workerDetailVisible {
			return []workflowFocusPane{focusWorkflows, focusWorkerDetail}
		}
		return []workflowFocusPane{focusWorkflows}
	}
	order := []workflowFocusPane{}
	if wl.filtersOnSide() {
		order = append(order, focusFilters)
	}
	order = append(order, focusWorkflows)
	if wl.previewModeEnabled() {
		if wl.previewShowsSidePane() || wl.previewKind == previewHierarchy {
			order = append(order, focusEvents, focusEventDetail)
		} else {
			order = append(order, focusEventDetail)
		}
	}
	if wl.timelineVisible {
		order = append(order, focusTimeline)
	}
	return order
}

func (wl *WorkflowList) cycleFocus(delta int) {
	order := wl.previewFocusOrder()
	if len(order) == 0 {
		return
	}
	idx := 0
	for i, pane := range order {
		if pane == wl.focusPane {
			idx = i
			break
		}
	}
	next := (idx + delta) % len(order)
	if next < 0 {
		next += len(order)
	}
	wl.setFocusPane(order[next])
}

func (wl *WorkflowList) setFocusPane(pane workflowFocusPane) {
	if pane == focusTimeline {
		wl.syncTimelineFromActivity()
	}
	wl.focusPane = pane
	if wl.app == nil || wl.app.JigApp() == nil {
		wl.applyFocusStyles()
		return
	}
	switch pane {
	case focusPollers:
		if wl.taskQueues != nil {
			wl.app.JigApp().SetFocus(wl.taskQueues.pollerTable)
		}
	case focusScheduleDetail:
		if wl.schedules != nil {
			wl.app.JigApp().SetFocus(wl.schedules.detail)
		}
	case focusScheduleRuns:
		if wl.schedules != nil {
			wl.app.JigApp().SetFocus(wl.schedules.runsTable)
		}
	case focusWorkerDetail:
		if wl.workers != nil {
			wl.app.JigApp().SetFocus(wl.workers.detail)
		}
	case focusEvents:
		if p := wl.eventsPreviewPrimitive(); p != nil {
			wl.app.JigApp().SetFocus(p)
		} else {
			wl.app.JigApp().SetFocus(wl.eventTable)
		}
	case focusEventDetail:
		if wl.previewKind == previewDetails && wl.workflowIOView != nil {
			wl.app.JigApp().SetFocus(wl.workflowIOView)
		} else if wl.previewKind == previewHierarchy && wl.hierarchyView != nil && wl.hierarchyView.graph != nil {
			wl.app.JigApp().SetFocus(wl.hierarchyView.graph)
		} else if p := wl.activityDetailFocusPrimitive(); p != nil {
			wl.app.JigApp().SetFocus(p)
		} else {
			wl.app.JigApp().SetFocus(wl.eventDetail)
		}
	case focusTimeline:
		wl.app.JigApp().SetFocus(wl.timelineView)
	case focusFilters:
		if wl.filterBar != nil {
			wl.app.JigApp().SetFocus(wl.filterBar)
		} else {
			wl.app.JigApp().SetFocus(wl.table)
		}
	default:
		if wl.taskQueuesActive() && wl.taskQueues != nil {
			wl.app.JigApp().SetFocus(wl.taskQueues.queueTable)
		} else if wl.schedulesActive() && wl.schedules != nil {
			wl.app.JigApp().SetFocus(wl.schedules.table)
		} else if wl.workersActive() && wl.workers != nil {
			wl.app.JigApp().SetFocus(wl.workers.table)
		} else {
			wl.app.JigApp().SetFocus(wl.table)
		}
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) applyFocusStyles() {
	active := wl == nil || wl.app == nil || !wl.app.modalHasFocus()
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetFocused(active && (wl.focusPane == focusWorkflows || wl.focusPane == focusFilters))
	}
	if wl.previewPanel != nil {
		wl.previewPanel.SetFocused(active && wl.focusPane == focusEvents)
	}
	if wl.eventDetailPanel != nil {
		wl.eventDetailPanel.SetFocused(active && wl.previewShowsSidePane() && wl.focusPane == focusEventDetail)
	}
	if wl.hierarchyGraphPanel != nil {
		wl.hierarchyGraphPanel.SetFocused(active && wl.previewKind == previewHierarchy && wl.focusPane == focusEventDetail)
	}
	if wl.eventsPanel != nil {
		wl.eventsPanel.SetFocused(active && wl.focusPane == focusEvents)
	}
	if wl.timelinePanel != nil {
		wl.timelinePanel.SetFocused(active && wl.focusPane == focusTimeline)
	}
	if wl.taskQueues != nil && wl.taskQueues.pollerPanel != nil {
		wl.taskQueues.pollerPanel.SetFocused(active && wl.focusPane == focusPollers)
	}
	if wl.schedules != nil && wl.schedules.detailPanel != nil {
		wl.schedules.detailPanel.SetFocused(active && wl.focusPane == focusScheduleDetail)
	}
	if wl.schedules != nil && wl.schedules.runsPanel != nil {
		wl.schedules.runsPanel.SetFocused(active && wl.focusPane == focusScheduleRuns)
	}
	if wl.workers != nil && wl.workers.previewPanel != nil {
		wl.workers.previewPanel.SetFocused(active && wl.focusPane == focusWorkerDetail)
	}
	if wl.taskQueues != nil && wl.taskQueues.queueTable != nil {
		wl.taskQueues.queueTable.SetSelectable(active && wl.taskQueuesActive() && wl.focusPane == focusWorkflows, false)
	}
	if wl.taskQueues != nil && wl.taskQueues.pollerTable != nil {
		wl.taskQueues.pollerTable.SetSelectable(active && wl.taskQueuesActive() && wl.focusPane == focusPollers, false)
	}
	if wl.schedules != nil && wl.schedules.table != nil {
		wl.schedules.table.SetSelectable(active && wl.schedulesActive() && wl.focusPane == focusWorkflows, false)
	}
	if wl.schedules != nil && wl.schedules.detail != nil {
		wl.schedules.detail.SetSelectable(active && wl.schedulesActive() && wl.focusPane == focusScheduleDetail, false)
	}
	if wl.schedules != nil && wl.schedules.runsTable != nil {
		wl.schedules.runsTable.SetSelectable(active && wl.schedulesActive() && wl.focusPane == focusScheduleRuns, false)
	}
	if wl.workers != nil && wl.workers.table != nil {
		wl.workers.table.SetSelectable(active && wl.workersActive() && wl.focusPane == focusWorkflows, false)
	}
	if wl.table != nil {
		wl.table.SetSelectable(wl.workflowsActive(), false)
	}
	if wl.eventTable != nil {
		wl.eventTable.SetSelectable(wl.previewKind != previewHierarchy, false)
	}
	if wl.workflowDetail != nil {
		wl.workflowDetail.SetSelectable(active && wl.previewKind == previewDetails && wl.focusPane == focusEvents, false)
	}
	if wl.activityDetail != nil {
		wl.activityDetail.SetSelectable(active && wl.activityDetailTableFocused() && wl.focusPane == focusEventDetail, false)
	}
}

func (wl *WorkflowList) syncFocusFromPrimitives() {
	var pane workflowFocusPane
	switch {
	case wl.workflowDetail != nil && wl.workflowDetail.HasFocus():
		pane = focusEvents
	case wl.workflowIOView != nil && wl.workflowIOView.HasFocus():
		pane = focusEventDetail
	case wl.workflowIOTabs != nil && wl.workflowIOTabs.HasFocus():
		pane = focusEventDetail
	case wl.activityDetail != nil && wl.activityDetail.HasFocus():
		pane = focusEventDetail
	case wl.activityDetailTabs != nil && wl.activityDetailTabs.HasFocus():
		pane = focusEventDetail
	case wl.eventDetail != nil && wl.eventDetail.HasFocus():
		pane = focusEventDetail
	case wl.hierarchyView != nil && wl.hierarchyView.graph != nil && wl.hierarchyView.graph.HasFocus():
		pane = focusEventDetail
	case wl.hierarchyView != nil && wl.hierarchyView.tree != nil && wl.hierarchyView.tree.HasFocus():
		pane = focusEvents
	case wl.eventTreeView != nil && wl.eventTreeView.HasFocus():
		pane = focusEvents
	case wl.eventTable != nil && wl.eventTable.HasFocus():
		pane = focusEvents
	case wl.timelineView != nil && wl.timelineView.HasFocus():
		pane = focusTimeline
	case wl.taskQueues != nil && wl.taskQueues.pollerTable != nil && wl.taskQueues.pollerTable.HasFocus():
		pane = focusPollers
	case wl.schedules != nil && wl.schedules.detail != nil && wl.schedules.detail.HasFocus():
		pane = focusScheduleDetail
	case wl.schedules != nil && wl.schedules.runsTable != nil && wl.schedules.runsTable.HasFocus():
		pane = focusScheduleRuns
	case wl.workers != nil && wl.workers.detail != nil && wl.workers.detail.HasFocus():
		pane = focusWorkerDetail
	case wl.taskQueues != nil && wl.taskQueues.queueTable != nil && wl.taskQueues.queueTable.HasFocus():
		pane = focusWorkflows
	case wl.taskQueues != nil && wl.taskQueues.HasFocus():
		pane = focusWorkflows
	case wl.schedules != nil && wl.schedules.table != nil && wl.schedules.table.HasFocus():
		pane = focusWorkflows
	case wl.workers != nil && wl.workers.table != nil && wl.workers.table.HasFocus():
		pane = focusWorkflows
	case wl.filterBar != nil && wl.filterBar.HasFocus():
		pane = focusFilters
	case wl.table != nil && wl.table.HasFocus():
		pane = focusWorkflows
	default:
		wl.applyFocusStyles()
		return
	}
	if pane != wl.focusPane {
		wl.focusPane = pane
	}
	wl.applyFocusStyles()
}
