package view

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// taskQueueEntry represents a task queue in the list.
type taskQueueEntry struct {
	Name        string
	Type        string
	PollerCount int
	Backlog     int
}

// TaskQueueView displays task queue information.
type TaskQueueView struct {
	*tview.Flex
	app            *App
	queueTable     *components.Table
	queueScroll    *charScrollView
	pollerTable    *components.Table
	pollerScroll   *charScrollView
	queuePanel     *components.Panel
	pollerPanel    *components.Panel
	allQueues      []taskQueueEntry // Full unfiltered list
	queues         []taskQueueEntry // Filtered list for display
	pollers        []temporal.Poller
	visiblePollers []temporal.Poller // Rows currently in the poller table
	selectedQueue  string
	loading        bool
	suppressSelect bool
	searchText     string
	baseTitle      string
	cache          *taskQueueCache
	pollerGen      uint64
	statsGen       uint64
	pollerTimer    *time.Timer
	autoRefresh    bool
	liveBusy       bool
	fetchGen       uint64
	fetchCancel    func()
	refreshTicker  *time.Ticker
	stopRefresh    chan struct{}
}

// NewTaskQueueView creates a new task queue view.
func NewTaskQueueView(app *App) *TaskQueueView {
	tq := &TaskQueueView{
		Flex:        tview.NewFlex().SetDirection(tview.FlexColumn),
		app:         app,
		queueTable:  components.NewTable(),
		pollerTable: components.NewTable(),
		queues:      []taskQueueEntry{},
		pollers:     []temporal.Poller{},
		cache:       newTaskQueueCache(previewCacheLimit(app)),
		stopRefresh: make(chan struct{}, 1),
	}
	tq.setup()

	// Register for automatic theme refresh
	theme.RegisterRefreshable(tq)

	return tq
}

func (tq *TaskQueueView) setup() {
	tq.SetBackgroundColor(theme.Bg())

	// Task queues table
	tq.queueTable.SetHeaders("NAME", "TYPE", "POLLERS", "BACKLOG")
	tq.queueTable.SetBorder(false)
	tq.queueTable.SetBackgroundColor(theme.Bg())
	tq.queueTable.SetEvaluateAllRows(true)
	tq.queueScroll = attachTableCharScroll(tq.queueTable, tq.app)

	// Pollers table
	tq.pollerTable.SetHeaders("IDENTITY", "TYPE", "LAST ACCESS")
	tq.pollerTable.SetBorder(false)
	tq.pollerTable.SetBackgroundColor(theme.Bg())
	tq.pollerTable.SetEvaluateAllRows(true)
	tq.pollerScroll = attachTableCharScroll(tq.pollerTable, tq.app)

	// Create panels with icons (blubber pattern)
	tq.baseTitle = fmt.Sprintf("%s Task Queues", theme.IconTaskQueue)
	tq.queuePanel = components.NewPanel().SetTitle(tq.baseTitle)

	tq.pollerPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Pollers", theme.IconActivity))
	tq.pollerPanel.SetContent(tq.pollerScroll)

	tq.queueTable.SetSelectionChangedFunc(func(_, _ int) {
		if tq.suppressSelect {
			return
		}
		idx := tq.queueTable.SelectedRow()
		if idx >= 0 && idx < len(tq.queues) {
			tq.loadPollers(idx)
		}
	})

}

func (tq *TaskQueueView) setLoading(loading bool) {
	tq.setLoadIndicator(loading, false)
}

func (tq *TaskQueueView) setRefreshing(loading bool) {
	tq.setLoadIndicator(loading, true)
}

func (tq *TaskQueueView) setLoadIndicator(loading, quiet bool) {
	tq.loading = loading
	if tq.app == nil {
		return
	}
	if quiet {
		tq.app.SetViewRefreshing("task-queues", loading)
		return
	}
	tq.app.SetViewLoading("task-queues", loading)
}

func (tq *TaskQueueView) applyFilter(query string) {
	tq.searchText = query
	tq.updateTitle()
	if query == "" {
		tq.queues = tq.allQueues
	} else {
		tq.queues = nil
		q := strings.ToLower(query)
		for _, queue := range tq.allQueues {
			if strings.Contains(strings.ToLower(queue.Name), q) ||
				strings.Contains(strings.ToLower(queue.Type), q) {
				tq.queues = append(tq.queues, queue)
			}
		}
	}
	tq.populateQueueTable()
}

