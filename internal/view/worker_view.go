package view

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/rivo/tview"
)

// workerUtilizationHeight fits two bars plus a spacer inside the panel border.
const workerUtilizationHeight = 5

// workerReloadDebounce keeps rapid tab switches from re-running the whole sweep.
const workerReloadDebounce = 3 * time.Second

// workerRow is one worker instance: an identity polling one task queue.
type workerRow struct {
	Worker temporal.Worker
}

type WorkerView struct {
	app          *App
	table        *components.Table
	tableScroll  *charScrollView
	detail       *components.Table
	detailScroll *charScrollView
	detailRows   []workflowInfoRow
	previewPanel *components.Panel
	cpuBar       *utilizationBar
	memBar       *utilizationBar
	utilPanel    *components.Panel
	detailFlex   *tview.Flex
	allWorkers   []temporal.Worker
	workers      []temporal.Worker // Filtered instances, in display order
	rows         []workerRow
	searchText   string
	loading      bool
	lastLoad     time.Time
	pending      *workerInstanceRequest // Selection waiting on a load
}

// workerInstanceRequest is a poller we were asked to reveal in the worker tree.
type workerInstanceRequest struct {
	Identity  string
	TaskQueue string
}

func NewWorkerView(app *App) *WorkerView {
	wv := &WorkerView{
		app:    app,
		table:  components.NewTable(),
		detail: components.NewTable(),
	}
	wv.setup()
	return wv
}

func (wv *WorkerView) setup() {
	wv.table.SetHeaders(workerTableHeaders()...)
	wv.table.SetBorder(false)
	wv.table.SetEvaluateAllRows(true)
	wv.table.SetBackgroundColor(theme.Bg())
	wv.table.ConfigureEmpty(theme.IconUsers, "No Workers", "No worker instances found in this namespace")
	wv.table.SetSelectionChangedFunc(func(row, col int) {
		wv.updatePreview()
	})

	wv.tableScroll = newCharScrollView(wv.table, func() int {
		return tableContentWidth(wv.table)
	})
	bindTableCharScroll(wv.table, wv.tableScroll, func() int {
		return mouseScrollStepFromApp(wv.app)
	})

	wv.detail.SetBorder(false)
	wv.detail.SetBackgroundColor(theme.Bg())
	wv.detail.SetEvaluateAllRows(true)
	wv.detailScroll = newCharScrollView(wv.detail, func() int {
		return workflowInfoContentWidth(wv.detailRows)
	})
	bindTableCharScroll(wv.detail, wv.detailScroll, func() int {
		return mouseScrollStepFromApp(wv.app)
	})
	wv.previewPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Worker", theme.IconUsers))
	wv.previewPanel.SetContent(wv.detailScroll)

	wv.cpuBar = newUtilizationBar("CPU")
	wv.memBar = newUtilizationBar("MEM")
	utilFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	utilFlex.SetBackgroundColor(theme.Bg())
	utilFlex.AddItem(wv.cpuBar, 1, 0, false)
	utilFlex.AddItem(tview.NewBox().SetBackgroundColor(theme.Bg()), 1, 0, false)
	utilFlex.AddItem(wv.memBar, 1, 0, false)
	wv.utilPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Utilization", theme.IconBolt))
	wv.utilPanel.SetContent(utilFlex)

	wv.detailFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	wv.detailFlex.SetBackgroundColor(theme.Bg())
	wv.detailFlex.AddItem(wv.previewPanel, 0, 1, false)
	wv.detailFlex.AddItem(wv.utilPanel, workerUtilizationHeight, 0, false)
}

func workerTableHeaders() []string {
	return []string{"INSTANCE", "STATUS", "TASK QUEUE", "HEARTBEAT", "START", "BUILD ID", "PID", "CPU", "MEM"}
}

func (wv *WorkerView) RefreshTheme() {
	wv.table.SetBackgroundColor(theme.Bg())
	wv.detail.SetBackgroundColor(theme.Bg())
	wv.populateTable()
}

