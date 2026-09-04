package view

import (
	"github.com/galaxy-io/tempo/internal/temporal"
)

func (wl *WorkflowList) actionTarget() (workflowActionTarget, bool) {
	w, ok := wl.selectedWorkflow()
	if !ok || w.ID == "" {
		return workflowActionTarget{}, false
	}
	ns := wl.namespace
	if ns == "" && wl.app != nil {
		ns = wl.app.CurrentNamespace()
	}
	return workflowActionTarget{
		Namespace: ns,
		ID:        w.ID,
		RunID:     w.RunID,
		Status:    w.Status,
	}, true
}

func (wl *WorkflowList) afterWorkflowAction() {
	if w, ok := wl.selectedWorkflow(); ok && wl.previewCache != nil {
		wl.previewCache.delete(w.ID, w.RunID)
	}
	wl.loadData()
}

func (wl *WorkflowList) showCancelSelected() {
	t, ok := wl.actionTarget()
	if !ok || !workflowIsRunning(t.Status) {
		return
	}
	showCancelWorkflowModal(wl.app, t, func() {
		wl.app.ToastSuccess("Cancelled " + t.ID)
		wl.afterWorkflowAction()
	})
}

func (wl *WorkflowList) showTerminateSelected() {
	t, ok := wl.actionTarget()
	if !ok || !workflowIsRunning(t.Status) {
		return
	}
	showTerminateWorkflowModal(wl.app, t, func() {
		wl.app.ToastSuccess("Terminated " + t.ID)
		wl.afterWorkflowAction()
	})
}

func (wl *WorkflowList) showSignalSelected() {
	t, ok := wl.actionTarget()
	if !ok || !workflowIsRunning(t.Status) {
		return
	}
	showSignalWorkflowModal(wl.app, t, func() {
		wl.app.ToastSuccess("Signaled " + t.ID)
		wl.afterWorkflowAction()
	})
}

func (wl *WorkflowList) showQuerySelected() {
	t, ok := wl.actionTarget()
	if !ok || !workflowIsRunning(t.Status) {
		return
	}
	showQueryWorkflowModal(wl.app, t)
}

func (wl *WorkflowList) showResetSelected() {
	t, ok := wl.actionTarget()
	if !ok || !workflowCanReset(t.Status) {
		return
	}
	showResetWorkflowModal(wl.app, t, func(newRunID string) {
		if newRunID != "" {
			wl.updateWorkflowRunID(t.ID, t.RunID, newRunID)
		}
		wl.app.ToastSuccess("Reset " + t.ID)
		wl.afterWorkflowAction()
	})
}

func (wl *WorkflowList) showDeleteSelected() {
	t, ok := wl.actionTarget()
	if !ok {
		return
	}
	showDeleteWorkflowModal(wl.app, t, func() {
		wl.app.ToastSuccess("Deleted " + t.ID)
		if wl.previewCache != nil {
			wl.previewCache.delete(t.ID, t.RunID)
		}
		wl.clearPreview()
		wl.loadData()
	})
}

func (wl *WorkflowList) refreshSelectedPreview() {
	w, ok := wl.selectedWorkflow()
	if !ok {
		return
	}
	if wl.previewCache != nil {
		wl.previewCache.delete(w.ID, w.RunID)
	}
	if wl.historyNeeded() {
		wl.schedulePreview(w, true)
	}
}

func (wl *WorkflowList) showPreviewEventSearch() {
	if !wl.previewModeEnabled() || wl.previewKind != previewEvents {
		return
	}
	wl.app.ShowFilterMode(wl.previewEventSearch, FilterModeCallbacks{
		OnChange: func(text string) {
			wl.applyPreviewEventSearch(text)
		},
		OnSubmit: func(text string) {
			wl.applyPreviewEventSearch(text)
		},
		OnCancel: func() {},
	})
}

func (wl *WorkflowList) applyPreviewEventSearch(query string) {
	wl.previewEventSearch = query
	if w, ok := wl.currentPreviewWorkflow(); ok {
		wl.renderPreviewEvents(w)
		return
	}
	wl.renderPreviewEvents(temporal.Workflow{})
}

func (wl *WorkflowList) visiblePreviewEvents() []temporal.EnhancedHistoryEvent {
	return filterHistoryEvents(wl.previewEvents, wl.previewEventSearch)
}

func (wl *WorkflowList) selectedPreviewEvent() (temporal.EnhancedHistoryEvent, bool) {
	events := wl.visiblePreviewEvents()
	if wl.eventTable == nil || len(events) == 0 {
		return temporal.EnhancedHistoryEvent{}, false
	}
	row := wl.eventTable.SelectedRow()
	if row < 0 && len(events) > 0 {
		row = 0
	}
	if row < 0 || row >= len(events) {
		return temporal.EnhancedHistoryEvent{}, false
	}
	return events[row], true
}

func (wl *WorkflowList) jumpToPreviewChild() {
	ev, ok := wl.selectedPreviewEvent()
	if !ok || ev.ChildWorkflowID == "" || ev.ChildRunID == "" {
		return
	}
	if wl.selectWorkflowByID(ev.ChildWorkflowID) {
		if w, ok := wl.selectedWorkflow(); ok && wl.historyNeeded() {
			wl.schedulePreview(w, false)
		}
		return
	}
	if wl.app != nil {
		wl.app.NavigateToWorkflowDetail(ev.ChildWorkflowID, ev.ChildRunID)
	}
}

func (wl *WorkflowList) updateWorkflowRunID(id, oldRunID, newRunID string) {
	update := func(list []temporal.Workflow) {
		for i := range list {
			if list[i].ID == id && list[i].RunID == oldRunID {
				list[i].RunID = newRunID
			}
		}
	}
	update(wl.workflows)
	update(wl.allWorkflows)
}

func (wl *WorkflowList) mergeWorkflow(fresh temporal.Workflow) {
	merge := func(list []temporal.Workflow) {
		for i := range list {
			if list[i].ID == fresh.ID && list[i].RunID == fresh.RunID {
				list[i] = fresh
				return
			}
		}
	}
	merge(wl.workflows)
	merge(wl.allWorkflows)
}

func (wl *WorkflowList) actionHints(w temporal.Workflow) []KeyHint {
	var hints []KeyHint
	if workflowIsRunning(w.Status) {
		hints = append(hints,
			KeyHint{Key: "c", Description: "Cancel"},
			KeyHint{Key: "X", Description: "Terminate"},
			KeyHint{Key: "Q", Description: "Query"},
		)
	}
	if workflowCanReset(w.Status) {
		hints = append(hints, KeyHint{Key: "R", Description: "Reset"})
	}
	return hints
}