func (tq *TaskQueueView) updateTitle() {
	if tq.searchText == "" {
		tq.queuePanel.SetTitle(tq.baseTitle)
	} else {
		tq.queuePanel.SetTitle(tq.baseTitle + " (/" + tq.searchText + ")")
	}
}

func (tq *TaskQueueView) showSearch() {
	tq.app.ShowSearchPrompt(tq.searchText, tq.applyFilter, nil)
}

// RefreshTheme updates all component colors after a theme change.
func (tq *TaskQueueView) RefreshTheme() {
	bg := theme.Bg()

	tq.SetBackgroundColor(bg)
	tq.queueTable.SetBackgroundColor(bg)
	tq.pollerTable.SetBackgroundColor(bg)
	if tq.queueScroll != nil {
		tq.queueScroll.SetBackgroundColor(bg)
	}
	if tq.pollerScroll != nil {
		tq.pollerScroll.SetBackgroundColor(bg)
	}

	// Re-render tables with new theme colors
	tq.populateQueueTable()
	if len(tq.queues) > 0 && tq.queueTable.SelectedRow() >= 0 {
		tq.populatePollerTable(tq.queues[tq.queueTable.SelectedRow()].Type)
	}
}

func (tq *TaskQueueView) loadData() {
	tq.loadQueueList(false)
}

func (tq *TaskQueueView) loadQueueList(force bool) {
	provider := tq.app.Provider()
	if provider == nil {
		tq.loadMockQueues()
		return
	}

	if !force && tq.applyCachedCatalogQueues() {
		tq.prefetchQueueStats()
		return
	}

	ctx, cancel, gen := tq.beginFetch(20 * time.Second)
	tq.liveBusy = true
	tq.setLoading(true)
	go func() {
		defer cancel()
		names, err := provider.ListTaskQueueNames(ctx, tq.app.CurrentNamespace())
		tq.onUI(func() {
			if gen != tq.fetchGen {
				return
			}
			tq.liveBusy = false
			tq.setLoading(false)
			if err != nil {
				tq.showQueueError(err)
				return
			}
			tq.applyQueueNames(names)
			tq.prefetchQueueStats()
		})
	}()
}

// beginFetch starts a queue-list fetch and stops the one already running, so a
// manual refresh during auto-refresh does not apply both results.
func (tq *TaskQueueView) beginFetch(timeout time.Duration) (context.Context, func(), uint64) {
	if tq.fetchCancel != nil {
		tq.fetchCancel()
	}
	tq.fetchGen++
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	tq.fetchCancel = cancel
	return ctx, cancel, tq.fetchGen
}

func (tq *TaskQueueView) applyCachedCatalogQueues() bool {
	if tq == nil || tq.app == nil {
		return false
	}
	_, queues, ok := tq.app.catalogSuggestions(tq.namespace())
	if !ok || len(queues) == 0 {
		return false
	}
	tq.applyQueueNames(queues)
	return true
}

const noTaskQueuesName = "(no task queues found)"

func isPlaceholderQueue(name string) bool {
	return name == "" || name == noTaskQueuesName
}

func (tq *TaskQueueView) applyQueueNames(names []string) {
	tq.replaceQueueNames(names, false)
	if len(tq.queues) > 0 && !isPlaceholderQueue(tq.queues[0].Name) {
		row := tq.queueTable.SelectedRow()
		if row < 0 || row >= len(tq.queues) {
			row = 0
		}
		tq.loadPollers(row)
	}
}

func (tq *TaskQueueView) mergeQueueNames(names []string) {
	tq.replaceQueueNames(names, true)
}

func (tq *TaskQueueView) replaceQueueNames(names []string, keepStats bool) {
	selected := tq.selectedQueueName()
	existing := map[string]taskQueueEntry{}
	if keepStats {
		for _, q := range tq.allQueues {
			if !isPlaceholderQueue(q.Name) {
				existing[q.Name] = q
			}
		}
	}

	tq.allQueues = tq.allQueues[:0]
	for _, name := range names {
		if isPlaceholderQueue(name) {
			continue
		}
		if q, ok := existing[name]; ok {
			tq.allQueues = append(tq.allQueues, q)
			continue
		}
		tq.allQueues = append(tq.allQueues, taskQueueEntry{
			Name: name,
			Type: "Combined",
		})
	}
	if len(tq.allQueues) == 0 {
		tq.allQueues = append(tq.allQueues, taskQueueEntry{
			Name: noTaskQueuesName,
			Type: "-",
		})
	}

	atomic.AddUint64(&tq.statsGen, 1)
	wasSuppress := tq.suppressSelect
	tq.suppressSelect = true
	tq.applyFilter(tq.searchText)
	tq.selectQueueByName(selected)
	if idx := tq.queueTable.SelectedRow(); idx >= 0 && idx < len(tq.queues) {
		tq.selectedQueue = tq.queues[idx].Name
	}
	if !wasSuppress {
		tq.suppressSelect = false
	}
}