func (wv *WorkerView) applyFilter(query string) {
	wv.searchText = query
	wv.workers = sortWorkerInstances(filterWorkers(wv.allWorkers, query))
	wv.rebuildRows()
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
	wv.lastLoad = time.Now()
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
		workers, err := loadWorkers(ctx, provider, namespace)
		if err != nil {
			if wv.app != nil && wv.app.JigApp() != nil {
				wv.app.JigApp().QueueUpdateDraw(func() {
					wv.loading = false
					wv.showError(err)
				})
			}
			return
		}
		apply := func() {
			wv.loading = false
			wv.allWorkers = workers
			wv.applyFilter(wv.searchText)
		}
		if wv.app != nil && wv.app.JigApp() != nil {
			wv.app.JigApp().QueueUpdateDraw(apply)
			return
		}
		apply()
	}()
}

// workerQueueSweepConcurrency bounds the DescribeTaskQueue calls that collect
// poll registries, so a namespace with many queues stays roughly one round trip.
const workerQueueSweepConcurrency = 8

// loadWorkers combines the two views the server offers of a worker: heartbeats
// from ListWorkers, and the poll registries of every task queue. Heartbeats carry
// the detail, the poll registries prove who is actually polling, and neither is a
// superset of the other, so the tab shows the union.
func loadWorkers(ctx context.Context, provider temporal.Provider, namespace string) ([]temporal.Worker, error) {
	heartbeats, listErr := provider.ListWorkers(ctx, namespace)
	polled, pollErr := loadPolledWorkers(ctx, provider, namespace)
	if listErr != nil && pollErr != nil {
		return nil, listErr
	}
	return temporal.MergeWorkerSources(heartbeats, polled, time.Now()), nil
}

// loadPolledWorkers derives instances from every task queue's poll registry.
func loadPolledWorkers(ctx context.Context, provider temporal.Provider, namespace string) ([]temporal.Worker, error) {
	names, err := provider.ListTaskQueueNames(ctx, namespace)
	if err != nil {
		return nil, err
	}

	type result struct {
		index   int
		workers []temporal.Worker
	}
	results := make([][]temporal.Worker, len(names))
	queue := make(chan int)
	out := make(chan result)
	workers := workerQueueSweepConcurrency
	if len(names) < workers {
		workers = len(names)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range queue {
				_, pollers, descErr := provider.DescribeTaskQueue(ctx, namespace, names[idx])
				if descErr != nil {
					continue
				}
				out <- result{index: idx, workers: temporal.WorkersFromPollers(names[idx], pollers)}
			}
		}()
	}
	go func() {
		for i := range names {
			queue <- i
		}
		close(queue)
		wg.Wait()
		close(out)
	}()
	for r := range out {
		results[r.index] = r.workers
	}

	// Keep task queue order stable so the tree does not shuffle between loads.
	var all []temporal.Worker
	for _, group := range results {
		all = append(all, group...)
	}
	return all, nil
}

