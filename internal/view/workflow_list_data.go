package view

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func (wl *WorkflowList) setLoading(loading bool) {
	wl.loading = loading
	wl.app.SetViewLoading("workflows", loading)
}

// refresh reloads the list from the server. Everything cached for the current
// list is dropped first: an explicit refresh must never be answered from a
// snapshot taken before it.
func (wl *WorkflowList) refresh() {
	wl.invalidateCaches()
	wl.loadData()
}

// invalidateCaches drops the preview history and hierarchy held for this list.
func (wl *WorkflowList) invalidateCaches() {
	wl.previewCache.clear()
	wl.previewEvents = nil
	wl.previewActivities = nil
	wl.previewWorkflowID = ""
	wl.previewRunID = ""
	if wl.hierarchyView != nil {
		wl.hierarchyView.Invalidate()
	}
}

func (wl *WorkflowList) loadData() {
	wl.fetchWorkflows(false)
}

func (wl *WorkflowList) liveRefresh() {
	if wl.liveBusy {
		return
	}
	wl.liveBusy = true
	wl.fetchWorkflows(true)
}

func (wl *WorkflowList) fetchWorkflows(live bool) {
	if wl.preloaded {
		go func() {
			wl.app.JigApp().QueueUpdateDraw(func() {
				wl.liveBusy = false
				wl.populateTable()
				wl.updateStats()
			})
		}()
		return
	}

	provider := wl.app.Provider()
	if provider == nil {
		wl.liveBusy = false
		wl.loadMockData()
		return
	}

	wl.setLoading(true)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resolvedQuery, err := resolveTimePlaceholders(wl.visibilityQuery)
		if err != nil {
			wl.app.ShowToastError(fmt.Sprintf("Invalid query: %v", err))
			wl.app.JigApp().QueueUpdateDraw(func() {
				wl.liveBusy = false
				wl.setLoading(false)
			})
			return
		}
		opts := temporal.ListOptions{
			PageSize: 100,
			Query:    resolvedQuery,
		}
		workflows, _, err := provider.ListWorkflows(ctx, wl.namespace, opts)

		wl.app.JigApp().QueueUpdateDraw(func() {
			wl.liveBusy = false
			wl.setLoading(false)
			if err != nil {
				if !live {
					wl.showError(err)
				}
				return
			}
			sort.Slice(workflows, func(i, j int) bool {
				return workflows[i].StartTime.After(workflows[j].StartTime)
			})
			if !live {
				wl.previewWorkflowID = ""
				wl.previewRunID = ""
			}
			wl.allWorkflows = workflows
			wl.applyFilter()
			if !live && wl.shouldFocusWorkflowTable() {
				wl.app.JigApp().SetFocus(wl.table)
			}
			if live {
				wl.refreshLivePreview()
			}
		})
	}()
}

func (wl *WorkflowList) loadMockData() {
	now := time.Now()
	wl.allWorkflows = []temporal.Workflow{
		{
			ID: "order-processing-abc123", RunID: "run-001-xyz", Type: "OrderWorkflow",
			Status: "Running", Namespace: wl.namespace, TaskQueue: "order-tasks",
			StartTime: now.Add(-5 * time.Minute),
		},
		{
			ID: "payment-xyz789", RunID: "run-002-abc", Type: "PaymentWorkflow",
			Status: "Completed", Namespace: wl.namespace, TaskQueue: "payment-tasks",
			StartTime: now.Add(-1 * time.Hour), EndTime: ptr(now.Add(-55 * time.Minute)),
			ParentID: ptr("order-processing-abc123"),
		},
		{
			ID: "fulfillment-ghi000", RunID: "run-006-mno", Type: "FulfillmentWorkflow",
			Status: "Running", Namespace: wl.namespace, TaskQueue: "fulfill-tasks",
			StartTime: now.Add(-50 * time.Minute),
			ParentID:  ptr("payment-xyz789"),
		},
		{
			ID: "shipment-def456", RunID: "run-003-def", Type: "ShipmentWorkflow",
			Status: "Failed", Namespace: wl.namespace, TaskQueue: "shipment-tasks",
			StartTime: now.Add(-30 * time.Minute), EndTime: ptr(now.Add(-25 * time.Minute)),
		},
		{
			ID: "inventory-check-111", RunID: "run-004-ghi", Type: "InventoryWorkflow",
			Status: "Running", Namespace: wl.namespace, TaskQueue: "inventory-tasks",
			StartTime: now.Add(-10 * time.Minute),
		},
		{
			ID: "user-signup-222", RunID: "run-005-jkl", Type: "UserOnboardingWorkflow",
			Status: "Completed", Namespace: wl.namespace, TaskQueue: "user-tasks",
			StartTime: now.Add(-2 * time.Hour), EndTime: ptr(now.Add(-1*time.Hour - 45*time.Minute)),
		},
	}
	wl.applyFilter()
}

// renderColumns redraws the headers and row cells for the current column layout,
// leaving the selection and preview alone. Used for live column edits.
func (wl *WorkflowList) renderColumns() {
	cols := wl.columnLayout()
	row := wl.table.SelectedRow()
	wl.table.ClearRows()
	applyWorkflowColumnHeaders(wl.table, cols)
	now := time.Now()
	for i, w := range wl.workflows {
		cells := make([]components.TableCell, len(cols))
		for j, col := range cols {
			cells[j] = col.cell(now, w, wl.workflowDepth(i))
		}
		wl.table.AddStyledRow(cells)
	}
	if row >= 0 && row < wl.table.RowCount() {
		wl.table.SelectRow(row)
	}
	if wl.tableScroll != nil {
		wl.tableScroll.clamp()
	}
}

