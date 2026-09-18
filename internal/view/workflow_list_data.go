package view

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func (wl *WorkflowList) pageSize() int {
	if wl != nil && wl.app != nil {
		return wl.app.Config().WorkflowPageLimit()
	}
	return config.DefaultWorkflowPageSize
}

func (wl *WorkflowList) previewLoadDelay() time.Duration {
	if wl != nil && wl.app != nil {
		return wl.app.Config().PreviewLoadDelay()
	}
	return config.DefaultPreviewLoadDelay
}

func (wl *WorkflowList) setLoading(loading bool) {
	wl.setLoadIndicator(loading, false)
}

func (wl *WorkflowList) setRefreshing(loading bool) {
	wl.setLoadIndicator(loading, true)
}

func (wl *WorkflowList) setLoadIndicator(loading, quiet bool) {
	wl.loading = loading
	if wl.app == nil {
		return
	}
	if quiet {
		wl.app.SetViewRefreshing("workflows", loading)
		return
	}
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
	wl.previewPending = false
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
		if wl.filterText != "" && wl.visibilityQuery == "" {
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
	wl.setLoadIndicator(true, live)

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
				PageSize: wl.pageSize(),
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
	wl.setRefreshing(true)

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
					PageSize:  wl.pageSize(),
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
			wl.setLoading(false)
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
	wl.setRefreshing(true)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		counts, err := provider.CountWorkflows(ctx, wl.namespace, query)
		wl.app.JigApp().QueueUpdateDraw(func() {
			if gen != wl.pageGen {
				return
			}
			wl.liveBusy = false
			wl.setLoading(false)
			if err == nil {
				wl.applyServerCounts(counts)
			}
		})
	}()
}

type listEdgePin int

const (
	listEdgeNone listEdgePin = iota
	listEdgeStart
	listEdgeEnd
)

func (wl *WorkflowList) jumpWorkflowListEdge(end bool) {
	if wl == nil || len(wl.workflows) == 0 {
		return
	}
	col := 0
	if wl.table != nil {
		_, col = wl.table.GetOffset()
	}
	if end {
		wl.listEdgePin = listEdgeEnd
		wl.selectWorkflowRow(len(wl.workflows) - 1)
		if wl.table != nil {
			wl.table.SetOffset(len(wl.workflows)-1, col)
		}
	} else {
		wl.listEdgePin = listEdgeStart
		wl.selectWorkflowRow(0)
		if wl.table != nil {
			wl.table.SetOffset(0, col)
		}
	}
	wl.rememberHighlightedWorkflow()
}

func (wl *WorkflowList) clearListEdgePinIfMoved() {
	if wl == nil || wl.table == nil || wl.listEdgePin == listEdgeNone {
		return
	}
	row := wl.table.SelectedRow()
	switch wl.listEdgePin {
	case listEdgeStart:
		if row != 0 {
			wl.listEdgePin = listEdgeNone
		}
	case listEdgeEnd:
		if row != len(wl.workflows)-1 {
			wl.listEdgePin = listEdgeNone
		}
	}
}

func (wl *WorkflowList) maybeFetchPages() {
	if wl == nil || wl.preloaded || wl.pageBusy || wl.liveBusy || wl.listEdgePin != listEdgeNone {
		return
	}
	if wl.filterText != "" && wl.visibilityQuery == "" {
		return
	}
	if !wl.workflowsActive() || wl.app == nil || wl.app.Provider() == nil {
		return
	}
	switch wl.adjacentPageNeed() {
	case 1:
		wl.fetchAdjacentPage(false)
	case -1:
		wl.fetchAdjacentPage(true)
	}
}

// adjacentPageNeed reports whether the visible list should load the next page
// (1), the previous page (-1), or neither (0). g/G land on the exact first or
// last row; fetching there while the 4-page window is full trims the other end
// and yanks the highlight back into the remaining rows.
func (wl *WorkflowList) adjacentPageNeed() int {
	if wl == nil {
		return 0
	}
	n := len(wl.workflows)
	if n == 0 {
		n = len(wl.allWorkflows)
	}
	if n == 0 {
		return 0
	}
	idx := -1
	if wl.table != nil {
		idx = wl.table.SelectedRow()
	}
	if idx < 0 {
		idx = workflowIndexByIdentity(wl.workflows, wl.highlightedWorkflowID, wl.highlightedRunID)
	}
	if idx < 0 {
		idx = 0
	}
	if idx == 0 || idx == n-1 {
		return 0
	}
	budget := wl.visibleWorkflowBudget()
	if wl.pager.hasNext() && idx+budget >= n-1 {
		return 1
	}
	if wl.pager.hasPrev() && idx < budget {
		return -1
	}
	return 0
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
	wl.setLoading(true)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		workflows, next, err := provider.ListWorkflows(ctx, wl.namespace, temporal.ListOptions{
			PageSize:  wl.pageSize(),
			PageToken: token,
			Query:     query,
		})
		finish := func() {
			if gen != wl.pageGen {
				return
			}
			wl.pageBusy = false
			wl.setLoading(false)
			if err != nil {
				return
			}
			wl.pager.accept(pageIndex, token, next, workflows)
			wl.applyLoadedWindow()
			wl.maybeFetchPages()
		}
		if jig := wl.app.JigApp(); jig != nil {
			jig.QueueUpdateDraw(finish)
			return
		}
		finish()
	}()
}