func (tq *TaskQueueView) selectedQueueName() string {
	if tq.queueTable == nil {
		return tq.selectedQueue
	}
	if idx := tq.queueTable.SelectedRow(); idx >= 0 && idx < len(tq.queues) {
		return tq.queues[idx].Name
	}
	return tq.selectedQueue
}

func (tq *TaskQueueView) selectQueueByName(name string) {
	if isPlaceholderQueue(name) || tq.queueTable == nil {
		return
	}
	for i, q := range tq.queues {
		if q.Name == name {
			wasSuppress := tq.suppressSelect
			tq.suppressSelect = true
			tq.queueTable.SelectRow(i)
			if !wasSuppress {
				tq.suppressSelect = false
			}
			return
		}
	}
}

func (tq *TaskQueueView) showQueueError(err error) {
	tq.queueTable.ClearRows()
	tq.queueTable.SetHeaders("NAME", "TYPE", "POLLERS", "BACKLOG")
	tq.queueTable.AddRowWithColor(theme.Error(),
		"Error loading task queues",
		err.Error(),
		"",
		"",
	)
}

func (tq *TaskQueueView) loadMockQueues() {
	tq.allQueues = []taskQueueEntry{
		{Name: "order-tasks", Type: "Combined", PollerCount: 5, Backlog: 12},
		{Name: "payment-tasks", Type: "Combined", PollerCount: 3, Backlog: 0},
		{Name: "shipment-tasks", Type: "Combined", PollerCount: 2, Backlog: 5},
		{Name: "notification-tasks", Type: "Combined", PollerCount: 2, Backlog: 0},
	}
	tq.applyFilter(tq.searchText)
}

func (tq *TaskQueueView) populateQueueTable() {
	currentRow := tq.queueTable.SelectedRow()
	wasSuppress := tq.suppressSelect
	tq.suppressSelect = true

	tq.queueTable.ClearRows()
	tq.queueTable.SetHeaders("NAME", "TYPE", "POLLERS", "BACKLOG")

	for _, q := range tq.queues {
		cells, colors := queueRowCells(q)
		tableRow := tq.queueTable.Table.GetRowCount()
		tq.queueTable.AddRow(cells...)
		cell := tq.queueTable.GetCell(tableRow, 3)
		cell.SetTextColor(colors[3])
	}

	if tq.queueTable.RowCount() > 0 {
		if currentRow >= 0 && currentRow < len(tq.queues) {
			tq.queueTable.SelectRow(currentRow)
		} else {
			tq.queueTable.SelectRow(0)
		}
	}
	if !wasSuppress {
		tq.suppressSelect = false
	}
}

func (tq *TaskQueueView) namespace() string {
	if tq.app == nil {
		return ""
	}
	return tq.app.CurrentNamespace()
}

func queueRowCells(q taskQueueEntry) (cells []string, colors []tcell.Color) {
	backlogIcon := theme.IconCompleted
	backlogColor := temporal.StatusCompleted.Color()
	if q.Backlog > 50 {
		backlogIcon = theme.IconError
		backlogColor = temporal.StatusFailed.Color()
	} else if q.Backlog > 10 {
		backlogIcon = theme.IconRunning
		backlogColor = temporal.StatusRunning.Color()
	}
	typeIcon := theme.IconWorkflow
	if q.Type == "Activity" {
		typeIcon = theme.IconActivity
	}
	return []string{
		theme.IconTaskQueue + " " + q.Name,
		typeIcon + " " + q.Type,
		fmt.Sprintf("%d", q.PollerCount),
		fmt.Sprintf("%s %d", backlogIcon, q.Backlog),
	}, []tcell.Color{
		0, 0, 0, backlogColor,
	}
}

func (tq *TaskQueueView) paintQueueRow(index int) {
	if index < 0 || index >= len(tq.queues) || tq.queueTable == nil {
		return
	}
	cells, colors := queueRowCells(tq.queues[index])
	_ = tq.queueTable.UpdateColoredRow(index, cells, colors)
}

