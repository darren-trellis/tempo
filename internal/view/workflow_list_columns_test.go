package view

import (
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

func TestTreeModeHidesParentIDColumn(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflowTreeMode = true
	for _, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnParentID {
			t.Fatal("tree view should hide parent id")
		}
	}
}
