package view

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

type workflowColumn struct {
	id     string
	header string
	width  int
}

func (wl *WorkflowList) columnLayout() []workflowColumn {
	var raw []config.WorkflowColumnConfig
	if cfg := wl.app.Config(); cfg != nil {
		raw = cfg.WorkflowColumnLayout()
	} else {
		raw = config.DefaultWorkflowColumns()
	}

	cols := make([]workflowColumn, 0, len(raw))
	for _, col := range raw {
		header, ok := workflowColumnHeader(col.ID)
		if !ok {
			continue
		}
		cols = append(cols, workflowColumn{
			id:     col.ID,
			header: header,
			width:  col.Width,
		})
	}
	if len(cols) == 0 {
		for _, col := range config.DefaultWorkflowColumns() {
			header, _ := workflowColumnHeader(col.ID)
			cols = append(cols, workflowColumn{id: col.ID, header: header, width: col.Width})
		}
	}
	return cols
}

func workflowColumnHeader(id string) (string, bool) {
	if name, ok := config.SearchAttributeColumnName(id); ok {
		return name, true
	}
	switch id {
	case config.WorkflowColumnWorkflowID:
		return "WORKFLOW ID", true
	case config.WorkflowColumnParentID:
		return "PARENT ID", true
	case config.WorkflowColumnStatus:
		return "STATUS", true
	case config.WorkflowColumnType:
		return "TYPE", true
	case config.WorkflowColumnStarted:
		return "STARTED", true
	case config.WorkflowColumnEnded:
		return "ENDED", true
	case config.WorkflowColumnDuration:
		return "DURATION", true
	case config.WorkflowColumnTaskQueue:
		return "TASK QUEUE", true
	case config.WorkflowColumnRunID:
		return "RUN ID", true
	default:
		return "", false
	}
}

func (c workflowColumn) cell(now time.Time, w temporal.Workflow, prefix, timeFmt string) components.TableCell {
	var text string
	var status *theme.Status
	if c.id == config.WorkflowColumnStatus {
		text, status = workflowStatusText(w, c.width)
	} else {
		text, status = workflowColumnValue(c.id, now, w, timeFmt)
		if c.id == config.WorkflowColumnWorkflowID && prefix != "" {
			text = colorizeWorkflowTreePrefix(fitWidth(prefix+text, c.width), prefix)
		} else {
			text = fitWidth(text, c.width)
		}
	}
	return components.TableCell{
		Text:       text,
		Status:     status,
		Expansion:  0,
		MaxWidth:   c.width,
		Selectable: true,
	}
}

func workflowStatusText(w temporal.Workflow, width int) (string, *theme.Status) {
	label, status := temporal.WorkflowDisplayStatus(w)
	if w.HasTaskFailure() {
		label = "Unhandled"
	}
	return statusColumnText(label, status, width), status
}

func statusColumnText(label string, status *theme.Status, width int) string {
	icon := statusIconForWidth(status, width)
	if width == 1 && icon != "" {
		return icon
	}
	full := label
	if icon != "" {
		full = icon + " " + label
	}
	if width <= 0 {
		return full
	}
	runes := []rune(full)
	if len(runes) <= width {
		return fitWidth(full, width)
	}
	iconLen := len([]rune(icon))
	if icon != "" && width < iconLen+2 {
		return icon
	}
	if icon != "" {
		return icon + " " + fitWidth(label, width-iconLen-1)
	}
	return fitWidth(label, width)
}

func statusIconForWidth(status *theme.Status, width int) string {
	if status == nil {
		return ""
	}
	if width == 1 {
		return compactStatusIcon(status)
	}
	return status.Icon()
}

func compactStatusIcon(status *theme.Status) string {
	if status == nil {
		return ""
	}
	switch status {
	case temporal.StatusRunning:
		return theme.IconPlay
	case temporal.StatusCompleted:
		return theme.IconCheck
	case temporal.StatusFailed:
		return theme.IconFailed
	case temporal.StatusCanceled:
		return theme.IconCanceled
	case temporal.StatusTerminated:
		return theme.IconStop
	case temporal.StatusTimedOut:
		return theme.IconClock
	case temporal.StatusUnhandledFailure:
		return theme.IconWarning
	case temporal.StatusScheduled, temporal.StatusUnknown:
		return theme.IconDot
	default:
		if icon := status.Icon(); icon != "" {
			return icon
		}
		return theme.IconDot
	}
}

