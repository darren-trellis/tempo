package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestFormatWorkflowInfoIncludesParent(t *testing.T) {
	parent := "parent-workflow"
	got := formatWorkflowInfo(temporal.Workflow{
		ID:        "child-workflow",
		RunID:     "run-abcdefghijklmnopqrstuvwx",
		Type:      "ChildType",
		Status:    "Running",
		TaskQueue: "default",
		StartTime: time.Now(),
		ParentID:  &parent,
	})
	if !strings.Contains(got, "Parent") || !strings.Contains(got, parent) {
		t.Fatalf("expected parent workflow id in details, got %q", got)
	}
	if !strings.Contains(got, "run-abcdefghijklmnopqrstuvwx") {
		t.Fatalf("run id should not be truncated, got %q", got)
	}
}

func TestWorkflowInfoRowsIncludeFullValues(t *testing.T) {
	parent := "parent-workflow"
	runID := "run-abcdefghijklmnopqrstuvwxyz-full"
	rows := workflowInfoRows(time.Now(), temporal.Workflow{
		ID:        "child-workflow",
		RunID:     runID,
		Type:      "ChildType",
		Status:    "Running",
		TaskQueue: "very-long-task-queue-name",
		StartTime: time.Now(),
		ParentID:  &parent,
	})
	if idx := workflowInfoRowIndex(rows, workflowInfoParent); idx < 0 || rows[idx].Value != parent {
		t.Fatalf("parent row: %+v", rows)
	}
	if idx := workflowInfoRowIndex(rows, workflowInfoRunID); idx < 0 || rows[idx].Value != runID {
		t.Fatalf("run id row: %+v", rows)
	}
	if got := workflowInfoContentWidth(rows); got < len("Task Queue")+1+len("very-long-task-queue-name") {
		t.Fatalf("content width too small: %d", got)
	}
}

func TestWorkflowInfoRowsMarkUnhandledFailure(t *testing.T) {
	rows := workflowInfoRows(time.Now(), temporal.Workflow{
		ID:          "wf",
		Status:      "Running",
		TaskFailure: true,
		StartTime:   time.Now(),
	})
	idx := workflowInfoRowIndex(rows, workflowInfoStatus)
	if idx < 0 {
		t.Fatal("missing status row")
	}
	if rows[idx].Value != "Unhandled Failure" {
		t.Fatalf("value=%q", rows[idx].Value)
	}
	if !strings.Contains(rows[idx].Display, "Unhandled Failure") {
		t.Fatalf("display=%q", rows[idx].Display)
	}
}

func TestFormatWorkflowInfoOmitsMissingParent(t *testing.T) {
	got := formatWorkflowInfo(temporal.Workflow{
		ID:        "solo-workflow",
		RunID:     "run-abcdefghijklmnopqrstuvwx",
		Type:      "SoloType",
		Status:    "Running",
		TaskQueue: "default",
		StartTime: time.Now(),
	})
	if strings.Contains(got, "Parent") {
		t.Fatalf("did not expect parent line, got %q", got)
	}
}