func (wl *WorkflowList) rememberHighlightedWorkflow() {
	if wl == nil || wl.table == nil {
		return
	}
	row := wl.table.SelectedRow()
	if row < 0 || row >= len(wl.workflows) {
		return
	}
	wl.highlightedWorkflowID = wl.workflows[row].ID
	wl.highlightedRunID = wl.workflows[row].RunID
}

func workflowIndexByIdentity(workflows []temporal.Workflow, id, runID string) int {
	if id == "" {
		return -1
	}
	fallback := -1
	for i, w := range workflows {
		if w.ID != id {
			continue
		}
		if runID == "" || w.RunID == runID {
			return i
		}
		if fallback < 0 {
			fallback = i
		}
	}
	return fallback
}

func (wl *WorkflowList) populateTable() {
	id, runID := wl.highlightedWorkflowID, wl.highlightedRunID
	if id == "" && wl.table != nil {
		row := wl.table.SelectedRow()
		if row >= 0 && row < len(wl.workflows) {
			id, runID = wl.workflows[row].ID, wl.workflows[row].RunID
		}
	}

	cols := wl.columnLayout()
	wl.table.ClearRows()
	applyWorkflowColumnHeaders(wl.table, cols)

	if len(wl.workflows) == 0 {
		if len(wl.allWorkflows) == 0 {
			wl.table.ConfigureEmpty(theme.IconInfo, "No Workflows", "No workflows found in this namespace")
		} else {
			wl.table.ConfigureEmpty(theme.IconSearch, "No Results", "No workflows match the current filter")
		}
		wl.SetMasterContent(wl.table)
		wl.clearPreview()
		return
	}

	wl.SetMasterContent(wl.table)

	now := time.Now()
	for i, w := range wl.workflows {
		cells := make([]components.TableCell, len(cols))
		for j, col := range cols {
			cells[j] = col.cell(now, w, wl.workflowDepth(i))
		}
		wl.table.AddStyledRow(cells)
	}

	idx := workflowIndexByIdentity(wl.workflows, id, runID)
	if idx < 0 {
		idx = 0
	}
	wl.table.SelectRow(idx)
	wl.rememberHighlightedWorkflow()
	wl.schedulePreview(wl.workflows[idx], false)
}

func (wl *WorkflowList) updateStats() {
	var running, completed, failed int
	for _, w := range wl.workflows {
		switch w.Status {
		case "Running":
			running++
		case "Completed":
			completed++
		case "Failed":
			failed++
		}
	}
	if wl.app == nil || wl.app.statusBar == nil {
		return
	}
	wl.app.SetWorkflowStats(WorkflowStats{
		Running:   running,
		Completed: completed,
		Failed:    failed,
	})
}

func (wl *WorkflowList) showError(err error) {
	cols := wl.columnLayout()
	wl.table.ClearRows()
	applyWorkflowColumnHeaders(wl.table, cols)
	cells := make([]string, len(cols))
	if len(cells) > 0 {
		cells[0] = theme.IconError + " Error loading workflows"
	}
	if len(cells) > 1 {
		cells[1] = err.Error()
	}
	wl.table.AddRowWithColor(theme.Error(), cells...)
}

// Auto-refresh methods

func (wl *WorkflowList) toggleAutoRefresh() {
	wl.autoRefresh = !wl.autoRefresh
	if wl.autoRefresh {
		wl.startAutoRefresh()
	} else {
		wl.stopAutoRefresh()
	}
}

func refreshRate(app *App) time.Duration {
	if app == nil {
		return config.DefaultRefreshRate
	}
	return app.Config().RefreshRate()
}

func (wl *WorkflowList) refreshInterval() time.Duration {
	return refreshRate(wl.app)
}

func (wl *WorkflowList) syncAutoRefresh() {
	if !wl.autoRefresh {
		return
	}
	wl.stopAutoRefresh()
	wl.startAutoRefresh()
}

func (wl *WorkflowList) refreshLivePreview() {
	if !wl.previewModeEnabled() && !wl.timelineVisible {
		return
	}
	w, ok := wl.selectedWorkflow()
	if !ok {
		return
	}
	wl.schedulePreview(w, true)
}

func (wl *WorkflowList) startAutoRefresh() {
	select {
	case <-wl.stopRefresh:
	default:
	}

	wl.refreshTicker = time.NewTicker(wl.refreshInterval())
	ticker := wl.refreshTicker
	go func() {
		for {
			select {
			case <-ticker.C:
				wl.app.JigApp().QueueUpdateDraw(func() {
					wl.liveRefresh()
				})
			case <-wl.stopRefresh:
				return
			}
		}
	}()
}

func (wl *WorkflowList) stopAutoRefresh() {
	if wl.refreshTicker != nil {
		wl.refreshTicker.Stop()
		wl.refreshTicker = nil
	}
	select {
	case wl.stopRefresh <- struct{}{}:
	default:
	}
}
