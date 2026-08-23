package view

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/rivo/tview"
)

type workerEntry struct {
	Identity   string
	Queues     []string
	Types      []string
	LastAccess time.Time
}

type WorkerView struct {
	app        *App
	table      *components.Table
	allWorkers []workerEntry
	workers    []workerEntry
	searchText string
	loading    bool
}

func NewWorkerView(app *App) *WorkerView {
	wv := &WorkerView{
		app:   app,
		table: components.NewTable(),
	}
	wv.setup()
	return wv
}

func (wv *WorkerView) setup() {
	wv.table.SetHeaders("IDENTITY", "QUEUES", "TYPE", "LAST ACCESS")
	wv.table.SetBorder(false)
	wv.table.SetBackgroundColor(theme.Bg())
	wv.table.ConfigureEmpty(theme.IconUsers, "No Workers", "No pollers found in this namespace")
}

func (wv *WorkerView) RefreshTheme() {
	wv.table.SetBackgroundColor(theme.Bg())
	wv.populateTable()
}

func (wv *WorkerView) applyFilter(query string) {
	wv.searchText = query
	if query == "" {
		wv.workers = wv.allWorkers
	} else {
		wv.workers = nil
		q := strings.ToLower(query)
		for _, w := range wv.allWorkers {
			if strings.Contains(strings.ToLower(w.Identity), q) ||
				strings.Contains(strings.ToLower(strings.Join(w.Queues, " ")), q) ||
				strings.Contains(strings.ToLower(strings.Join(w.Types, " ")), q) {
				wv.workers = append(wv.workers, w)
			}
		}
	}
	wv.populateTable()
}

func (wv *WorkerView) showSearch() {
	if wv.app == nil {
		return
	}
	wv.app.ShowFilterMode(wv.searchText, FilterModeCallbacks{
		OnChange: func(text string) {
			wv.applyFilter(text)
		},
		OnSubmit: func(text string) {
			wv.applyFilter(text)
		},
		OnCancel: func() {},
	})
}

func (wv *WorkerView) loadData() {
	provider := wv.app.Provider()
	if provider == nil {
		wv.loadMockData()
		return
	}

	wv.loading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		namespace := ""
		if wv.app != nil {
			namespace = wv.app.CurrentNamespace()
		}
		workflows, _, err := provider.ListWorkflows(ctx, namespace, temporal.ListOptions{PageSize: 100})
		if err != nil {
			if wv.app != nil && wv.app.JigApp() != nil {
				wv.app.JigApp().QueueUpdateDraw(func() {
					wv.loading = false
					wv.showError(err)
				})
			}
			return
		}

		queueSet := make(map[string]struct{})
		for _, wf := range workflows {
			if wf.TaskQueue != "" {
				queueSet[wf.TaskQueue] = struct{}{}
			}
		}

		byIdentity := map[string]*workerEntry{}
		for name := range queueSet {
			_, pollers, descErr := provider.DescribeTaskQueue(ctx, namespace, name)
			if descErr != nil {
				continue
			}
			mergeWorkerPollers(byIdentity, name, pollers)
		}

		workers := workerEntriesFromMap(byIdentity)
		if wv.app != nil && wv.app.JigApp() != nil {
			wv.app.JigApp().QueueUpdateDraw(func() {
				wv.loading = false
				wv.allWorkers = workers
				wv.applyFilter(wv.searchText)
			})
			return
		}
		wv.loading = false
		wv.allWorkers = workers
		wv.applyFilter(wv.searchText)
	}()
}

func (wv *WorkerView) loadMockData() {
	now := time.Now()
	wv.allWorkers = []workerEntry{
		{Identity: "worker-1@host-001", Queues: []string{"order-tasks", "payment-tasks"}, Types: []string{"Activity", "Workflow"}, LastAccess: now.Add(-3 * time.Second)},
		{Identity: "worker-2@host-002", Queues: []string{"order-tasks", "shipment-tasks"}, Types: []string{"Activity", "Workflow"}, LastAccess: now.Add(-2 * time.Second)},
		{Identity: "worker-3@host-003", Queues: []string{"shipment-tasks", "notification-tasks"}, Types: []string{"Activity"}, LastAccess: now.Add(-1 * time.Second)},
	}
	wv.applyFilter(wv.searchText)
}

func (wv *WorkerView) showError(err error) {
	wv.table.ClearRows()
	wv.table.SetHeaders("IDENTITY", "QUEUES", "TYPE", "LAST ACCESS")
	wv.table.AddRowWithColor(theme.Error(), "Error loading workers", err.Error(), "", "")
}

func (wv *WorkerView) populateTable() {
	currentRow := wv.table.SelectedRow()
	wv.table.ClearRows()
	wv.table.SetHeaders("IDENTITY", "QUEUES", "TYPE", "LAST ACCESS")
	now := time.Now()
	for _, w := range wv.workers {
		wv.table.AddRow(
			theme.IconUsers+" "+w.Identity,
			truncate(strings.Join(w.Queues, ", "), 36),
			strings.Join(w.Types, ", "),
			formatRelativeTime(now, w.LastAccess),
		)
	}
	if wv.table.RowCount() == 0 {
		return
	}
	if currentRow >= 0 && currentRow < len(wv.workers) {
		wv.table.SelectRow(currentRow)
		return
	}
	wv.table.SelectRow(0)
}

func (wv *WorkerView) Start() {
	if len(wv.allWorkers) > 0 {
		wv.applyFilter(wv.searchText)
		return
	}
	wv.loadData()
}

func (wv *WorkerView) Stop() {
	wv.table.SetInputCapture(nil)
}

func mergeWorkerPollers(byIdentity map[string]*workerEntry, queue string, pollers []temporal.Poller) {
	for _, p := range pollers {
		if p.Identity == "" {
			continue
		}
		entry := byIdentity[p.Identity]
		if entry == nil {
			entry = &workerEntry{Identity: p.Identity}
			byIdentity[p.Identity] = entry
		}
		entry.Queues = appendUnique(entry.Queues, queue)
		entry.Types = appendUnique(entry.Types, p.TaskQueueType)
		if p.LastAccessTime.After(entry.LastAccess) {
			entry.LastAccess = p.LastAccessTime
		}
	}
}

func workerEntriesFromMap(byIdentity map[string]*workerEntry) []workerEntry {
	workers := make([]workerEntry, 0, len(byIdentity))
	for _, entry := range byIdentity {
		sort.Strings(entry.Queues)
		sort.Strings(entry.Types)
		workers = append(workers, *entry)
	}
	sort.Slice(workers, func(i, j int) bool {
		if workers[i].LastAccess.Equal(workers[j].LastAccess) {
			return workers[i].Identity < workers[j].Identity
		}
		return workers[i].LastAccess.After(workers[j].LastAccess)
	})
	return workers
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (wv *WorkerView) Focus(delegate func(p tview.Primitive)) {
	delegate(wv.table)
}
