package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func startedAndCompleted() []temporal.EnhancedHistoryEvent {
	return []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Input: `{"order":1}`},
		{ID: 2, Type: "ActivityTaskCompleted", Result: `"activity"`},
		{ID: 3, Type: "WorkflowExecutionCompleted", Result: `{"ok":true}`},
	}
}

func TestWorkflowHistoryComplete(t *testing.T) {
	if workflowHistoryComplete(nil) {
		t.Fatal("no events is not a complete history")
	}
	// A snapshot taken mid-run has no terminal event, however long it is.
	running := []temporal.EnhancedHistoryEvent{
		{Type: "WorkflowExecutionStarted"},
		{Type: "ActivityTaskScheduled"},
		{Type: "ActivityTaskCompleted", Result: `"activity"`},
		{Type: "ChildWorkflowExecutionCompleted", Result: `"child"`},
	}
	if workflowHistoryComplete(running) {
		t.Fatal("a child workflow finishing does not end the parent")
	}
	for _, terminal := range []string{
		"WorkflowExecutionCompleted", "WorkflowExecutionFailed", "WorkflowExecutionCanceled",
		"WorkflowExecutionTerminated", "WorkflowExecutionTimedOut", "WorkflowExecutionContinuedAsNew",
	} {
		events := append(running, temporal.EnhancedHistoryEvent{Type: terminal})
		if !workflowHistoryComplete(events) {
			t.Fatalf("%s should end the history", terminal)
		}
	}
}

// TestWorkflowIOEventsRejectsPreCompletionSnapshot is the bug: with the preview
// closed nothing refreshes the cache, so the input/output modal was reading a
// snapshot captured while the workflow was still running and reporting no output.
func TestWorkflowIOEventsRejectsPreCompletionSnapshot(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	done := time.Now()
	w := temporal.Workflow{ID: "wf-1", RunID: "run-1", Type: "OrderWorkflow", Status: "Completed", EndTime: &done}

	// Cached while it was still running.
	wl.previewCache.put(w.ID, w.RunID, []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Input: `{"order":1}`},
		{ID: 2, Type: "ActivityTaskCompleted", Result: `"activity"`},
	})
	if _, ok := wl.workflowIOEvents(w); ok {
		t.Fatal("a pre-completion snapshot should not answer for a finished workflow")
	}

	// Once the full history is cached, use it.
	wl.previewCache.put(w.ID, w.RunID, startedAndCompleted())
	events, ok := wl.workflowIOEvents(w)
	if !ok {
		t.Fatal("a complete history should answer")
	}
	input, output := workflowIOFromEvents(events)
	if input == "" || output == "" {
		t.Fatalf("input=%q output=%q", input, output)
	}

	// The same applies to the live preview slice.
	wl.previewCache = newPreviewCache(0)
	wl.previewWorkflowID, wl.previewRunID = w.ID, w.RunID
	wl.previewEvents = []temporal.EnhancedHistoryEvent{{ID: 1, Type: "WorkflowExecutionStarted", Input: `{"order":1}`}}
	if _, ok := wl.workflowIOEvents(w); ok {
		t.Fatal("a pre-completion preview slice should not answer either")
	}

	// A workflow still running has nothing better to offer, so use what we have.
	running := temporal.Workflow{ID: "wf-2", RunID: "run-2", Status: "Running"}
	wl.previewWorkflowID, wl.previewRunID = running.ID, running.RunID
	if _, ok := wl.workflowIOEvents(running); !ok {
		t.Fatal("a running workflow should still show what history exists")
	}
}

func TestWorkflowIOPayloadWithPreviewClosed(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	done := time.Now()
	w := temporal.Workflow{ID: "wf-1", RunID: "run-1", Type: "OrderWorkflow", Status: "Completed", EndTime: &done}
	wl.workflows = []temporal.Workflow{w}
	wl.allWorkflows = wl.workflows
	wl.renderColumns()
	wl.table.SelectRow(0)
	wl.previewCache.put(w.ID, w.RunID, startedAndCompleted())

	if wl.previewModeEnabled() {
		t.Fatal("this covers the preview being closed")
	}
	title, input, output, ok := wl.previewIOPayload()
	if !ok || title != "OrderWorkflow" {
		t.Fatalf("payload: title=%q ok=%v", title, ok)
	}
	if input == "" {
		t.Fatal("input should be read from the history")
	}
	if output == "" {
		t.Fatal("output should be read from the history with the preview closed")
	}
}

func TestMockPreviewEventsCarryIO(t *testing.T) {
	done := time.Now()
	events := mockPreviewEvents(temporal.Workflow{ID: "wf-1", Status: "Completed", EndTime: &done})
	input, output := workflowIOFromEvents(events)
	if input == "" || output == "" {
		t.Fatalf("dev mode should demonstrate input/output: input=%q output=%q", input, output)
	}

	failed := mockPreviewEvents(temporal.Workflow{ID: "wf-2", Status: "Failed", EndTime: &done})
	if _, output := workflowIOFromEvents(failed); output == "" {
		t.Fatal("a failed workflow should show its failure as output")
	}
}
