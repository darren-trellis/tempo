package view

import "testing"

func TestWorkflowDetailFocusCycle(t *testing.T) {
	wd := NewWorkflowDetail(&App{}, "wf", "run")
	if wd.focusPane != detailFocusEvents {
		t.Fatal("detail should start on events")
	}
	if !wd.eventsPanel.IsFocused() || wd.workflowPanel.IsFocused() || wd.eventDetailPanel.IsFocused() {
		t.Fatal("only the events pane should show focus style")
	}

	wd.cycleFocus(1)
	if wd.focusPane != detailFocusEventDetail {
		t.Fatalf("tab should move to event detail, got %d", wd.focusPane)
	}
	if !wd.eventDetailPanel.IsFocused() || wd.eventsPanel.IsFocused() {
		t.Fatal("event detail should show focus style")
	}

	wd.cycleFocus(1)
	if wd.focusPane != detailFocusWorkflow {
		t.Fatalf("tab should move to workflow, got %d", wd.focusPane)
	}

	wd.cycleFocus(1)
	if wd.focusPane != detailFocusEvents {
		t.Fatal("tab should wrap back to events")
	}
}

func TestWorkflowDetailHintsOmitActive(t *testing.T) {
	wd := NewWorkflowDetail(&App{}, "wf", "run")
	for _, h := range wd.Hints() {
		if h.Key == "tab" && h.Description != "Detail" {
			t.Fatalf("events tab hint: %q", h.Description)
		}
	}
	wd.setFocusPane(detailFocusEventDetail)
	for _, h := range wd.Hints() {
		if h.Description == "Active" || h.Description == "active" {
			t.Fatal("hints should not say Active")
		}
	}
}
