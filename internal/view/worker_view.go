package view

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/rivo/tview"
)

type workerHostGroup struct {
	Host      string
	CPU       float32
	Memory    float32
	Resources bool
	Workers   []temporal.Worker
}

type workerRow struct {
	Host   string
	IsHost bool
	Worker temporal.Worker
}

type WorkerView struct {
	app          *App
	table        *components.Table
	preview      *tview.TextView
	previewPanel *components.Panel
	allWorkers   []temporal.Worker
	groups       []workerHostGroup
	rows         []workerRow
	collapsed    map[string]bool
	searchText   string
	loading      bool
}

func NewWorkerView(app *App) *WorkerView {
	wv := &WorkerView{
		app:       app,
		table:     components.NewTable(),
		preview:   tview.NewTextView(),
		collapsed: map[string]bool{},
	}
	wv.setup()
	return wv
}

func (wv *WorkerView) setup() {
	wv.table.SetHeaders(workerTableHeaders()...)
	wv.table.SetBorder(false)
	wv.table.SetBackgroundColor(theme.Bg())
	wv.table.ConfigureEmpty(theme.IconUsers, "No Workers", "No worker instances found in this namespace")
	wv.table.SetSelectionChangedFunc(func(row, col int) {
		wv.updatePreview()
	})

	wv.preview.SetDynamicColors(true)
	wv.preview.SetBackgroundColor(theme.Bg())
	wv.preview.SetTextColor(theme.Fg())
	wv.preview.SetWordWrap(true)
	wv.previewPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Worker", theme.IconUsers))
	wv.previewPanel.SetContent(wv.preview)
}

func workerTableHeaders() []string {
	return []string{"INSTANCE", "STATUS", "TASK QUEUE", "HEARTBEAT", "START", "BUILD ID", "PID", "CPU", "MEM"}
}

func (wv *WorkerView) RefreshTheme() {
	wv.table.SetBackgroundColor(theme.Bg())
	wv.preview.SetBackgroundColor(theme.Bg())
	wv.preview.SetTextColor(theme.Fg())
	wv.populateTable()
}

