package view

import (
	"context"
	"fmt"
	"strings"
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
	selectedQueue  string
	loading        bool
	suppressSelect bool
	searchText     string
	baseTitle      string
	cache          *taskQueueCache
	pollerGen      uint64
	pollerTimer    *time.Timer
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

	// Update pollers when queue selection changes
	tq.queueTable.SetSelectionChangedFunc(func(row, col int) {
		// Skip if we're suppressing selection events (during programmatic updates)
		if tq.suppressSelect {
			return
		}
		if row > 0 && row-1 < len(tq.queues) {
			tq.loadPollers(row - 1)
		}
	})

}

func (tq *TaskQueueView) setLoading(loading bool) {
	tq.loading = loading
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
	tq.app.ShowFilterMode(tq.searchText, FilterModeCallbacks{
		OnChange: func(text string) {
			tq.applyFilter(text)
		},
		OnSubmit: func(text string) {
			tq.applyFilter(text)
		},
		OnCancel: func() {},
	})
}

// RefreshTheme updates all component colors after a theme change.
func (tq *TaskQueueView) RefreshTheme() {
	bg := theme.Bg()

	// Update main container
	tq.SetBackgroundColor(bg)

	// Update tables
	tq.queueTable.SetBackgroundColor(bg)
	tq.pollerTable.SetBackgroundColor(bg)

	// Re-render tables with new theme colors
	tq.populateQueueTable()
	if len(tq.queues) > 0 && tq.queueTable.SelectedRow() >= 0 {
		tq.populatePollerTable(tq.queues[tq.queueTable.SelectedRow()].Type)
	}
}

func (tq *TaskQueueView) loadData() {
	provider := tq.app.Provider()
	if provider == nil {
		tq.loadMockQueues()
		return
	}

	tq.setLoading(true)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		names, err := provider.ListTaskQueueNames(ctx, tq.app.CurrentNamespace())

		tq.app.JigApp().QueueUpdateDraw(func() {
			tq.setLoading(false)
			if err != nil {
				tq.showQueueError(err)
				return
			}

			tq.allQueues = []taskQueueEntry{}
			for _, name := range names {
				tq.allQueues = append(tq.allQueues, taskQueueEntry{
					Name:        name,
					Type:        "Combined",
					PollerCount: 0,
					Backlog:     0,
				})
			}

			if len(tq.allQueues) == 0 {
				tq.allQueues = append(tq.allQueues, taskQueueEntry{
					Name:        "(no task queues found)",
					Type:        "-",
					PollerCount: 0,
					Backlog:     0,
				})
			}

			tq.applyFilter(tq.searchText)

			if len(tq.queues) > 0 && tq.queues[0].Name != "(no task queues found)" {
				row := tq.queueTable.SelectedRow()
				if row < 0 || row >= len(tq.queues) {
					row = 0
				}
				tq.loadPollers(row)
			}
		})
	}()
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
	// Preserve current selection
	currentRow := tq.queueTable.SelectedRow()

	tq.queueTable.ClearRows()
	tq.queueTable.SetHeaders("NAME", "TYPE", "POLLERS", "BACKLOG")

	for _, q := range tq.queues {
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

		// Track row position before adding
		tableRow := tq.queueTable.Table.GetRowCount()
		tq.queueTable.AddRow(
			theme.IconTaskQueue+" "+q.Name,
			typeIcon+" "+q.Type,
			fmt.Sprintf("%d", q.PollerCount),
			fmt.Sprintf("%s %d", backlogIcon, q.Backlog),
		)
		// Color the backlog cell
		cell := tq.queueTable.GetCell(tableRow, 3)
		cell.SetTextColor(backlogColor)
	}

	if tq.queueTable.RowCount() > 0 {
		// Only manage suppressSelect if it's not already being managed by caller
		wasSuppress := tq.suppressSelect
		if !wasSuppress {
			tq.suppressSelect = true
		}
		// Restore previous selection if valid, otherwise select first row
		if currentRow >= 0 && currentRow < len(tq.queues) {
			tq.queueTable.SelectRow(currentRow)
		} else {
			tq.queueTable.SelectRow(0)
		}
		if !wasSuppress {
			tq.suppressSelect = false
		}
	}
}

