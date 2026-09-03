package view

import (
	"context"
	"fmt"
	"sync"
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

	if live {
		if wl.filterText != "" {
			wl.refreshCounts()
			return
		}
		if wl.pageBusy {
			wl.liveBusy = false
			return
		}
		if len(wl.pager.pages) == 0 {
			wl.startWindow(true)
			return
		}
		wl.refreshLoadedPages()
		return
	}
	wl.startWindow(false)
}

func (wl *WorkflowList) startWindow(live bool) {
	resolvedQuery, err := resolveTimePlaceholders(wl.visibilityQuery)
	if err != nil {
		wl.app.ShowToastError(fmt.Sprintf("Invalid query: %v", err))
		wl.liveBusy = false
		return
	}

	provider := wl.app.Provider()
	if provider == nil {
		wl.liveBusy = false
		return
	}

	wl.pageGen++
	gen := wl.pageGen
	wl.pager.reset(resolvedQuery)
	wl.pageBusy = true
	if !live {
		wl.setLoading(true)
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		var workflows []temporal.Workflow
		var next string
		var listErr error
		var counts temporal.WorkflowCounts
		var countErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			workflows, next, listErr = provider.ListWorkflows(ctx, wl.namespace, temporal.ListOptions{
				PageSize: workflowPageSize,
				Query:    resolvedQuery,
			})
		}()
		go func() {
			defer wg.Done()
			counts, countErr = provider.CountWorkflows(ctx, wl.namespace, resolvedQuery)
		}()
		wg.Wait()

		wl.app.JigApp().QueueUpdateDraw(func() {
			if gen != wl.pageGen {
				return
			}
			wl.liveBusy = false
			wl.pageBusy = false
			wl.setLoading(false)
			if listErr != nil {
				if !live {
					wl.showError(listErr)
				}
				return
			}
			if !live {
				wl.previewWorkflowID = ""
				wl.previewRunID = ""
			}
			wl.pager.accept(0, "", next, workflows)
			wl.applyLoadedWindow()
			if countErr == nil {
				wl.applyServerCounts(counts)
			}
			if !live && wl.shouldFocusWorkflowTable() {
				wl.app.JigApp().SetFocus(wl.table)
			}
			if live {
				wl.refreshLivePreview()
			}
			wl.maybeFetchPages()
		})
	}()
}

func (wl *WorkflowList) refreshLoadedPages() {
	provider := wl.app.Provider()
	if provider == nil {
		wl.liveBusy = false
		return
	}
	pages := append([]workflowPage(nil), wl.pager.pages...)
	first := wl.pager.firstPage
	query := wl.pager.query
	gen := wl.pageGen
	wl.pageBusy = true

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type pageResult struct {
			index     int
			token     string
			next      string
			workflows []temporal.Workflow
			err       error
		}
		results := make([]pageResult, len(pages))
		var wg sync.WaitGroup
		var counts temporal.WorkflowCounts
		var countErr error
		wg.Add(len(pages) + 1)
		for i, page := range pages {
			go func(i int, page workflowPage) {
				defer wg.Done()
				workflows, next, err := provider.ListWorkflows(ctx, wl.namespace, temporal.ListOptions{
					PageSize:  workflowPageSize,
					PageToken: page.token,
					Query:     query,
				})
				results[i] = pageResult{index: first + i, token: page.token, next: next, workflows: workflows, err: err}
			}(i, page)
		}
		go func() {
			defer wg.Done()
			counts, countErr = provider.CountWorkflows(ctx, wl.namespace, query)
		}()
		wg.Wait()

		wl.app.JigApp().QueueUpdateDraw(func() {
			if gen != wl.pageGen {
				return
			}
			wl.liveBusy = false
			wl.pageBusy = false
			for _, result := range results {
				if result.err != nil {
					continue
				}
				wl.pager.accept(result.index, result.token, result.next, result.workflows)
			}
			wl.applyLoadedWindow()
			if countErr == nil {
				wl.applyServerCounts(counts)
			}
			wl.refreshLivePreview()
			wl.maybeFetchPages()
		})
	}()
}