func workflowColumnValue(id string, now time.Time, w temporal.Workflow, timeFmt string) (string, *theme.Status) {
	switch id {
	case config.WorkflowColumnWorkflowID:
		return w.ID, nil
	case config.WorkflowColumnParentID:
		if w.ParentID != nil {
			return *w.ParentID, nil
		}
		return "", nil
	case config.WorkflowColumnStatus:
		label, status := temporal.WorkflowDisplayStatus(w)
		if w.HasTaskFailure() {
			label = "Unhandled"
		}
		text := label
		if icon := status.Icon(); icon != "" {
			text = icon + " " + label
		}
		return text, status
	case config.WorkflowColumnType:
		return w.Type, nil
	case config.WorkflowColumnStarted:
		return formatDisplayTime(now, w.StartTime, timeFmt), nil
	case config.WorkflowColumnEnded:
		return workflowEndTime(now, w, timeFmt), nil
	case config.WorkflowColumnDuration:
		return workflowDuration(now, w), nil
	case config.WorkflowColumnTaskQueue:
		return w.TaskQueue, nil
	case config.WorkflowColumnRunID:
		return w.RunID, nil
	default:
		if name, ok := config.SearchAttributeColumnName(id); ok {
			return formatSearchAttributeColumn(w.SearchAttributes[name], now, timeFmt), nil
		}
		return "", nil
	}
}

func formatSearchAttributeColumn(raw string, now time.Time, timeFmt string) string {
	if raw == "" {
		return ""
	}
	if when, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return formatDisplayTime(now, when, timeFmt)
	}
	if when, err := time.Parse(time.RFC3339, raw); err == nil {
		return formatDisplayTime(now, when, timeFmt)
	}
	return raw
}

func applyWorkflowColumnHeaders(table *components.Table, cols []workflowColumn) {
	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}
	table.SetHeaders(headers...)
	for col := table.GetColumnCount() - 1; col >= len(cols); col-- {
		table.RemoveColumn(col)
	}
	for i, col := range cols {
		cell := table.GetCell(0, i)
		if cell == nil {
			continue
		}
		cell.SetText(fitWidth(col.header, col.width))
		cell.SetMaxWidth(col.width)
		cell.SetExpansion(0)
	}
}

func setTableHeaders(table *components.Table, headers ...string) {
	if table == nil {
		return
	}
	table.SetHeaders(headers...)
	for col := table.GetColumnCount() - 1; col >= len(headers); col-- {
		table.RemoveColumn(col)
	}
}

func fitWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	n := len([]rune(s))
	if n > width {
		return truncateIfNeeded(s, width)
	}
	if n < width {
		return s + padSpaces(width-n)
	}
	return s
}

func (wl *WorkflowList) styledWorkflowCells(now time.Time, w temporal.Workflow, rowIdx int) []components.TableCell {
	cols := wl.columnLayout()
	timeFmt := wl.workflowTimeFormat()
	cells := make([]components.TableCell, len(cols))
	for j, col := range cols {
		cells[j] = col.cell(now, w, wl.workflowRowPrefix(rowIdx), timeFmt)
	}
	if !wl.shouldColorCodeWorkflows() {
		return cells
	}
	_, status := temporal.WorkflowDisplayStatus(w)
	for i := range cells {
		cells[i].Color = status.Color()
		cells[i].Status = status
	}
	return cells
}

func (wl *WorkflowList) shouldColorCodeWorkflows() bool {
	return wl != nil && wl.app != nil && wl.app.Config() != nil && wl.app.Config().ShouldColorCodeWorkflows()
}

func (wl *WorkflowList) shouldColorCodeActivities() bool {
	return wl != nil && wl.app != nil && wl.app.Config() != nil && wl.app.Config().ShouldColorCodeActivities()
}

func (wl *WorkflowList) workflowTimeFormat() string {
	if wl != nil && wl.app != nil && wl.app.Config() != nil {
		return wl.app.Config().ResolvedWorkflowTimeFormat()
	}
	return config.TimeFormatRelative
}

func (wl *WorkflowList) activityTimeFormat() string {
	if wl != nil && wl.app != nil && wl.app.Config() != nil {
		return wl.app.Config().ResolvedActivityTimeFormat()
	}
	return config.TimeFormatRelative
}

