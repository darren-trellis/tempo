package view

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
		if h.Key == "tab" || h.Key == "j/k" {
			t.Fatalf("obvious key should stay off the footer: %q", h.Key)
		}
	}
	wd.setFocusPane(detailFocusEventDetail)
	for _, h := range wd.Hints() {
		if h.Description == "Active" || h.Description == "active" {
			t.Fatal("hints should not say Active")
		}
	}
}

func TestScrollTextViewStopsAtTop(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true)
	view.SetText(strings.Repeat("line\n", 50))
	view.ScrollTo(5, 0)

	scrollTextView(view, -100)
	if row, _ := view.GetScrollOffset(); row != 0 {
		t.Fatalf("scroll should clamp at top, got %d", row)
	}
}

func TestHandleTextViewScrollPageUpStopsAtTop(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true)
	view.SetText(strings.Repeat("line\n", 50))

	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone)) {
		t.Fatal("page up should be handled")
	}
	if row, _ := view.GetScrollOffset(); row < 0 {
		t.Fatalf("page up should not scroll above top, got %d", row)
	}
}

func TestWorkflowDetailPageUpStopsAtTop(t *testing.T) {
	wd := NewWorkflowDetail(&App{}, "wf", "run")
	capture := wd.eventDetailView.GetInputCapture()
	if capture == nil {
		t.Fatal("event detail should capture keys")
	}
	wd.eventDetailView.SetText(strings.Repeat("line\n", 80))
	wd.eventDetailView.ScrollTo(3, 0)

	if ev := capture(tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone)); ev != nil {
		t.Fatal("page up should be consumed")
	}
	if row, _ := wd.eventDetailView.GetScrollOffset(); row < 0 {
		t.Fatalf("page up should not leave a negative offset, got %d", row)
	}
}