func (wl *WorkflowList) refreshCounts() {
	provider := wl.app.Provider()
	if provider == nil {
		wl.liveBusy = false
		return
	}
	query := wl.pager.query
	if query == "" {
		resolved, err := resolveTimePlaceholders(wl.visibilityQuery)
		if err != nil {
			wl.liveBusy = false
			return
		}
		query = resolved
	}
	gen := wl.pageGen
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		counts, err := provider.CountWorkflows(ctx, wl.namespace, query)
		wl.app.JigApp().QueueUpdateDraw(func() {
			if gen != wl.pageGen {
				return
			}
			wl.liveBusy = false
			if err == nil {
				wl.applyServerCounts(counts)
			}
		})
	}()
}

func (wl *WorkflowList) maybeFetchPages() {
	if wl == nil || wl.preloaded || wl.pageBusy || wl.liveBusy {
		return
	}
	if wl.filterText != "" {
		return
	}
	if !wl.workflowsActive() || wl.app == nil || wl.app.Provider() == nil {
		return
	}
	idx := workflowIndexByIdentity(wl.allWorkflows, wl.highlightedWorkflowID, wl.highlightedRunID)
	if idx < 0 {
		idx = 0
	}
	budget := wl.visibleWorkflowBudget()
	n := len(wl.allWorkflows)
	if wl.pager.hasNext() && (n == 0 || idx+budget >= n-1) {
		wl.fetchAdjacentPage(false)
		return
	}
	if wl.pager.hasPrev() && idx < budget {
		wl.fetchAdjacentPage(true)
	}
}

func (wl *WorkflowList) fetchAdjacentPage(prev bool) {
	if wl.pageBusy {
		return
	}
	var pageIndex int
	var token string
	if prev {
		t, ok := wl.pager.prevToken()
		if !ok {
			return
		}
		token = t
		pageIndex = wl.pager.firstPage - 1
	} else {
		if !wl.pager.hasNext() {
			return
		}
		token = wl.pager.nextToken()
		pageIndex = wl.pager.nextPageIndex()
	}
	provider := wl.app.Provider()
	if provider == nil {
		return
	}
	query := wl.pager.query
	gen := wl.pageGen
	wl.pageBusy = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		workflows, next, err := provider.ListWorkflows(ctx, wl.namespace, temporal.ListOptions{
			PageSize:  workflowPageSize,
			PageToken: token,
			Query:     query,
		})
		wl.app.JigApp().QueueUpdateDraw(func() {
			if gen != wl.pageGen {
				return
			}
			wl.pageBusy = false
			if err != nil {
				return
			}
			wl.pager.accept(pageIndex, token, next, workflows)
			wl.applyLoadedWindow()
			wl.maybeFetchPages()
		})
	}()
}

func (wl *WorkflowList) applyLoadedWindow() {
	wl.allWorkflows = wl.pager.items()
	wl.applyFilter()
}

func (wl *WorkflowList) applyServerCounts(counts temporal.WorkflowCounts) {
	wl.serverStats = WorkflowStats{
		Running:    counts.Running,
		Completed:  counts.Completed,
		Failed:     counts.Failed,
		Canceled:   counts.Canceled,
		Terminated: counts.Terminated,
	}
	wl.serverStatsOK = true
	wl.updateStats()
}

func (wl *WorkflowList) visibleWorkflowBudget() int {
	if wl.table == nil {
		return 8
	}
	_, _, _, h := wl.table.GetInnerRect()
	if h < 8 {
		return 8
	}
	return h
}

func (wl *WorkflowList) loadMockData() {
	wl.serverStatsOK = false
	wl.pager = workflowPager{}
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

func (wl *WorkflowList) displayedStats() WorkflowStats {
	if wl.serverStatsOK {
		return wl.serverStats
	}
	var stats WorkflowStats
	for _, w := range wl.workflows {
		switch w.Status {
		case "Running":
			stats.Running++
		case "Completed":
			stats.Completed++
		case "Failed":
			stats.Failed++
		case "Canceled":
			stats.Canceled++
		case "Terminated":
			stats.Terminated++
		}
	}
	return stats
}

func (wl *WorkflowList) updateStats() {
	if wl.app == nil || wl.app.statusBar == nil {
		return
	}
	wl.app.SetWorkflowStats(wl.displayedStats())
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