func (tq *TaskQueueView) namespace() string {
	if tq.app == nil {
		return ""
	}
	return tq.app.CurrentNamespace()
}

func (tq *TaskQueueView) loadPollers(queueIndex int) {
	tq.schedulePollers(queueIndex, false)
}

func (tq *TaskQueueView) schedulePollers(queueIndex int, force bool) {
	if queueIndex < 0 || queueIndex >= len(tq.queues) {
		return
	}

	queue := tq.queues[queueIndex]
	if queue.Name == "" || queue.Name == "(no task queues found)" {
		return
	}
	tq.selectedQueue = queue.Name

	if !force {
		if entry, ok := tq.cache.get(tq.namespace(), queue.Name); ok {
			tq.applyPollerCache(queue.Name, entry)
			return
		}
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, pollers, err := provider.DescribeTaskQueue(ctx, tq.namespace(), queue.Name)
	if atomic.LoadUint64(&tq.pollerGen) != gen {
		return
	}

	apply := func() {
		if err != nil {
			if tq.selectedQueue == queue.Name {
				tq.showPollerError(err)
			}
			return
		}
		entry := taskQueueCacheEntry{pollers: pollers}
		if info != nil {
			entry.pollerCount = info.PollerCount
			entry.backlog = info.Backlog
		}
		tq.cache.put(tq.namespace(), queue.Name, entry)
		if tq.selectedQueue != queue.Name {
			return
		}
		tq.applyPollerCache(queue.Name, entry)
	}
	if tq.app.JigApp() != nil {
		tq.app.JigApp().QueueUpdateDraw(apply)
		return
	}
	apply()
}

func (tq *TaskQueueView) applyPollerCache(queueName string, entry taskQueueCacheEntry) {
	tq.pollers = copyPollers(entry.pollers)
	tq.updateQueueStats(queueName, entry.pollerCount, entry.backlog)
	tq.populatePollerTable("")
}

func (tq *TaskQueueView) updateQueueStats(name string, pollerCount, backlog int) {
	for i := range tq.queues {
		if tq.queues[i].Name == name {
			tq.queues[i].PollerCount = pollerCount
			tq.queues[i].Backlog = backlog
		}
	}
	for i := range tq.allQueues {
		if tq.allQueues[i].Name == name {
			tq.allQueues[i].PollerCount = pollerCount
			tq.allQueues[i].Backlog = backlog
		}
	}
	row := tq.queueTable.SelectedRow()
	tq.suppressSelect = true
	tq.populateQueueTable()
	if row >= 0 && row < len(tq.queues) {
		tq.queueTable.SelectRow(row)
	}
	tq.suppressSelect = false
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

	now := time.Now()
	for _, p := range tq.pollers {
		// Filter by queue type if specified
		if queueType != "" && p.TaskQueueType != queueType {
			continue
		}

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

func (tq *TaskQueueView) showPollerError(err error) {
	tq.pollerTable.ClearRows()
	tq.pollerTable.SetHeaders("IDENTITY", "TYPE", "LAST ACCESS")
	tq.pollerTable.AddRowWithColor(theme.Error(),
		theme.IconError+" Error loading pollers",
		err.Error(),
		"",
	)
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

// Start is called when the view becomes active.
func (tq *TaskQueueView) Start() {
	if len(tq.allQueues) > 0 {
		tq.applyFilter(tq.searchText)
		row := tq.queueTable.SelectedRow()
		if row >= 0 && row < len(tq.queues) {
			tq.schedulePollers(row, false)
		}
		return
	}
	tq.loadData()
}

// Stop is called when the view is deactivated.
func (tq *TaskQueueView) Stop() {
	if tq.pollerTimer != nil {
		tq.pollerTimer.Stop()
	}
	tq.queueTable.SetInputCapture(nil)
	tq.pollerTable.SetInputCapture(nil)
}

// Hints returns keybinding hints for this view.
func (tq *TaskQueueView) Hints() []KeyHint {
	return []KeyHint{
		{Key: "/", Description: "Search"},
		{Key: "r", Description: "Refresh"},
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