func (wv *WorkerView) loadMockData() {
	now := time.Now()
	wv.allWorkers = []temporal.Worker{
		{
			InstanceKey: "inst-1", Identity: "worker-1", Host: "host-001", ProcessID: "4122",
			TaskQueue: "order-tasks", Types: []string{"Activity", "Workflow"}, Status: temporal.WorkerStatusRunning,
			StartTime: now.Add(-2 * time.Hour), LastHeartbeat: now.Add(-3 * time.Second),
			BuildID: "build-a", Deployment: "checkout", SDKName: "temporal-go", SDKVersion: "1.38.0",
			HasHostInfo: true, CPU: 0.18, Memory: 0.41,
			WorkflowSlots:   temporal.WorkerSlots{Used: 2, Available: 100, Processed: 1400, Failed: 3},
			ActivitySlots:   temporal.WorkerSlots{Used: 4, Available: 200, Processed: 900},
			WorkflowPollers: temporal.WorkerPollers{Current: 2, Autoscaling: true},
			ActivityPollers: temporal.WorkerPollers{Current: 4},
			StickyCacheHit:  80, StickyCacheMiss: 4, StickyCacheSize: 12,
		},
		{
			InstanceKey: "inst-2", Identity: "worker-2", Host: "host-001", ProcessID: "4123",
			TaskQueue: "payment-tasks", Types: []string{"Activity", "Workflow"}, Status: temporal.WorkerStatusRunning,
			StartTime: now.Add(-90 * time.Minute), LastHeartbeat: now.Add(-2 * time.Second),
			BuildID: "build-a", Deployment: "checkout", SDKName: "temporal-go", SDKVersion: "1.38.0",
			HasHostInfo: true, CPU: 0.18, Memory: 0.41,
		},
		{
			InstanceKey: "inst-3", Identity: "worker-3", Host: "host-002", ProcessID: "8811",
			TaskQueue: "shipment-tasks", Types: []string{"Activity"}, Status: temporal.WorkerStatusShuttingDown,
			StartTime: now.Add(-30 * time.Minute), LastHeartbeat: now.Add(-1 * time.Second),
			BuildID: "build-b", SDKName: "temporal-ts", SDKVersion: "1.13.0",
			HasHostInfo: true, CPU: 0.07, Memory: 0.22,
		},
	}
	wv.applyFilter(wv.searchText)
}

func (wv *WorkerView) showError(err error) {
	wv.table.ClearRows()
	wv.table.SetHeaders(workerTableHeaders()...)
	wv.table.AddRowWithColor(theme.Error(), "Error loading workers", err.Error(), "", "", "", "", "", "")
	wv.setDetailRows(nil)
}

func (wv *WorkerView) rebuildRows() {
	wv.rows = make([]workerRow, 0, len(wv.workers))
	for _, w := range wv.workers {
		wv.rows = append(wv.rows, workerRow{Worker: w})
	}
}

// renderRows redraws the tree without touching the selection.
func (wv *WorkerView) renderRows() {
	wv.table.ClearRows()
	wv.table.SetHeaders(workerTableHeaders()...)
	now := time.Now()
	for _, row := range wv.rows {
		wv.addInstanceRow(now, row.Worker)
	}
}

func (wv *WorkerView) populateTable() {
	current := wv.selectedKey()
	wv.renderRows()
	if wv.table.RowCount() == 0 {
		wv.setDetailRows(nil)
		wv.setUtilization(0, 0, false)
		return
	}
	if pending := wv.pending; pending != nil {
		if wv.selectInstance(pending.Identity, pending.TaskQueue) {
			wv.pending = nil
			return
		}
	}
	if idx := wv.rowIndexByKey(current); idx >= 0 {
		wv.table.SelectRow(idx)
	} else {
		wv.table.SelectRow(0)
	}
	wv.updatePreview()
}

// RevealInstance selects the worker instance behind a poller identity, expanding
// its host group. When the workers have not loaded yet the request is remembered
// and applied once they arrive. It reports whether a row was selected now.
func (wv *WorkerView) RevealInstance(identity, taskQueue string) bool {
	if wv == nil || identity == "" {
		return false
	}
	if wv.selectInstance(identity, taskQueue) {
		wv.pending = nil
		return true
	}
	wv.pending = &workerInstanceRequest{Identity: identity, TaskQueue: taskQueue}
	return false
}

// selectInstance moves the cursor onto the matching instance, if there is one.
func (wv *WorkerView) selectInstance(identity, taskQueue string) bool {
	match := matchWorkerInstance(wv.workers, identity, taskQueue)
	if match == nil {
		return false
	}
	idx := wv.rowIndexByKey(workerRowKey(workerRow{Worker: *match}))
	if idx < 0 {
		return false
	}
	wv.table.SelectRow(idx)
	wv.updatePreview()
	return true
}

