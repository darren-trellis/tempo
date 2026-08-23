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
