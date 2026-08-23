package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

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

func TestWorkflowDetailRestoresPaneTabAfterStop(t *testing.T) {
	wd := NewWorkflowDetail(&App{}, "wf", "run")
	wd.Stop()
	wd.Start()

	capture := wd.eventDetailView.GetInputCapture()
	if capture == nil {
		t.Fatal("event detail should keep tab handling after a modal closes")
	}
	wd.focusPane = detailFocusEventDetail
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
		t.Fatal("tab should be consumed")
	}
	if wd.focusPane != detailFocusWorkflow {
		t.Fatalf("tab should leave event detail, got %d", wd.focusPane)
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
