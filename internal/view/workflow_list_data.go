package view

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func (wl *WorkflowList) setLoading(loading bool) {
	wl.loading = loading
}

func (wl *WorkflowList) loadData() {
	if wl.preloaded {
		go func() {
			wl.app.JigApp().QueueUpdateDraw(func() {
				wl.populateTable()
				wl.updateStats()
			})
		}()
		return
	}

	provider := wl.app.Provider()
	if provider == nil {
		wl.loadMockData()
		return
	}

	wl.setLoading(true)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Resolve time placeholders in the query
		resolvedQuery, err := resolveTimePlaceholders(wl.visibilityQuery)
		if err != nil {
			wl.app.ShowToastError(fmt.Sprintf("Invalid query: %v", err))
			wl.app.JigApp().QueueUpdateDraw(func() {
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
			wl.setLoading(false)
			if err != nil {
				wl.showError(err)
				return
			}
			// Sort by most recent first
			sort.Slice(workflows, func(i, j int) bool {
				return workflows[i].StartTime.After(workflows[j].StartTime)
			})
			wl.allWorkflows = workflows
			wl.applyFilter()
			// Set focus to table after data loads
			if len(wl.workflows) > 0 {
				wl.app.JigApp().SetFocus(wl.table)
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

func (wl *WorkflowList) populateTable() {
	currentRow := wl.table.SelectedRow()

	cols := wl.columnLayout()
	wl.table.ClearRows()
	applyWorkflowColumnHeaders(wl.table, cols)

	if len(wl.workflows) == 0 {
		if len(wl.allWorkflows) == 0 {
			wl.SetMasterContent(wl.emptyState)
		} else {
			wl.SetMasterContent(wl.noResultsState)
		}
		return
	}

	wl.SetMasterContent(wl.table)

	now := time.Now()
	for _, w := range wl.workflows {
		cells := make([]components.TableCell, len(cols))
		for i, col := range cols {
			cells[i] = col.cell(now, w)
		}
		wl.table.AddStyledRow(cells)
	}

	if wl.table.RowCount() > 0 {
		if currentRow >= 0 && currentRow < len(wl.workflows) {
			wl.table.SelectRow(currentRow)
		} else {
			wl.table.SelectRow(0)
		}
	}
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

func (wl *WorkflowList) startAutoRefresh() {
	// Drain any stale stop signal from previous stop
	select {
	case <-wl.stopRefresh:
	default:
	}

	wl.refreshTicker = time.NewTicker(5 * time.Second)
	ticker := wl.refreshTicker // Capture locally to avoid nil access after stop
	go func() {
		for {
			select {
			case <-ticker.C:
				wl.app.JigApp().QueueUpdateDraw(func() {
					wl.loadData()
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
