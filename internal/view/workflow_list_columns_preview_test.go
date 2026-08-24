package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func columnHeaders(wl *WorkflowList) []string {
	var out []string
	for col := 0; col < wl.table.GetColumnCount(); col++ {
		cell := wl.table.GetCell(0, col)
		if cell == nil {
			continue
		}
		out = append(out, cell.Text)
	}
	return out
}

func TestColumnEditorPreviewAndRevert(t *testing.T) {
	cfg := config.DefaultConfig()
	wl := NewWorkflowList(&App{config: cfg}, "default")
	wl.workflows = []temporal.Workflow{{ID: "wf-1", Type: "T", Status: "Running"}}
	wl.allWorkflows = wl.workflows
	wl.renderColumns()

	before := columnHeaders(wl)
	if len(before) < 2 {
		t.Fatalf("expected the default layout, got %v", before)
	}
	snapshot := wl.columnSnapshot()
	if snapshot != nil {
		t.Fatal("an unset layout should snapshot as nil, so a revert keeps it unset")
	}

	// A live edit lands in the list immediately, without being saved.
	wl.previewColumns([]config.WorkflowColumnConfig{
		{ID: config.WorkflowColumnStatus, Width: 12},
	})
	after := columnHeaders(wl)
	if len(after) != 1 {
		t.Fatalf("preview should have narrowed the list to one column, got %v", after)
	}
	if wl.table.RowCount() != 1 {
		t.Fatalf("rows should be re-rendered, got %d", wl.table.RowCount())
	}
	if cfg.WorkflowColumns == nil {
		t.Fatal("preview should have staged the layout in the running config")
	}

	// Cancelling puts the original layout back, still unset on disk.
	wl.restoreColumns(snapshot)
	if got := columnHeaders(wl); len(got) != len(before) {
		t.Fatalf("revert should restore the layout: got %v want %v", got, before)
	}
	if cfg.WorkflowColumns != nil {
		t.Fatal("revert should leave the layout unset, not write out the defaults")
	}
}

func TestColumnEditorPreviewKeepsSelection(t *testing.T) {
	wl := NewWorkflowList(&App{config: config.DefaultConfig()}, "default")
	wl.workflows = []temporal.Workflow{
		{ID: "wf-1", Type: "T", Status: "Running"},
		{ID: "wf-2", Type: "T", Status: "Running"},
		{ID: "wf-3", Type: "T", Status: "Running"},
	}
	wl.allWorkflows = wl.workflows
	wl.renderColumns()
	wl.table.SelectRow(2)

	wl.previewColumns([]config.WorkflowColumnConfig{
		{ID: config.WorkflowColumnWorkflowID, Width: 30},
		{ID: config.WorkflowColumnStatus, Width: 12},
	})
	if got := wl.table.SelectedRow(); got != 2 {
		t.Fatalf("editing columns should not move the cursor, got row %d", got)
	}
}

func TestColumnSnapshotRoundTripsAConfiguredLayout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WorkflowColumns = []config.WorkflowColumnConfig{
		{ID: config.WorkflowColumnWorkflowID, Width: 25},
		{ID: config.WorkflowColumnStatus, Width: 10},
	}
	wl := NewWorkflowList(&App{config: cfg}, "default")

	snapshot := wl.columnSnapshot()
	if len(snapshot) != 2 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	wl.previewColumns([]config.WorkflowColumnConfig{{ID: config.WorkflowColumnType, Width: 20}})
	if len(cfg.WorkflowColumns) != 1 {
		t.Fatalf("preview should replace the layout: %+v", cfg.WorkflowColumns)
	}
	wl.restoreColumns(snapshot)
	if len(cfg.WorkflowColumns) != 2 || cfg.WorkflowColumns[0].Width != 25 {
		t.Fatalf("revert should restore widths and order: %+v", cfg.WorkflowColumns)
	}
}