func (tq *TaskQueueView) onUI(fn func()) {
	if tq != nil && tq.app != nil && tq.app.JigApp() != nil {
		tq.app.JigApp().QueueUpdateDraw(fn)
		return
	}
	fn()
}

func (tq *TaskQueueView) loadPollers(queueIndex int) {
	tq.schedulePollers(queueIndex, false)
}

func (tq *TaskQueueView) schedulePollers(queueIndex int, force bool) {
	if queueIndex < 0 || queueIndex >= len(tq.queues) {
		return
	}

	queue := tq.queues[queueIndex]
	if isPlaceholderQueue(queue.Name) {
		return
	}
	tq.selectedQueue = queue.Name

	if force {
		tq.cache.remove(tq.namespace(), queue.Name)
	} else if entry, ok := tq.cache.get(tq.namespace(), queue.Name); ok {
		tq.applyPollerCache(queue.Name, entry)
		return
	}

	provider := tq.app.Provider()
	if provider == nil {
		tq.loadMockPollers(queue)
		return
	}

	tq.pollerTable.ClearRows()
	tq.pollerTable.SetHeaders("IDENTITY", "TYPE", "LAST ACCESS")

	gen := atomic.AddUint64(&tq.pollerGen, 1)
	if tq.pollerTimer != nil {
		tq.pollerTimer.Stop()
	}
	tq.pollerTimer = time.AfterFunc(200*time.Millisecond, func() {
		tq.fetchPollers(gen, queue)
	})
}