func (wl *WorkflowList) activityColumnLayout() []workflowColumn {
	var raw []config.WorkflowColumnConfig
	if cfg := wl.app.Config(); cfg != nil {
		raw = cfg.ActivityColumnLayout()
	} else {
		raw = config.DefaultActivityColumns()
	}

	cols := make([]workflowColumn, 0, len(raw))
	for _, col := range raw {
		header, ok := activityColumnHeader(col.ID)
		if !ok {
			continue
		}
		cols = append(cols, workflowColumn{
			id:     col.ID,
			header: header,
			width:  col.Width,
		})
	}
	if len(cols) == 0 {
		for _, col := range config.DefaultActivityColumns() {
			header, _ := activityColumnHeader(col.ID)
			cols = append(cols, workflowColumn{id: col.ID, header: header, width: col.Width})
		}
	}
	return cols
}

func activityColumnHeader(id string) (string, bool) {
	switch id {
	case config.ActivityColumnStatus:
		return "STATUS", true
	case config.ActivityColumnName:
		return "NAME", true
	case config.ActivityColumnStarted:
		return "STARTED", true
	case config.ActivityColumnEnded:
		return "ENDED", true
	case config.ActivityColumnDuration:
		return "DURATION", true
	default:
		return "", false
	}
}

func activityColumnValue(id string, now time.Time, a previewActivity, timeFmt string) string {
	switch id {
	case config.ActivityColumnName:
		if a.Type != "" {
			return a.Type
		}
		return "Activity"
	case config.ActivityColumnStarted:
		return formatDisplayTime(now, a.StartTime, timeFmt)
	case config.ActivityColumnEnded:
		if a.EndTime != nil {
			return formatDisplayTime(now, *a.EndTime, timeFmt)
		}
		return "-"
	case config.ActivityColumnDuration:
		return a.durationAt(now)
	default:
		return ""
	}
}

func (c workflowColumn) activityCell(now time.Time, a previewActivity, timeFmt string) components.TableCell {
	var text string
	var status *theme.Status
	if c.id == config.ActivityColumnStatus {
		status = temporal.GetActivityStatus(a.Status)
		text = statusColumnText(a.Status, status, c.width)
	} else {
		text = fitWidth(activityColumnValue(c.id, now, a, timeFmt), c.width)
	}
	return components.TableCell{
		Text:       text,
		Status:     status,
		Expansion:  0,
		MaxWidth:   c.width,
		Selectable: true,
	}
}

func (wl *WorkflowList) styledActivityCells(now time.Time, a previewActivity) []components.TableCell {
	cols := wl.activityColumnLayout()
	timeFmt := wl.activityTimeFormat()
	cells := make([]components.TableCell, len(cols))
	for j, col := range cols {
		cells[j] = col.activityCell(now, a, timeFmt)
	}
	if !wl.shouldColorCodeActivities() {
		return cells
	}
	status := temporal.GetActivityStatus(a.Status)
	for i := range cells {
		cells[i].Color = status.Color()
		cells[i].Status = status
	}
	return cells
}

func (wl *WorkflowList) applyActivityTableHeaders() {
	if wl.eventTable == nil {
		return
	}
	applyWorkflowColumnHeaders(wl.eventTable, wl.activityColumnLayout())
}

func padSpaces(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = ' '
	}
	return string(buf)
}

type columnEditorItem struct {
	id     string
	width  int
	hidden bool
}

type tableColumnEditor struct {
	title    string
	success  string
	header   func(string) (string, bool)
	items    func() []columnEditorItem
	defaults func() []columnEditorItem
	snapshot func() []config.WorkflowColumnConfig
	preview  func([]config.WorkflowColumnConfig)
	restore  func([]config.WorkflowColumnConfig)
	commit   func([]config.WorkflowColumnConfig) error
	redraw   func()
}

