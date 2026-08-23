package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestWorkflowIOHints(t *testing.T) {
	hints := workflowIOHints(false)
	if len(hints) != 4 {
		t.Fatalf("want io hints without nav keys, got %d", len(hints))
	}
	if hints[0].Description != "Maximize" {
		t.Fatalf("restore hint: %+v", hints[0])
	}
	if got := workflowIOHints(true)[0].Description; got != "Minimize" {
		t.Fatalf("maximize hint: %q", got)
	}
}

func TestWorkflowIOFromEvents(t *testing.T) {
	input, output := workflowIOFromEvents([]temporal.EnhancedHistoryEvent{
		{Type: "WorkflowExecutionStarted", Input: `{"id":1}`},
		{Type: "ActivityTaskCompleted", Result: "ignored"},
		{Type: "WorkflowExecutionCompleted", Result: `{"ok":true}`},
	})
	if input != `{"id":1}` {
		t.Fatalf("input: %q", input)
	}
	if output != `{"ok":true}` {
		t.Fatalf("output: %q", output)
	}

	_, failed := workflowIOFromEvents([]temporal.EnhancedHistoryEvent{
		{Type: "WorkflowExecutionFailed", Failure: "boom"},
	})
	if failed != "boom" {
		t.Fatalf("failure output: %q", failed)
	}
}

func TestWorkflowIOWithoutPreview(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if desc := hintDescription(wl.Hints(), "i"); desc != "Input/Output" {
		t.Fatalf("workflows pane should show io without preview, got %q", desc)
	}
	if wl.showPreviewIO() {
		t.Fatal("input/output needs a selected workflow")
	}

	wf := temporal.Workflow{ID: "wf", RunID: "run", Type: "Order"}
	wl.workflows = []temporal.Workflow{wf}
	wl.table.AddRowWithColor(0, "Completed", "Order", "wf", "run")
	wl.table.SelectRow(0)
	wl.previewCache.put(wf.ID, wf.RunID, []temporal.EnhancedHistoryEvent{
		{Type: "WorkflowExecutionStarted", Input: `{"id":1}`},
		{Type: "WorkflowExecutionCompleted", Result: `{"ok":true}`},
	})

	title, input, output, ok := wl.previewIOPayload()
	if !ok || title != "Order" || input != `{"id":1}` || output != `{"ok":true}` {
		t.Fatalf("workflow io without preview: title=%q input=%q output=%q ok=%v", title, input, output, ok)
	}
	if !wl.showPreviewIO() {
		t.Fatal("input/output should open from the workflows list")
	}
}

func TestPreviewActivityIOPayload(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.previewMode = true
	wl.previewKind = previewActivities
	wl.previewActivities = []previewActivity{
		{Type: "ValidateOrder", Input: `{"id":1}`, Result: `{"ok":true}`},
		{Type: "Charge", Input: `{"amt":5}`, Failure: "timeout"},
	}
	wl.eventTable.AddRowWithColor(0, "Completed", "ValidateOrder", "12:00:00", "2s")
	wl.eventTable.AddRowWithColor(0, "Failed", "Charge", "12:00:03", "1s")
	wl.eventTable.SelectRow(0)

	title, input, output, ok := wl.previewIOPayload()
	if !ok || title != "ValidateOrder" || input != `{"id":1}` || output != `{"ok":true}` {
		t.Fatalf("first activity io: title=%q input=%q output=%q ok=%v", title, input, output, ok)
	}

	wl.eventTable.SelectRow(1)
	title, input, output, ok = wl.previewIOPayload()
	if !ok || title != "Charge" || input != `{"amt":5}` || output != "timeout" {
		t.Fatalf("failed activity io: title=%q input=%q output=%q ok=%v", title, input, output, ok)
	}
}
