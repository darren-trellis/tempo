package view

import (
	"fmt"
	"strconv"
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

func (c workflowColumn) cell(now time.Time, w temporal.Workflow, depth int) components.TableCell {
	text, status := workflowColumnValue(c.id, now, w)
	if c.id == config.WorkflowColumnWorkflowID && depth > 0 {
		text = workflowTreePrefix(depth) + text
	}
	return components.TableCell{
		Text:       fitWidth(text, c.width),
		Status:     status,
		Expansion:  0,
		MaxWidth:   c.width,
		Selectable: true,
	}
}

func workflowColumnValue(id string, now time.Time, w temporal.Workflow) (string, *theme.Status) {
	switch id {
	case config.WorkflowColumnWorkflowID:
		return w.ID, nil
	case config.WorkflowColumnParentID:
		if w.ParentID != nil {
			return *w.ParentID, nil
		}
		return "", nil
	case config.WorkflowColumnStatus:
		status := temporal.GetWorkflowStatus(w.Status)
		text := w.Status
		if icon := status.Icon(); icon != "" {
			text = icon + " " + w.Status
		}
		return text, status
	case config.WorkflowColumnType:
		return w.Type, nil
	case config.WorkflowColumnStarted:
		return formatRelativeTime(now, w.StartTime), nil
	case config.WorkflowColumnEnded:
		return workflowEndTime(now, w), nil
	case config.WorkflowColumnDuration:
		return workflowDuration(now, w), nil
	case config.WorkflowColumnTaskQueue:
		return w.TaskQueue, nil
	case config.WorkflowColumnRunID:
		return w.RunID, nil
	default:
		return "", nil
	}
}

func applyWorkflowColumnHeaders(table *components.Table, cols []workflowColumn) {
	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}
	table.SetHeaders(headers...)
	// SetHeaders only writes the columns it is given, so a narrower layout would
	// leave the extra columns of the previous one on screen.
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

func fitWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	if len(s) > width {
		return truncateIfNeeded(s, width)
	}
	if len(s) < width {
		return s + padSpaces(width-len(s))
	}
	return s
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

func (wl *WorkflowList) showColumnEditor() {
	items := wl.columnEditorItems()
	table := components.NewTable()
	table.SetBorder(false)

	original := wl.columnSnapshot()

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
		wl.previewColumns(editedColumns())
	}

	restore := func() {
		wl.restoreColumns(original)
	}

	var refresh func()
	refresh = func() {
		row := table.SelectedRow()
		table.ClearRows()
		table.SetHeaders("COLUMN", "WIDTH", "VISIBLE")
		for _, item := range items {
			header, _ := workflowColumnHeader(item.id)
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
		items[row].width = config.ClampWorkflowColumnWidth(items[row].width + delta)
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
		if cfg := wl.app.Config(); cfg != nil {
			cfg.SetWorkflowColumns(editedColumns())
			if err := cfg.Save(); err != nil {
				saving = false
				wl.app.ToastError("Failed to save columns: " + err.Error())
				return
			}
		}
		wl.closeModal()
		wl.populateTable()
		wl.app.ToastSuccess("Saved workflow columns")
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
			items = defaultColumnEditorItems()
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
		Title:  fmt.Sprintf("%s Workflow Columns", theme.IconWorkflow),
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
	modal.SetOnCancel(func() {
		restore()
		wl.closeModal()
	})

	wl.app.PushModal(modal)
	if wl.app.JigApp().Menu() != nil {
		wl.app.JigApp().Menu().SetHints(hints)
	}
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
	return items
}

func defaultColumnEditorItems() []columnEditorItem {
	items := make([]columnEditorItem, 0, len(config.DefaultWorkflowColumns()))
	for _, col := range config.DefaultWorkflowColumns() {
		items = append(items, columnEditorItem{id: col.ID, width: col.Width})
	}
	return items
}