func (wl *WorkflowList) showColumnEditor() {
	wl.showTableColumnEditor(tableColumnEditor{
		title:    fmt.Sprintf("%s Workflow Columns", theme.IconWorkflow),
		success:  "Saved workflow columns",
		header:   workflowColumnHeader,
		items:    wl.columnEditorItems,
		defaults: wl.defaultWorkflowColumnEditorItems,
		snapshot: wl.columnSnapshot,
		preview:  wl.previewColumns,
		restore:  wl.restoreColumns,
		commit: func(cols []config.WorkflowColumnConfig) error {
			if cfg := wl.app.Config(); cfg != nil {
				cfg.SetWorkflowColumns(cols)
				return wl.app.SaveConfig()
			}
			return nil
		},
		redraw: wl.populateTable,
	})
}

func (wl *WorkflowList) showActivityColumnEditor() {
	wl.showTableColumnEditor(tableColumnEditor{
		title:    fmt.Sprintf("%s Activity Columns", theme.IconActivity),
		success:  "Saved activity columns",
		header:   activityColumnHeader,
		items:    wl.activityColumnEditorItems,
		defaults: defaultActivityColumnEditorItems,
		snapshot: wl.activityColumnSnapshot,
		preview:  wl.previewActivityColumns,
		restore:  wl.restoreActivityColumns,
		commit: func(cols []config.WorkflowColumnConfig) error {
			if cfg := wl.app.Config(); cfg != nil {
				cfg.SetActivityColumns(cols)
				return wl.app.SaveConfig()
			}
			return nil
		},
		redraw: wl.renderActivityColumns,
	})
}

func (wl *WorkflowList) showTableColumnEditor(spec tableColumnEditor) {
	items := spec.items()
	table := components.NewTable()
	table.SetBorder(false)

	original := spec.snapshot()

	editedColumns := func() []config.WorkflowColumnConfig {
		var cols []config.WorkflowColumnConfig
		for _, item := range items {
			if item.hidden {
				continue
			}
			cols = append(cols, config.WorkflowColumnConfig{ID: item.id, Width: item.width})
		}
		return cols
	}

	preview := func() {
		spec.preview(editedColumns())
	}

	committed := false

	restore := func() {
		if committed {
			return
		}
		spec.restore(original)
	}

	var refresh func()
	refresh = func() {
		row := table.SelectedRow()
		table.ClearRows()
		table.SetHeaders("COLUMN", "WIDTH", "VISIBLE")
		for _, item := range items {
			header, _ := spec.header(item.id)
			visible := "yes"
			if item.hidden {
				visible = "no"
			}
			if item.hidden {
				table.AddRowWithColor(theme.FgDim(), header, strconv.Itoa(item.width), visible)
			} else {
				table.AddRow(header, strconv.Itoa(item.width), visible)
			}
		}
		if row < 0 {
			row = 0
		}
		if row >= len(items) {
			row = len(items) - 1
		}
		if row >= 0 {
			table.SelectRow(row)
		}
	}

	visibleCount := func() int {
		n := 0
		for _, item := range items {
			if !item.hidden {
				n++
			}
		}
		return n
	}

	move := func(delta int) {
		row := table.SelectedRow()
		next := row + delta
		if row < 0 || next < 0 || next >= len(items) {
			return
		}
		items[row], items[next] = items[next], items[row]
		refresh()
		preview()
		table.SelectRow(next)
	}

	resize := func(delta int) {
		row := table.SelectedRow()
		if row < 0 || row >= len(items) {
			return
		}
		items[row].width = config.ClampWorkflowColumnWidth(items[row].id, items[row].width+delta)
		refresh()
		preview()
		table.SelectRow(row)
	}

	toggle := func() {
		row := table.SelectedRow()
		if row < 0 || row >= len(items) {
			return
		}
		if !items[row].hidden && visibleCount() == 1 {
			wl.app.ToastWarning("Keep at least one column visible")
			return
		}
		items[row].hidden = !items[row].hidden
		refresh()
		preview()
		table.SelectRow(row)
	}

	var saving bool
	save := func() {
		if saving {
			return
		}
		saving = true
		if err := spec.commit(editedColumns()); err != nil {
			saving = false
			wl.app.ToastError("Failed to save columns: " + err.Error())
			return
		}
		committed = true
		wl.closeModal()
		spec.redraw()
		wl.app.ToastSuccess(spec.success)
	}

	bindings := input.NewKeyBindings().
		OnRune('J', func(e *tcell.EventKey) bool {
			move(1)
			return true
		}).
		OnRune('K', func(e *tcell.EventKey) bool {
			move(-1)
			return true
		}).
		OnRune('+', func(e *tcell.EventKey) bool {
			resize(1)
			return true
		}).
		OnRune('=', func(e *tcell.EventKey) bool {
			resize(1)
			return true
		}).
		OnRune('-', func(e *tcell.EventKey) bool {
			resize(-1)
			return true
		}).
		OnRune('h', func(e *tcell.EventKey) bool {
			resize(-1)
			return true
		}).
		OnRune('l', func(e *tcell.EventKey) bool {
			resize(1)
			return true
		}).
		OnRune(' ', func(e *tcell.EventKey) bool {
			toggle()
			return true
		}).
		OnRune('r', func(e *tcell.EventKey) bool {
			items = spec.defaults()
			refresh()
			preview()
			table.SelectRow(0)
			return true
		})

	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			save()
			return nil
		}
		if event.Key() == tcell.KeyEscape {
			restore()
			wl.closeModal()
			return nil
		}
		if event.Key() == tcell.KeyLeft {
			resize(-1)
			return nil
		}
		if event.Key() == tcell.KeyRight {
			resize(1)
			return nil
		}
		if bindings.Handle(event) {
			return nil
		}
		return event
	})

	refresh()

	wl.keepDataOnStart = true
	modal := newOverlayModal(components.ModalConfig{
		Title:  spec.title,
		Width:  56,
		Height: 20,
	}, wl)
	modal.SetContent(table)
	hints := []components.KeyHint{
		{Key: "J/K", Description: "Reorder"},
		{Key: "+/-", Description: "Width"},
		{Key: "space", Description: "Hide"},
		{Key: "r", Description: "Reset"},
		{Key: "enter", Description: "Save"},
		{Key: "esc", Description: "Cancel"},
	}
	modal.SetHints(hints)
	modal.SetOnSubmit(save)
	modal.SetOnDismiss(func() bool {
		restore()
		return true
	})
	modal.SetOnCancel(func() {
		restore()
		wl.closeModal()
	})

	wl.app.PushModal(modal)
	wl.app.JigApp().SetFocus(table)
}

