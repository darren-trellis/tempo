package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestNewWorkflowListDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewWorkflowList panicked: %v", r)
		}
	}()
	NewWorkflowList(&App{}, "default")
}

func TestWorkflowTableKeepsHorizontalScrollKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()

	if wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) {
		t.Fatal("left should not switch preview tabs from the workflows table")
	}
	if wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyRune, 'h', 0)) {
		t.Fatal("h should not switch preview tabs from the workflows table")
	}

	wl.table.SetOffset(0, 3)
	wl.table.InputHandler()(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), func(tview.Primitive) {})
	_, col := wl.table.GetOffset()
	if col != 2 {
		t.Fatalf("left should scroll the table, column offset=%d", col)
	}

	if !wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should still switch preview tabs")
	}
	if wl.previewKind != previewEvents {
		t.Fatalf("] should go to events, got %d", wl.previewKind)
	}
	if desc := hintDescription(wl.Hints(), "h/l"); desc != "Scroll" {
		t.Fatalf("workflows hint should keep h/l scroll, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "[/]/1-3"); desc != "View" {
		t.Fatalf("preview tabs should use [/]/1-3, got %q", desc)
	}
}

func TestPreviewModeLayout(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.GetItemCount() != 1 {
		t.Fatalf("default layout should be workflows only, got %d items", wl.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "Detail" {
		t.Fatalf("default enter hint: got %q", desc)
	}

	wl.togglePreviewMode()
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("preview should show events pane, got %d items", wl.mainFlex.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "Activities" {
		t.Fatalf("preview enter hint: got %q", desc)
	}

	wl.togglePreviewMode()
	if wl.GetItemCount() != 1 {
		t.Fatalf("hiding preview should show workflows only, got %d items", wl.GetItemCount())
	}
}

func TestPreviewTextViewPageUpStopsAtTop(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	capture := wl.eventDetail.GetInputCapture()
	if capture == nil {
		t.Fatal("event detail should capture keys")
	}
	wl.eventDetail.SetText(strings.Repeat("line\n", 80))
	wl.eventDetail.ScrollTo(4, 0)

	if ev := capture(tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone)); ev != nil {
		t.Fatal("page up should be consumed")
	}
	if row, _ := wl.eventDetail.GetScrollOffset(); row < 0 {
		t.Fatalf("page up should not leave a negative offset, got %d", row)
	}
}

func TestTimelineToggleLayout(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.GetItemCount() != 1 {
		t.Fatalf("default should hide timeline, got %d items", wl.GetItemCount())
	}
	wl.toggleTimeline()
	if !wl.timelineVisible || wl.GetItemCount() != 2 {
		t.Fatalf("timeline should add a bottom pane, visible=%v items=%d", wl.timelineVisible, wl.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "z"); desc != "Timeline" {
		t.Fatalf("timeline hint: %q", desc)
	}
	wl.toggleTimeline()
	if wl.timelineVisible || wl.GetItemCount() != 1 {
		t.Fatal("toggling again should hide the timeline")
	}
}

func TestTimelineHighlightsSelectedActivity(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.toggleTimeline()
	wl.showPreviewEvents(temporal.Workflow{ID: "wf", RunID: "run"}, []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: time.Now().Add(-time.Minute)},
		{ID: 5, Type: "ActivityTaskScheduled", Time: time.Now().Add(-50 * time.Second), ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: time.Now().Add(-40 * time.Second), ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: time.Now().Add(-30 * time.Second), ActivityType: "Second"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: time.Now().Add(-20 * time.Second), ScheduledEventID: 8},
	})
	if wl.eventTable.RowCount() < 2 {
		t.Fatalf("expected two activities, got %d", wl.eventTable.RowCount())
	}
	wl.eventTable.SelectRow(1)
	wl.updatePreviewSelection(2)
	if wl.highlightedActivityID != 8 {
		t.Fatalf("highlighted activity=%d", wl.highlightedActivityID)
	}
	wl.setFocusPane(focusTimeline)
	lane := wl.timelineView.SelectedLane()
	if lane == nil || timelineLaneScheduledID(*lane) != 8 {
		t.Fatal("timeline should highlight the selected activity bar")
	}
}

func hintDescription(hints []KeyHint, key string) string {
	for _, h := range hints {
		if h.Key == key {
			return h.Description
		}
	}
	return ""
}