func (wv *WorkerView) applyFilter(query string) {
	wv.searchText = query
	wv.groups = groupWorkersByHost(filterWorkers(wv.allWorkers, query))
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

func loadWorkers(ctx context.Context, provider temporal.Provider, namespace string) ([]temporal.Worker, error) {
	listed, listErr := provider.ListWorkers(ctx, namespace)
	if listErr == nil && len(listed) > 0 {
		return listed, nil
	}

	names, err := provider.ListTaskQueueNames(ctx, namespace)
	if err != nil {
		if listErr != nil {
			return nil, listErr
		}
		return nil, err
	}
	var workers []temporal.Worker
	for _, name := range names {
		_, pollers, descErr := provider.DescribeTaskQueue(ctx, namespace, name)
		if descErr != nil {
			continue
		}
		workers = append(workers, temporal.WorkersFromPollers(name, pollers)...)
	}
	return workers, nil
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
	wv.table.AddRowWithColor(theme.Error(), "Error loading workers", err.Error(), "", "", "", "", "", "", "")
	wv.preview.SetText("")
}

func (wv *WorkerView) rebuildRows() {
	wv.rows = flattenWorkerRows(wv.groups, wv.collapsed)
}

func (wv *WorkerView) populateTable() {
	current := wv.selectedKey()
	wv.table.ClearRows()
	wv.table.SetHeaders(workerTableHeaders()...)
	now := time.Now()
	for _, row := range wv.rows {
		if row.IsHost {
			wv.addHostRow(row.Host)
			continue
		}
		wv.addInstanceRow(now, row.Worker)
	}
	if wv.table.RowCount() == 0 {
		wv.preview.SetText("")
		return
	}
	if idx := wv.rowIndexByKey(current); idx >= 0 {
		wv.table.SelectRow(idx)
	} else {
		wv.table.SelectRow(0)
	}
	wv.updatePreview()
}

func (wv *WorkerView) addHostRow(host string) {
	group := wv.hostGroup(host)
	chevron := theme.IconChevronD
	if wv.collapsed[host] {
		chevron = theme.IconChevronR
	}
	count := 0
	if group != nil {
		count = len(group.Workers)
	}
	label := fmt.Sprintf("%s %s %s (%d)", chevron, theme.IconServer, host, count)
	cpu, mem := "-", "-"
	if group != nil && group.Resources {
		cpu = formatWorkerPercent(group.CPU)
		mem = formatWorkerPercent(group.Memory)
	}
	wv.table.AddRow(label, "", "", "", "", "", "", cpu, mem)
}

func (wv *WorkerView) addInstanceRow(now time.Time, w temporal.Worker) {
	status := temporal.GetWorkerStatus(w.Status)
	cells := []string{
		workflowTreePrefix(1) + theme.IconUser + " " + instanceLabel(w),
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

func (wv *WorkerView) hostGroup(host string) *workerHostGroup {
	for i := range wv.groups {
		if wv.groups[i].Host == host {
			return &wv.groups[i]
		}
	}
	return nil
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

func (wv *WorkerView) toggleSelectedHost() bool {
	row, ok := wv.selectedRow()
	if !ok {
		return false
	}
	host := row.Host
	if host == "" {
		return false
	}
	wv.collapsed[host] = !wv.collapsed[host]
	wv.rebuildRows()
	wv.populateTable()
	if idx := wv.rowIndexByKey(workerHostKey(host)); idx >= 0 {
		wv.table.SelectRow(idx)
	}
	return true
}

func (wv *WorkerView) setHostCollapsed(collapsed bool) bool {
	row, ok := wv.selectedRow()
	if !ok || row.Host == "" {
		return false
	}
	if wv.collapsed[row.Host] == collapsed {
		return true
	}
	wv.collapsed[row.Host] = collapsed
	wv.rebuildRows()
	wv.populateTable()
	if idx := wv.rowIndexByKey(workerHostKey(row.Host)); idx >= 0 {
		wv.table.SelectRow(idx)
	}
	return true
}

func (wv *WorkerView) updatePreview() {
	row, ok := wv.selectedRow()
	if !ok {
		wv.preview.SetText("")
		return
	}
	if row.IsHost {
		if group := wv.hostGroup(row.Host); group != nil {
			wv.preview.SetText(formatWorkerHostPreview(*group))
			wv.preview.ScrollToBeginning()
			return
		}
	}
	wv.preview.SetText(formatWorkerInstancePreview(row.Worker))
	wv.preview.ScrollToBeginning()
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
	if wv.preview != nil {
		wv.preview.SetInputCapture(nil)
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

func groupWorkersByHost(workers []temporal.Worker) []workerHostGroup {
	index := map[string]int{}
	var groups []workerHostGroup
	for _, w := range workers {
		host := w.Host
		if host == "" {
			host = temporal.HostFromIdentity(w.Identity)
		}
		i, ok := index[host]
		if !ok {
			i = len(groups)
			index[host] = i
			groups = append(groups, workerHostGroup{Host: host})
		}
		groups[i].Workers = append(groups[i].Workers, w)
		if w.HasHostInfo {
			groups[i].Resources = true
			groups[i].CPU = w.CPU
			groups[i].Memory = w.Memory
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Host < groups[j].Host
	})
	for i := range groups {
		sort.Slice(groups[i].Workers, func(a, b int) bool {
			left, right := groups[i].Workers[a], groups[i].Workers[b]
			if left.Identity == right.Identity {
				return left.TaskQueue < right.TaskQueue
			}
			return left.Identity < right.Identity
		})
	}
	return groups
}

func flattenWorkerRows(groups []workerHostGroup, collapsed map[string]bool) []workerRow {
	var rows []workerRow
	for _, group := range groups {
		rows = append(rows, workerRow{Host: group.Host, IsHost: true})
		if collapsed[group.Host] {
			continue
		}
		for _, w := range group.Workers {
			rows = append(rows, workerRow{Host: group.Host, Worker: w})
		}
	}
	return rows
}

func workerRowKey(row workerRow) string {
	if row.IsHost {
		return workerHostKey(row.Host)
	}
	return "inst:" + row.Worker.InstanceKey + "|" + row.Worker.Identity + "|" + row.Worker.TaskQueue
}

func workerHostKey(host string) string {
	return "host:" + host
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

func formatWorkerHostPreview(group workerHostGroup) string {
	now := time.Now()
	cpu, mem := "-", "-"
	if group.Resources {
		cpu = formatWorkerPercent(group.CPU)
		mem = formatWorkerPercent(group.Memory)
	}
	lines := []string{
		fmt.Sprintf("[%s::b]Host[-:-:-]", theme.TagAccent()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), group.Host),
		"",
		fmt.Sprintf("[%s]Instances[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%d[-]", theme.TagFg(), len(group.Workers)),
		"",
		fmt.Sprintf("[%s]CPU[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), cpu),
		"",
		fmt.Sprintf("[%s]Memory[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), mem),
		"",
		fmt.Sprintf("[%s]Workers[-]", theme.TagFgDim()),
	}
	if len(group.Workers) == 0 {
		lines = append(lines, fmt.Sprintf("[%s]No instances[-]", theme.TagFgDim()))
	}
	for _, w := range group.Workers {
		status := w.Status
		if status == "" {
			status = "-"
		}
		lines = append(lines, fmt.Sprintf("[%s]%s[-]  [%s]%s[-]  [%s]%s[-]  [%s]%s[-]",
			theme.TagFg(), instanceLabel(w),
			temporal.GetWorkerStatus(w.Status).ColorTag(), status,
			theme.TagFg(), dashIfEmpty(w.TaskQueue),
			theme.TagFgDim(), formatWorkerTime(now, w.LastHeartbeat),
		))
	}
	return strings.Join(lines, "\n")
}

func formatWorkerInstancePreview(w temporal.Worker) string {
	now := time.Now()
	status := w.Status
	if status == "" {
		status = "Unknown"
	}
	sdk := strings.TrimSpace(w.SDKName + " " + w.SDKVersion)
	lines := []string{
		fmt.Sprintf("[%s::b]Instance[-:-:-]", theme.TagAccent()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(firstNonEmpty(w.InstanceKey, w.Identity))),
		"",
		fmt.Sprintf("[%s]Status[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", temporal.GetWorkerStatus(w.Status).ColorTag(), status),
		"",
		fmt.Sprintf("[%s]Identity[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(w.Identity)),
		"",
		fmt.Sprintf("[%s]Task Queue[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(w.TaskQueue)),
		"",
		fmt.Sprintf("[%s]Types[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(strings.Join(w.Types, ", "))),
		"",
		fmt.Sprintf("[%s]Host[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(w.Host)),
		"",
		fmt.Sprintf("[%s]Process ID[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(w.ProcessID)),
		"",
		fmt.Sprintf("[%s]Build ID[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(w.BuildID)),
		"",
		fmt.Sprintf("[%s]Deployment[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(w.Deployment)),
		"",
		fmt.Sprintf("[%s]SDK[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), dashIfEmpty(sdk)),
		"",
		fmt.Sprintf("[%s]Started[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), formatWorkerTime(now, w.StartTime)),
		"",
		fmt.Sprintf("[%s]Last Heartbeat[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), formatWorkerTime(now, w.LastHeartbeat)),
		"",
		fmt.Sprintf("[%s]CPU[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), formatWorkerResource(w)),
		"",
		fmt.Sprintf("[%s]Memory[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), formatWorkerMemory(w)),
		"",
		fmt.Sprintf("[%s]Task Slots[-]", theme.TagFgDim()),
		formatSlotLine("Workflow", w.WorkflowSlots),
		formatSlotLine("Activity", w.ActivitySlots),
		formatSlotLine("Local Activity", w.LocalSlots),
		formatSlotLine("Nexus", w.NexusSlots),
		"",
		fmt.Sprintf("[%s]Pollers[-]", theme.TagFgDim()),
		formatPollerLine("Workflow", w.WorkflowPollers),
		formatPollerLine("Sticky", w.StickyPollers),
		formatPollerLine("Activity", w.ActivityPollers),
		formatPollerLine("Nexus", w.NexusPollers),
		"",
		fmt.Sprintf("[%s]Workflow Cache[-]", theme.TagFgDim()),
		fmt.Sprintf("[%s]%s[-]", theme.TagFg(), formatStickyCache(w)),
	}
	return strings.Join(lines, "\n")
}

func formatSlotLine(label string, slots temporal.WorkerSlots) string {
	if slots == (temporal.WorkerSlots{}) {
		return fmt.Sprintf("[%s]%s[-]  [%s]-[-]", theme.TagFg(), label, theme.TagFgDim())
	}
	kind := ""
	if slots.Kind != "" {
		kind = "  " + slots.Kind
	}
	return fmt.Sprintf("[%s]%s[-]  [%s]%d / %d[-]  [%s]processed %d  failed %d%s[-]",
		theme.TagFg(), label,
		theme.TagFg(), slots.Used, slots.Available,
		theme.TagFgDim(), slots.Processed, slots.Failed, kind,
	)
}

func formatPollerLine(label string, pollers temporal.WorkerPollers) string {
	if pollers == (temporal.WorkerPollers{}) {
		return fmt.Sprintf("[%s]%s[-]  [%s]-[-]", theme.TagFg(), label, theme.TagFgDim())
	}
	mode := "manual"
	if pollers.Autoscaling {
		mode = "autoscaling"
	}
	return fmt.Sprintf("[%s]%s[-]  [%s]%d[-]  [%s]%s[-]",
		theme.TagFg(), label, theme.TagFg(), pollers.Current, theme.TagFgDim(), mode)
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