// columnSnapshot copies the stored column layout, nil included, so a restore can
// put back "unset means defaults" rather than writing the defaults out.
func (wl *WorkflowList) columnSnapshot() []config.WorkflowColumnConfig {
	cfg := wl.app.Config()
	if cfg == nil || cfg.WorkflowColumns == nil {
		return nil
	}
	return append([]config.WorkflowColumnConfig(nil), cfg.WorkflowColumns...)
}

// previewColumns applies a layout to the list right away without saving it, so
// the editor's changes are visible behind the modal.
func (wl *WorkflowList) previewColumns(cols []config.WorkflowColumnConfig) {
	cfg := wl.app.Config()
	if cfg == nil {
		return
	}
	cfg.SetWorkflowColumns(cols)
	wl.renderColumns()
}

// restoreColumns puts a snapshot back, undoing any preview.
func (wl *WorkflowList) restoreColumns(snapshot []config.WorkflowColumnConfig) {
	if cfg := wl.app.Config(); cfg != nil {
		cfg.WorkflowColumns = snapshot
	}
	wl.renderColumns()
}

func (wl *WorkflowList) columnEditorItems() []columnEditorItem {
	visible := make(map[string]config.WorkflowColumnConfig)
	order := make([]string, 0)
	if cfg := wl.app.Config(); cfg != nil {
		for _, col := range cfg.WorkflowColumnLayout() {
			visible[col.ID] = col
			order = append(order, col.ID)
		}
	} else {
		for _, col := range config.DefaultWorkflowColumns() {
			visible[col.ID] = col
			order = append(order, col.ID)
		}
	}

	items := make([]columnEditorItem, 0, len(config.KnownWorkflowColumnIDs()))
	for _, id := range order {
		col := visible[id]
		items = append(items, columnEditorItem{id: col.ID, width: col.Width})
	}
	for _, id := range config.KnownWorkflowColumnIDs() {
		if _, ok := visible[id]; ok {
			continue
		}
		items = append(items, columnEditorItem{
			id:     id,
			width:  config.DefaultWorkflowColumnWidth(id),
			hidden: true,
		})
	}
	items = append(items, wl.hiddenSearchAttributeColumns(visible)...)
	return items
}

