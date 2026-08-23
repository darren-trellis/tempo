package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestNestWorkflowsIndentsChildren(t *testing.T) {
	now := time.Now()
	parent := "root-wf"
	child := "child-wf"
	workflows := []temporal.Workflow{
		{ID: parent, RunID: "r1", StartTime: now},
		{ID: "other", RunID: "r2", StartTime: now.Add(-time.Minute)},
		{ID: child, RunID: "r3", StartTime: now.Add(-30 * time.Second), ParentID: &parent},
		{ID: "grandchild", RunID: "r4", StartTime: now.Add(-10 * time.Second), ParentID: &child},
	}

	got, depths := nestWorkflows(workflows)
	if len(got) != 4 || len(depths) != 4 {
		t.Fatalf("len workflows=%d depths=%d", len(got), len(depths))
	}
	if got[0].ID != parent || depths[0] != 0 {
		t.Fatalf("root should stay first, got %s depth %d", got[0].ID, depths[0])
	}
	if got[1].ID != child || depths[1] != 1 {
		t.Fatalf("child should sit under parent, got %s depth %d", got[1].ID, depths[1])
	}
	if got[2].ID != "grandchild" || depths[2] != 2 {
		t.Fatalf("grandchild should nest, got %s depth %d", got[2].ID, depths[2])
	}
	if got[3].ID != "other" || depths[3] != 0 {
		t.Fatalf("unrelated root should follow, got %s depth %d", got[3].ID, depths[3])
	}
}

func TestNestWorkflowsMissingParentStaysRoot(t *testing.T) {
	missing := "not-in-list"
	workflows := []temporal.Workflow{
		{ID: "orphan", ParentID: &missing},
		{ID: "solo"},
	}
	got, depths := nestWorkflows(workflows)
	if len(got) != 2 || depths[0] != 0 || depths[1] != 0 {
		t.Fatalf("missing parents should stay roots, depths=%v", depths)
	}
}

func TestWorkflowTreePrefix(t *testing.T) {
	if workflowTreePrefix(0) != "" {
		t.Fatal("roots should not be indented")
	}
	if workflowTreePrefix(1) != "  └ " {
		t.Fatalf("got %q", workflowTreePrefix(1))
	}
	if workflowTreePrefix(2) != "    └ " {
		t.Fatalf("got %q", workflowTreePrefix(2))
	}
}

func TestToggleWorkflowTreeReordersRows(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	if desc := hintDescription(wl.Hints(), "b"); desc != "Tree" {
		t.Fatalf("list hint: %q", desc)
	}
	flat := append([]temporal.Workflow(nil), wl.workflows...)
	wl.toggleWorkflowTree()
	if !wl.workflowTreeMode {
		t.Fatal("tree mode should be on")
	}
	if desc := hintDescription(wl.Hints(), "b"); desc != "List" {
		t.Fatalf("tree hint: %q", desc)
	}
	if len(wl.workflows) != len(flat) {
		t.Fatalf("tree should keep all rows, got %d want %d", len(wl.workflows), len(flat))
	}
	foundChild := false
	for i, w := range wl.workflows {
		if w.ID == "payment-xyz789" {
			foundChild = true
			if wl.workflowDepth(i) != 1 {
				t.Fatalf("payment should be indented under parent, depth=%d", wl.workflowDepth(i))
			}
			if i == 0 || wl.workflows[i-1].ID != "order-processing-abc123" {
				t.Fatal("payment should appear directly under its parent")
			}
		}
		if w.ID == "fulfillment-ghi000" && wl.workflowDepth(i) != 2 {
			t.Fatalf("fulfillment should nest under payment, depth=%d", wl.workflowDepth(i))
		}
	}
	if !foundChild {
		t.Fatal("expected child workflow in tree")
	}
}