// matchWorkerInstance finds the worker a poller identity belongs to. An identity
// match beats a same-host match, and within each, the instance polling the same
// task queue wins.
func matchWorkerInstance(workers []temporal.Worker, identity, taskQueue string) *temporal.Worker {
	if identity == "" {
		return nil
	}
	host := temporal.HostFromIdentity(identity)
	var best *temporal.Worker
	bestScore := 0
	for i := range workers {
		w := &workers[i]
		score := 0
		switch {
		case w.Identity == identity || w.InstanceKey == identity:
			score = 3
		case host != "" && (w.Host == host || temporal.HostFromIdentity(w.Identity) == host):
			score = 1
		default:
			continue
		}
		if taskQueue != "" && w.TaskQueue == taskQueue {
			score++
		}
		if score > bestScore {
			best, bestScore = w, score
		}
	}
	return best
}

func (wv *WorkerView) addInstanceRow(now time.Time, w temporal.Worker) {
	status := temporal.GetWorkerStatus(w.Status)
	cells := []string{
		theme.IconUser + " " + instanceLabel(w),
		w.Status,
		w.TaskQueue,
		formatWorkerTime(now, w.LastHeartbeat),
		formatWorkerTime(now, w.StartTime),
		w.BuildID,
		w.ProcessID,
		formatWorkerResource(w),
		formatWorkerMemory(w),
	}
	wv.table.AddRowWithStatus(status, 1, cells...)
}

func (wv *WorkerView) selectedRow() (workerRow, bool) {
	idx := wv.table.SelectedRow()
	if idx < 0 || idx >= len(wv.rows) {
		return workerRow{}, false
	}
	return wv.rows[idx], true
}

func (wv *WorkerView) selectedKey() string {
	row, ok := wv.selectedRow()
	if !ok {
		return ""
	}
	return workerRowKey(row)
}

func (wv *WorkerView) rowIndexByKey(key string) int {
	if key == "" {
		return -1
	}
	for i, row := range wv.rows {
		if workerRowKey(row) == key {
			return i
		}
	}
	return -1
}

func (wv *WorkerView) updatePreview() {
	row, ok := wv.selectedRow()
	if !ok {
		wv.setDetailRows(nil)
		wv.setUtilization(0, 0, false)
		return
	}
	now := time.Now()
	wv.setDetailRows(workerInfoRows(now, row.Worker))
	wv.setUtilization(row.Worker.CPU, row.Worker.Memory, row.Worker.HasHostInfo)
}

// setDetailRows renders the label/value rows for the selected host or instance.
func (wv *WorkerView) setDetailRows(rows []workflowInfoRow) {
	selectedKey := ""
	if row, ok := wv.selectedDetailRow(); ok {
		selectedKey = row.Key
	}
	wv.detailRows = rows
	if wv.detail == nil {
		return
	}
	wv.detail.ClearRows()
	for _, row := range rows {
		wv.detail.AddStyledRow([]components.TableCell{
			{Text: row.Label, Color: theme.FgDim(), Selectable: true},
			{Text: row.displayText(), Color: row.Color, Selectable: true},
		})
	}
	if idx := workflowInfoRowIndex(rows, selectedKey); idx >= 0 {
		wv.detail.SelectRow(idx)
	} else if len(rows) > 0 {
		wv.detail.SelectRow(0)
	}
	if wv.detailScroll != nil {
		wv.detailScroll.clamp()
	}
}

func (wv *WorkerView) selectedDetailRow() (workflowInfoRow, bool) {
	if wv == nil || wv.detail == nil {
		return workflowInfoRow{}, false
	}
	idx := wv.detail.SelectedRow()
	if idx < 0 || idx >= len(wv.detailRows) {
		return workflowInfoRow{}, false
	}
	return wv.detailRows[idx], true
}

// yankDetailRow copies the selected detail value to the clipboard.
func (wv *WorkerView) yankDetailRow() {
	row, ok := wv.selectedDetailRow()
	if !ok || row.Value == "" || wv.app == nil {
		return
	}
	if err := copyToClipboard(row.Value); err != nil {
		wv.app.ToastError("Failed to copy: " + err.Error())
		return
	}
	wv.app.ToastSuccess("Copied " + row.Label)
}

