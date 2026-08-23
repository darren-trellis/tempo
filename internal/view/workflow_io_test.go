package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
)

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

func TestShowPreviewIORequiresPreviewMode(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.showPreviewIO() {
		t.Fatal("input/output should be preview-mode only")
	}
}
