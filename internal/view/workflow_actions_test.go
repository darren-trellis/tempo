package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestWorkflowActionStatus(t *testing.T) {
	if !workflowIsRunning("Running") || workflowIsRunning("Completed") {
		t.Fatal("running status")
	}
	if !workflowCanReset("Failed") || workflowCanReset("Running") {
		t.Fatal("reset status")
	}
}

func TestFilterHistoryEvents(t *testing.T) {
	events := []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Details: "id: 1"},
		{ID: 5, Type: "ActivityTaskScheduled", ActivityType: "ValidateOrder"},
		{ID: 9, Type: "ActivityTaskFailed", Failure: "timeout"},
	}
	got := filterHistoryEvents(events, "validate")
	if len(got) != 1 || got[0].ID != 5 {
		t.Fatalf("activity filter: %+v", got)
	}
	got = filterHistoryEvents(events, "timeout")
	if len(got) != 1 || got[0].ID != 9 {
		t.Fatalf("failure filter: %+v", got)
	}
	if n := len(filterHistoryEvents(events, "")); n != 3 {
		t.Fatalf("empty query should keep all, got %d", n)
	}
}

func TestPreviewEventSearchFiltersTable(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.previewKind = previewEvents
	wl.previewEvents = []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted"},
		{ID: 5, Type: "ActivityTaskScheduled", ActivityType: "Charge"},
	}
	wl.applyPreviewEventSearch("charge")
	visible := wl.visiblePreviewEvents()
	if len(visible) != 1 || visible[0].ID != 5 {
		t.Fatalf("visible: %+v", visible)
	}
}

func TestWorkflowDetailOmitsEventModal(t *testing.T) {
	wd := NewWorkflowDetail(&App{}, "wf", "run")
	if desc := hintDescription(wd.Hints(), "d"); desc != "" {
		t.Fatalf("event modal should be gone, got %q", desc)
	}
}

func TestPreviewEventHintsOmitEventModal(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.setPreviewKind(previewEvents)
	wl.setFocusPane(focusEvents)
	if desc := hintDescription(wl.Hints(), "d"); desc != "" && desc != "Delete" {
		t.Fatalf("preview should not hint event modal, got %q", desc)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'd', 0)); ev == nil {
		t.Fatal("d should not open an event modal from preview")
	}
	if desc := hintDescription(wl.Hints(), "/"); desc != "Search" {
		t.Fatalf("events search hint: %q", desc)
	}
}

func TestListHintsIncludeWorkflowActions(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	running := false
	for i, w := range wl.workflows {
		if w.Status == "Running" {
			wl.table.SelectRow(i)
			running = true
			break
		}
	}
	if !running {
		t.Fatal("expected a running mock workflow")
	}
	if desc := hintDescription(wl.Hints(), "c"); desc != "Cancel" {
		t.Fatalf("list cancel hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "Q"); desc != "Query" {
		t.Fatalf("list query hint: %q", desc)
	}
}