func (wv *WorkerView) setUtilization(cpu, mem float32, known bool) {
	if wv.cpuBar != nil {
		wv.cpuBar.setValue(cpu, known)
	}
	if wv.memBar != nil {
		wv.memBar.setValue(mem, known)
	}
}

func (wv *WorkerView) Start() {
	// Re-enter means re-read: heartbeat records outlive the process they describe,
	// so a cached list quietly shows dead workers after a restart. Show what we
	// have straight away, then refresh unless we just did.
	if len(wv.allWorkers) > 0 {
		wv.applyFilter(wv.searchText)
		if !wv.lastLoad.IsZero() && time.Since(wv.lastLoad) < workerReloadDebounce {
			return
		}
	}
	wv.loadData()
}

func (wv *WorkerView) Stop() {
	wv.table.SetInputCapture(nil)
	if wv.detail != nil {
		wv.detail.SetInputCapture(nil)
	}
}

func (wv *WorkerView) Focus(delegate func(p tview.Primitive)) {
	delegate(wv.table)
}

func filterWorkers(workers []temporal.Worker, query string) []temporal.Worker {
	if query == "" {
		return workers
	}
	q := strings.ToLower(query)
	var out []temporal.Worker
	for _, w := range workers {
		if workerMatches(w, q) {
			out = append(out, w)
		}
	}
	return out
}

func workerMatches(w temporal.Worker, q string) bool {
	fields := []string{
		w.Host, w.Identity, w.InstanceKey, w.TaskQueue, w.Status,
		w.BuildID, w.Deployment, w.ProcessID, w.SDKName, w.SDKVersion,
		strings.Join(w.Types, " "),
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}

// sortWorkerInstances keeps same-machine instances next to each other now that
// the tab lists them flat.
func sortWorkerInstances(workers []temporal.Worker) []temporal.Worker {
	out := make([]temporal.Worker, len(workers))
	copy(out, workers)
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		leftHost, rightHost := workerHost(left), workerHost(right)
		if leftHost != rightHost {
			return leftHost < rightHost
		}
		if left.Identity != right.Identity {
			return left.Identity < right.Identity
		}
		return left.TaskQueue < right.TaskQueue
	})
	return out
}

func workerHost(w temporal.Worker) string {
	if w.Host != "" {
		return w.Host
	}
	return temporal.HostFromIdentity(w.Identity)
}

func workerRowKey(row workerRow) string {
	return "inst:" + row.Worker.InstanceKey + "|" + row.Worker.Identity + "|" + row.Worker.TaskQueue
}

func instanceLabel(w temporal.Worker) string {
	if w.Identity != "" && w.Identity != w.Host {
		return w.Identity
	}
	if w.ProcessID != "" {
		return "pid " + w.ProcessID
	}
	if w.InstanceKey != "" {
		return w.InstanceKey
	}
	if w.TaskQueue != "" {
		return w.TaskQueue
	}
	return "worker"
}

func formatWorkerTime(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return formatRelativeTime(now, t)
}

func formatWorkerPercent(value float32) string {
	return fmt.Sprintf("%.0f%%", value*100)
}

func formatWorkerResource(w temporal.Worker) string {
	if !w.HasHostInfo {
		return "-"
	}
	return formatWorkerPercent(w.CPU)
}

func formatWorkerMemory(w temporal.Worker) string {
	if !w.HasHostInfo {
		return "-"
	}
	return formatWorkerPercent(w.Memory)
}