func (wl *WorkflowList) hiddenSearchAttributeColumns(visible map[string]config.WorkflowColumnConfig) []columnEditorItem {
	var items []columnEditorItem
	for _, name := range wl.customSearchAttributeNames() {
		id := config.SearchAttributeColumnID(name)
		if id == "" {
			continue
		}
		if _, ok := visible[id]; ok {
			continue
		}
		items = append(items, columnEditorItem{
			id:     id,
			width:  config.DefaultWorkflowColumnWidth(id),
			hidden: true,
		})
	}
	return items
}

func (wl *WorkflowList) customSearchAttributeNames() []string {
	if wl == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var names []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if wl.app != nil {
		ns := wl.namespace
		attrs := wl.app.catalog.getAttrs(ns)
		if len(attrs) == 0 {
			if alt := wl.app.catalogNamespace(); alt != "" && alt != ns {
				attrs = wl.app.catalog.getAttrs(alt)
			}
		}
		for _, attr := range attrs {
			add(attr.Name)
		}
	}
	workflows := wl.allWorkflows
	if len(workflows) == 0 {
		workflows = wl.workflows
	}
	for _, w := range workflows {
		for name := range w.SearchAttributes {
			add(name)
		}
	}
	sort.Strings(names)
	return names
}

func (wl *WorkflowList) defaultWorkflowColumnEditorItems() []columnEditorItem {
	items := defaultColumnEditorItems()
	items = append(items, wl.hiddenSearchAttributeColumns(nil)...)
	return items
}

func defaultColumnEditorItems() []columnEditorItem {
	items := make([]columnEditorItem, 0, len(config.DefaultWorkflowColumns()))
	for _, col := range config.DefaultWorkflowColumns() {
		items = append(items, columnEditorItem{id: col.ID, width: col.Width})
	}
	return items
}

func (wl *WorkflowList) activityColumnSnapshot() []config.WorkflowColumnConfig {
	cfg := wl.app.Config()
	if cfg == nil || cfg.ActivityColumns == nil {
		return nil
	}
	return append([]config.WorkflowColumnConfig(nil), cfg.ActivityColumns...)
}

func (wl *WorkflowList) previewActivityColumns(cols []config.WorkflowColumnConfig) {
	cfg := wl.app.Config()
	if cfg == nil {
		return
	}
	cfg.SetActivityColumns(cols)
	wl.renderActivityColumns()
}

func (wl *WorkflowList) restoreActivityColumns(snapshot []config.WorkflowColumnConfig) {
	if cfg := wl.app.Config(); cfg != nil {
		cfg.ActivityColumns = snapshot
	}
	wl.renderActivityColumns()
}

func (wl *WorkflowList) renderActivityColumns() {
	if wl.previewKind != previewActivities || wl.eventTable == nil {
		return
	}
	if w, ok := wl.currentPreviewWorkflow(); ok && len(wl.previewActivities) > 0 {
		wl.renderPreviewActivities(w)
		return
	}
	wl.applyActivityTableHeaders()
}

func (wl *WorkflowList) activityColumnEditorItems() []columnEditorItem {
	visible := make(map[string]config.WorkflowColumnConfig)
	order := make([]string, 0)
	if cfg := wl.app.Config(); cfg != nil {
		for _, col := range cfg.ActivityColumnLayout() {
			visible[col.ID] = col
			order = append(order, col.ID)
		}
	} else {
		for _, col := range config.DefaultActivityColumns() {
			visible[col.ID] = col
			order = append(order, col.ID)
		}
	}

	items := make([]columnEditorItem, 0, len(config.KnownActivityColumnIDs()))
	for _, id := range order {
		col := visible[id]
		items = append(items, columnEditorItem{id: col.ID, width: col.Width})
	}
	for _, id := range config.KnownActivityColumnIDs() {
		if _, ok := visible[id]; ok {
			continue
		}
		items = append(items, columnEditorItem{
			id:     id,
			width:  config.DefaultActivityColumnWidth(id),
			hidden: true,
		})
	}
	return items
}

func defaultActivityColumnEditorItems() []columnEditorItem {
	items := make([]columnEditorItem, 0, len(config.DefaultActivityColumns()))
	for _, col := range config.DefaultActivityColumns() {
		items = append(items, columnEditorItem{id: col.ID, width: col.Width})
	}
	return items
}
