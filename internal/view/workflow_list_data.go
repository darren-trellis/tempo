package view

import (
	"context"
	"fmt"
	"sort"
	"time"

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

	wl.table.ClearRows()
	wl.table.SetHeaders(workflowTableHeaders...)

	if len(wl.workflows) == 0 {
		if len(wl.allWorkflows) == 0 {
			wl.SetMasterContent(wl.emptyState)
		} else {
			wl.SetMasterContent(wl.noResultsState)
		}
		return
	}

	wl.SetMasterContent(wl.table)

	widths := wl.calculateColumnWidths()

	now := time.Now()
	for _, w := range wl.workflows {
		statusHandle := temporal.GetWorkflowStatus(w.Status)
		wl.table.AddRowWithStatus(statusHandle, 1,
			truncateIfNeeded(w.ID, widths.id),
			w.Status,
			truncateIfNeeded(w.Type, widths.typ),
			formatRelativeTime(now, w.StartTime),
			workflowEndTime(now, w),
			workflowDuration(now, w),
			truncateIfNeeded(w.TaskQueue, widths.queue),
			truncateIfNeeded(w.RunID, widths.runID),
		)
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
	wl.table.ClearRows()
	wl.table.SetHeaders(workflowTableHeaders...)
	wl.table.AddRowWithColor(theme.Error(),
		theme.IconError+" Error loading workflows",
		err.Error(),
		"", "", "", "", "", "",
	)
}

type workflowColWidths struct {
	id, typ, queue, runID int
}

func (wl *WorkflowList) calculateColumnWidths() workflowColWidths {
	_, _, totalWidth, _ := wl.MasterDetailView.GetInnerRect()
	width := totalWidth - 4
	if width <= 0 {
		return workflowColWidths{id: 25, typ: 15, queue: 14, runID: 12}
	}

	const (
		statusWidth   = 12
		startedWidth  = 11
		endedWidth    = 11
		durationWidth = 12
		separators    = 16
		minID         = 15
		minType       = 10
		minQueue      = 10
		minRunID      = 10
		maxRunID      = 36
	)

	fixed := statusWidth + startedWidth + endedWidth + durationWidth + separators
	available := width - fixed
	if available <= minID+minType+minQueue+minRunID {
		return workflowColWidths{id: minID, typ: minType, queue: minQueue, runID: minRunID}
	}

	runID := maxRunID
	if available < minID+minType+minQueue+maxRunID {
		runID = minRunID
	}

	variable := available - runID
	id := (variable * 50) / 100
	rest := variable - id
	typ := rest / 2
	queue := rest - typ

	if id >= 50 {
		id = 0
	} else if id < minID {
		id = minID
	}
	if typ >= 40 {
		typ = 0
	} else if typ < minType {
		typ = minType
	}
	if queue >= 40 {
		queue = 0
	} else if queue < minQueue {
		queue = minQueue
	}
	if runID >= maxRunID {
		runID = 0
	}

	return workflowColWidths{id: id, typ: typ, queue: queue, runID: runID}
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
