package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestWorkflowColumnParentID(t *testing.T) {
	parent := "order-processing-abc123"
	got, _ := workflowColumnValue(config.WorkflowColumnParentID, time.Now(), temporal.Workflow{
		ID:       "payment-xyz789",
		ParentID: &parent,
	})
	if got != parent {
		t.Fatalf("got %q", got)
	}

	empty, _ := workflowColumnValue(config.WorkflowColumnParentID, time.Now(), temporal.Workflow{ID: "solo"})
	if empty != "" {
		t.Fatalf("expected empty parent, got %q", empty)
	}

	header, ok := workflowColumnHeader(config.WorkflowColumnParentID)
	if !ok || header != "PARENT ID" {
		t.Fatalf("header=%q ok=%v", header, ok)
	}
}

func TestDefaultColumnLayoutIncludesParent(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflowTreeMode = false
	found := false
	for _, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnParentID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default workflow columns should include parent id")
	}
}

func TestWorkflowColumnMarksUnhandledFailure(t *testing.T) {
	text, status := workflowColumnValue(config.WorkflowColumnStatus, time.Now(), temporal.Workflow{
		Status:      "Running",
		TaskFailure: true,
	})
	if status != temporal.StatusUnhandledFailure {
		t.Fatalf("status=%v", status)
	}
	if !strings.Contains(text, temporal.StatusUnhandledFailure.Icon()) || !strings.Contains(text, "Unhandled") {
		t.Fatalf("text=%q", text)
	}

	running, runningStatus := workflowColumnValue(config.WorkflowColumnStatus, time.Now(), temporal.Workflow{
		Status: "Running",
	})
	if runningStatus != temporal.StatusRunning {
		t.Fatalf("running status=%v", runningStatus)
	}
	if running == text {
		t.Fatalf("running and unhandled should differ: %q", running)
	}
}

func TestMergeWorkflowRefreshesUnhandledStatus(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflows = []temporal.Workflow{{ID: "wf-1", RunID: "run-1", Type: "T", Status: "Running"}}
	wl.allWorkflows = wl.workflows
	wl.populateTable()

	wl.mergeWorkflow(temporal.Workflow{ID: "wf-1", RunID: "run-1", Type: "T", Status: "Running", TaskFailure: true})
	cells := wl.table.GetRowCells(0)
	statusIdx := -1
	for i, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnStatus {
			statusIdx = i
			break
		}
	}
	if statusIdx < 0 || statusIdx >= len(cells) {
		t.Fatal("missing status column")
	}
	if cells[statusIdx].Status != temporal.StatusUnhandledFailure && !strings.Contains(cells[statusIdx].Text, "Unhan") {
		t.Fatalf("status cell=%q status=%v", cells[statusIdx].Text, cells[statusIdx].Status)
	}
}

func TestTreeModeKeepsParentIDColumn(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflowTreeMode = true
	found := false
	for _, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnParentID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("tree view should keep parent id so hoisted matches stay attributable")
	}
}