func (tq *TaskQueueView) fetchPollers(gen uint64, queue taskQueueEntry) {
	if atomic.LoadUint64(&tq.pollerGen) != gen {
		return
	}
	if tq.app == nil {
		return
	}
	provider := tq.app.Provider()
	if provider == nil {
		return
	}

	if entry, ok := tq.cache.get(tq.namespace(), queue.Name); ok {
		tq.onUI(func() {
			if tq.selectedQueue == queue.Name {
				tq.applyPollerCache(queue.Name, entry)
			}
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, pollers, err := provider.DescribeTaskQueue(ctx, tq.namespace(), queue.Name)
	tq.onUI(func() {
		if err != nil {
			if tq.selectedQueue == queue.Name {
				tq.showPollerError(err)
			}
			return
		}
		tq.applyDescribedQueue(queue.Name, info, pollers)
	})
}

func (tq *TaskQueueView) queuesNeedingStats() []taskQueueEntry {
	out := make([]taskQueueEntry, 0, len(tq.allQueues))
	ns := tq.namespace()
	for _, q := range tq.allQueues {
		if isPlaceholderQueue(q.Name) {
			continue
		}
		if _, ok := tq.cache.get(ns, q.Name); ok {
			continue
		}
		out = append(out, q)
	}
	return out
}

func (tq *TaskQueueView) prefetchQueueStats() {
	if tq == nil || tq.app == nil || tq.app.Provider() == nil {
		return
	}
	queues := tq.queuesNeedingStats()
	if len(queues) == 0 {
		return
	}
	gen := atomic.LoadUint64(&tq.statsGen)
	if gen == 0 {
		gen = atomic.AddUint64(&tq.statsGen, 1)
	}
	run := func() { tq.fetchQueueStats(gen, queues) }
	if tq.app.JigApp() != nil {
		go run()
		return
	}
	run()
}

func (tq *TaskQueueView) fetchQueueStats(gen uint64, queues []taskQueueEntry) {
	if tq.app == nil {
		return
	}
	provider := tq.app.Provider()
	if provider == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ns := tq.namespace()

	type result struct {
		name    string
		info    *temporal.TaskQueueInfo
		pollers []temporal.Poller
		err     error
	}
	jobs := make(chan taskQueueEntry)
	out := make(chan result)
	workers := workerQueueSweepConcurrency
	if len(queues) < workers {
		workers = len(queues)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range jobs {
				if atomic.LoadUint64(&tq.statsGen) != gen {
					continue
				}
				info, pollers, err := provider.DescribeTaskQueue(ctx, ns, q.Name)
				out <- result{name: q.Name, info: info, pollers: pollers, err: err}
			}
		}()
	}
	go func() {
		for _, q := range queues {
			jobs <- q
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()
	for r := range out {
		if r.err != nil || atomic.LoadUint64(&tq.statsGen) != gen {
			continue
		}
		res := r
		tq.onUI(func() {
			if atomic.LoadUint64(&tq.statsGen) != gen {
				return
			}
			tq.applyDescribedQueue(res.name, res.info, res.pollers)
		})
	}
}

func (tq *TaskQueueView) applyDescribedQueue(name string, info *temporal.TaskQueueInfo, pollers []temporal.Poller) {
	entry := taskQueueCacheEntry{pollers: pollers, pollerCount: len(pollers)}
	if info != nil {
		entry.pollerCount = info.PollerCount
		entry.backlog = info.Backlog
	}
	tq.cache.put(tq.namespace(), name, entry)
	tq.updateQueueStats(name, entry.pollerCount, entry.backlog)
	if tq.selectedQueue == name {
		tq.pollers = copyPollers(entry.pollers)
		tq.populatePollerTable("")
	}
}

func (tq *TaskQueueView) applyPollerCache(queueName string, entry taskQueueCacheEntry) {
	tq.pollers = copyPollers(entry.pollers)
	tq.updateQueueStats(queueName, entry.pollerCount, entry.backlog)
	tq.populatePollerTable("")
}

func (tq *TaskQueueView) updateQueueStats(name string, pollerCount, backlog int) {
	for i := range tq.allQueues {
		if tq.allQueues[i].Name == name {
			tq.allQueues[i].PollerCount = pollerCount
			tq.allQueues[i].Backlog = backlog
		}
	}
	for i := range tq.queues {
		if tq.queues[i].Name == name {
			tq.queues[i].PollerCount = pollerCount
			tq.queues[i].Backlog = backlog
			tq.paintQueueRow(i)
			return
		}
	}
}

func (tq *TaskQueueView) loadMockPollers(queue taskQueueEntry) {
	now := time.Now()
	tq.pollers = []temporal.Poller{
		{Identity: "worker-1@host-001", LastAccessTime: now.Add(-5 * time.Second), TaskQueueType: "Workflow"},
		{Identity: "worker-1@host-001", LastAccessTime: now.Add(-3 * time.Second), TaskQueueType: "Activity"},
		{Identity: "worker-2@host-002", LastAccessTime: now.Add(-10 * time.Second), TaskQueueType: "Workflow"},
		{Identity: "worker-2@host-002", LastAccessTime: now.Add(-2 * time.Second), TaskQueueType: "Activity"},
		{Identity: "worker-3@host-003", LastAccessTime: now.Add(-1 * time.Second), TaskQueueType: "Activity"},
	}
	entry := taskQueueCacheEntry{
		pollers:     tq.pollers,
		pollerCount: len(tq.pollers),
	}
	tq.cache.put(tq.namespace(), queue.Name, entry)
	tq.applyPollerCache(queue.Name, entry)
}

func (tq *TaskQueueView) populatePollerTable(queueType string) {
	tq.pollerTable.ClearRows()
	tq.pollerTable.SetHeaders("IDENTITY", "TYPE", "LAST ACCESS")
	tq.visiblePollers = nil

	now := time.Now()
	for _, p := range tq.pollers {
		// Filter by queue type if specified
		if queueType != "" && p.TaskQueueType != queueType {
			continue
		}
		tq.visiblePollers = append(tq.visiblePollers, p)

		typeIcon := theme.IconWorkflow
		if p.TaskQueueType == "Activity" {
			typeIcon = theme.IconActivity
		}

		lastAccess := formatRelativeTime(now, p.LastAccessTime)
		tq.pollerTable.AddRow(
			theme.IconConnected+" "+p.Identity,
			typeIcon+" "+p.TaskQueueType,
			lastAccess,
		)
	}
}

// selectedPoller returns the poller under the cursor in the poller table.
func (tq *TaskQueueView) selectedPoller() (temporal.Poller, bool) {
	if tq == nil || tq.pollerTable == nil {
		return temporal.Poller{}, false
	}
	idx := tq.pollerTable.SelectedRow()
	if idx < 0 || idx >= len(tq.visiblePollers) {
		return temporal.Poller{}, false
	}
	return tq.visiblePollers[idx], true
}

func (tq *TaskQueueView) showPollerError(err error) {
	tq.pollerTable.ClearRows()
	tq.pollerTable.SetHeaders("IDENTITY", "TYPE", "LAST ACCESS")
	tq.visiblePollers = nil
	tq.pollerTable.AddRowWithColor(theme.Error(),
		theme.IconError+" Error loading pollers",
		err.Error(),
		"",
	)
}

// refresh reloads the queue list and its pollers from the server, dropping
// every cached queue first.
func (tq *TaskQueueView) refresh() {
	tq.cache.clear()
	tq.loadQueueList(true)
}

func (tq *TaskQueueView) refreshCurrentQueue() {
	row := tq.queueTable.SelectedRow()
	if row >= 0 && row < len(tq.queues) {
		tq.schedulePollers(row, true)
	}
}

// Name returns the view name.
func (tq *TaskQueueView) Name() string {
	return "task-queues"
}

func (tq *TaskQueueView) toggleAutoRefresh() {
	tq.autoRefresh = !tq.autoRefresh
	if tq.autoRefresh {
		tq.startAutoRefresh()
	} else {
		tq.stopAutoRefresh()
	}
	if tq.app != nil {
		tq.app.ToastSuccess(autoRefreshMessage(tq.autoRefresh))
	}
}

func (tq *TaskQueueView) syncAutoRefresh() {
	if !tq.autoRefresh {
		return
	}
	tq.stopAutoRefresh()
	tq.startAutoRefresh()
}

func (tq *TaskQueueView) startAutoRefresh() {
	tq.stopAutoRefresh()
	select {
	case <-tq.stopRefresh:
	default:
	}

	ticker := time.NewTicker(refreshRate(tq.app))
	tq.refreshTicker = ticker
	stop := tq.stopRefresh
	go func() {
		for {
			select {
			case <-ticker.C:
				tq.onUI(func() {
					tq.liveRefresh()
				})
			case <-stop:
				return
			}
		}
	}()
}

func (tq *TaskQueueView) stopAutoRefresh() {
	if tq.refreshTicker != nil {
		tq.refreshTicker.Stop()
		tq.refreshTicker = nil
	}
	if tq.stopRefresh == nil {
		return
	}
	select {
	case tq.stopRefresh <- struct{}{}:
	default:
	}
}

func (tq *TaskQueueView) liveRefresh() {
	if tq.liveBusy || tq.app == nil {
		return
	}
	provider := tq.app.Provider()
	if provider == nil {
		return
	}
	ctx, cancel, gen := tq.beginFetch(10 * time.Second)
	tq.liveBusy = true
	tq.setRefreshing(true)
	go func() {
		defer cancel()
		names, err := provider.ListTaskQueueNames(ctx, tq.namespace())
		tq.onUI(func() {
			if gen != tq.fetchGen {
				return
			}
			tq.liveBusy = false
			tq.setLoading(false)
			if err != nil {
				return
			}
			tq.mergeQueueNames(names)
			tq.cache.clear()
			tq.prefetchQueueStats()
		})
	}()
}

// Start is called when the view becomes active.
func (tq *TaskQueueView) Start() {
	if tq.autoRefresh {
		tq.startAutoRefresh()
	}
	if len(tq.allQueues) > 0 {
		tq.applyFilter(tq.searchText)
		row := tq.queueTable.SelectedRow()
		if row >= 0 && row < len(tq.queues) {
			tq.schedulePollers(row, false)
		}
		tq.prefetchQueueStats()
		return
	}
	tq.loadData()
}

// Stop is called when the view is deactivated.
func (tq *TaskQueueView) Stop() {
	if tq.pollerTimer != nil {
		tq.pollerTimer.Stop()
	}
	tq.stopAutoRefresh()
	tq.queueTable.SetInputCapture(nil)
	tq.pollerTable.SetInputCapture(nil)
}

// Hints returns keybinding hints for this view.
func (tq *TaskQueueView) Hints() []KeyHint {
	return []KeyHint{
		{Key: "/", Description: "Search"},
		{Key: "r", Description: "Refresh"},
		{Key: "a", Description: "Auto-refresh"},
		{Key: "T", Description: "Theme"},
		{Key: "esc", Description: "Back"},
	}
}

// Focus sets focus to the queue table.
func (tq *TaskQueueView) Focus(delegate func(p tview.Primitive)) {
	delegate(tq.queueTable)
}

// Draw applies theme colors dynamically and draws the view.
func (tq *TaskQueueView) Draw(screen tcell.Screen) {
	bg := theme.Bg()
	tq.SetBackgroundColor(bg)
	tq.Flex.Draw(screen)
}