// workerInfoRows describes one worker instance as label/value rows for the detail table.
func workerInfoRows(now time.Time, w temporal.Worker) []workflowInfoRow {
	status := w.Status
	if status == "" {
		status = "Unknown"
	}
	return []workflowInfoRow{
		{Key: "instance", Label: "Instance", Value: dashIfEmpty(firstNonEmpty(w.InstanceKey, w.Identity)), Color: theme.Fg()},
		{Key: "status", Label: "Status", Value: status, Color: temporal.GetWorkerStatus(w.Status).Color()},
		{Key: "identity", Label: "Identity", Value: dashIfEmpty(w.Identity), Color: theme.Fg()},
		{Key: "taskqueue", Label: "Task Queue", Value: dashIfEmpty(w.TaskQueue), Color: theme.Fg()},
		{Key: "types", Label: "Types", Value: dashIfEmpty(strings.Join(w.Types, ", ")), Color: theme.Fg()},
		{Key: "host", Label: "Host", Value: dashIfEmpty(w.Host), Color: theme.Fg()},
		{Key: "pid", Label: "Process ID", Value: dashIfEmpty(w.ProcessID), Color: theme.Fg()},
		{Key: "buildid", Label: "Build ID", Value: dashIfEmpty(w.BuildID), Color: theme.Fg()},
		{Key: "deployment", Label: "Deployment", Value: dashIfEmpty(w.Deployment), Color: theme.Fg()},
		{Key: "sdk", Label: "SDK", Value: dashIfEmpty(strings.TrimSpace(w.SDKName + " " + w.SDKVersion)), Color: theme.Fg()},
		{Key: "started", Label: "Started", Value: formatWorkerTime(now, w.StartTime), Color: theme.Fg()},
		{Key: "heartbeat", Label: "Last Heartbeat", Value: formatWorkerTime(now, w.LastHeartbeat), Color: theme.Fg()},
		{Key: "cpu", Label: "CPU", Value: formatWorkerResource(w), Color: theme.Fg()},
		{Key: "memory", Label: "Memory", Value: formatWorkerMemory(w), Color: theme.Fg()},
		{Key: "slots-workflow", Label: "Workflow Slots", Value: workerSlotsValue(w.WorkflowSlots), Color: theme.Fg()},
		{Key: "slots-activity", Label: "Activity Slots", Value: workerSlotsValue(w.ActivitySlots), Color: theme.Fg()},
		{Key: "slots-local", Label: "Local Slots", Value: workerSlotsValue(w.LocalSlots), Color: theme.Fg()},
		{Key: "slots-nexus", Label: "Nexus Slots", Value: workerSlotsValue(w.NexusSlots), Color: theme.Fg()},
		{Key: "pollers-workflow", Label: "Workflow Pollers", Value: workerPollersValue(w.WorkflowPollers), Color: theme.Fg()},
		{Key: "pollers-sticky", Label: "Sticky Pollers", Value: workerPollersValue(w.StickyPollers), Color: theme.Fg()},
		{Key: "pollers-activity", Label: "Activity Pollers", Value: workerPollersValue(w.ActivityPollers), Color: theme.Fg()},
		{Key: "pollers-nexus", Label: "Nexus Pollers", Value: workerPollersValue(w.NexusPollers), Color: theme.Fg()},
		{Key: "cache", Label: "Workflow Cache", Value: formatStickyCache(w), Color: theme.Fg()},
	}
}

func workerSlotsValue(slots temporal.WorkerSlots) string {
	if slots == (temporal.WorkerSlots{}) {
		return "-"
	}
	value := fmt.Sprintf("%d / %d  processed %d  failed %d", slots.Used, slots.Available, slots.Processed, slots.Failed)
	if slots.Kind != "" {
		value += "  " + slots.Kind
	}
	return value
}

func workerPollersValue(pollers temporal.WorkerPollers) string {
	if pollers == (temporal.WorkerPollers{}) {
		return "-"
	}
	mode := "manual"
	if pollers.Autoscaling {
		mode = "autoscaling"
	}
	return fmt.Sprintf("%d  %s", pollers.Current, mode)
}

func formatStickyCache(w temporal.Worker) string {
	total := w.StickyCacheHit + w.StickyCacheMiss
	if total == 0 && w.StickyCacheSize == 0 {
		return "-"
	}
	rate := 0
	if total > 0 {
		rate = int((float64(w.StickyCacheHit) / float64(total)) * 100)
	}
	return fmt.Sprintf("size %d  hit %d%%", w.StickyCacheSize, rate)
}

func dashIfEmpty(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