func (wl *WorkflowList) applyLoadedWindow() {
	if wl.listEdgePin == listEdgeNone {
		wl.rememberListAnchor()
	} else {
		wl.hasListAnchor = false
	}
	wl.allWorkflows = wl.pager.items()
	wl.applyFilter()
}

func (wl *WorkflowList) rememberListAnchor() {
	if wl == nil {
		return
	}
	id, runID := wl.highlightedWorkflowID, wl.highlightedRunID
	idx := workflowIndexByIdentity(wl.workflows, id, runID)
	if idx < 0 && wl.table != nil {
		row := wl.table.SelectedRow()
		if row >= 0 && row < len(wl.workflows) {
			idx = row
			wl.highlightedWorkflowID = wl.workflows[row].ID
			wl.highlightedRunID = wl.workflows[row].RunID
		}
	}
	if idx < 0 {
		wl.hasListAnchor = false
		return
	}
	wl.listAnchorIndex = idx
	wl.hasListAnchor = true
}

func (wl *WorkflowList) applyServerCounts(counts temporal.WorkflowCounts) {
	wl.serverStats = WorkflowStats{
		Running:        counts.Running,
		Completed:      counts.Completed,
		Failed:         counts.Failed,
		Canceled:       counts.Canceled,
		Terminated:     counts.Terminated,
		TimedOut:       counts.TimedOut,
		ContinuedAsNew: counts.ContinuedAsNew,
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
			StartTime: now.Add(-10 * time.Minute), TaskFailure: true,
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
	rowOff, colOff, horiz := wl.workflowTableScroll()
	cols := wl.columnLayout()
	row := wl.table.SelectedRow()
	wl.table.ClearRows()
	applyWorkflowColumnHeaders(wl.table, cols)
	now := time.Now()
	for i, w := range wl.workflows {
		wl.table.AddStyledRow(wl.styledWorkflowCells(now, w, i))
	}
	if row >= 0 && row < wl.table.RowCount() {
		wl.selectWorkflowRow(row)
	}
	wl.restoreWorkflowTableScroll(rowOff, colOff, horiz)
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

func (wl *WorkflowList) workflowTableScroll() (rowOffset, colOffset, horiz int) {
	if wl != nil && wl.table != nil {
		rowOffset, colOffset = wl.table.GetOffset()
	}
	if wl != nil && wl.tableScroll != nil {
		horiz = wl.tableScroll.offset
	}
	return rowOffset, colOffset, horiz
}

func (wl *WorkflowList) restoreWorkflowTableScroll(rowOffset, colOffset, horiz int) {
	if wl == nil {
		return
	}
	if wl.table != nil {
		wl.table.SetOffset(rowOffset, colOffset)
	}
	if wl.tableScroll != nil {
		wl.tableScroll.scrollTo(horiz)
	}
}

func (wl *WorkflowList) selectWorkflowRow(idx int) {
	if wl == nil || wl.table == nil || idx < 0 {
		return
	}
	if wl.table.SelectedRow() == idx {
		return
	}
	wl.table.SelectRow(idx)
}

func (wl *WorkflowList) populateTable() {
	rowOff, colOff, horiz := wl.workflowTableScroll()
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
		wl.applyProfileTitle()
		wl.clearPreview()
		return
	}

	wl.SetMasterContent(wl.table)

	now := time.Now()
	for i, w := range wl.workflows {
		wl.table.AddStyledRow(wl.styledWorkflowCells(now, w, i))
	}

	idx := workflowIndexByIdentity(wl.workflows, id, runID)
	if idx < 0 {
		idx = 0
	}
	switch wl.listEdgePin {
	case listEdgeStart:
		idx = 0
		rowOff = 0
		wl.hasListAnchor = false
	case listEdgeEnd:
		idx = len(wl.workflows) - 1
		rowOff = 0
		wl.hasListAnchor = false
	default:
		if wl.hasListAnchor {
			rowOff += idx - wl.listAnchorIndex
			if rowOff < 0 {
				rowOff = 0
			}
			wl.hasListAnchor = false
		}
	}
	wl.selectWorkflowRow(idx)
	wl.restoreWorkflowTableScroll(rowOff, colOff, horiz)
	wl.rememberHighlightedWorkflow()
	wl.applyProfileTitle()
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
		case "TimedOut":
			stats.TimedOut++
		case "ContinuedAsNew":
			stats.ContinuedAsNew++
		}
	}
	return stats
}

func (wl *WorkflowList) updateStats() {
	if wl.app == nil {
		return
	}
	if !wl.workflowsActive() {
		wl.app.ClearWorkflowStats()
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

func autoRefreshMessage(on bool) string {
	if on {
		return "Auto-refresh on"
	}
	return "Auto-refresh off"
}

func (wl *WorkflowList) toggleAutoRefresh() {
	wl.autoRefresh = !wl.autoRefresh
	if wl.autoRefresh {
		wl.startAutoRefresh()
	} else {
		wl.stopAutoRefresh()
	}
	if wl.app != nil {
		wl.app.ToastSuccess(autoRefreshMessage(wl.autoRefresh))
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
