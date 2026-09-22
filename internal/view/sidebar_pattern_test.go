package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestEscapeClosesEveryFocusableSidebar covers the shared pattern: a sidebar is
// open by default, escape closes it while it holds focus, and the primary pane
// brings it back.
func TestEscapeClosesEveryFocusableSidebar(t *testing.T) {
	cases := []struct {
		name    string
		kind    listKind
		pane    workflowFocusPane
		visible func(*WorkflowList) bool
		reopen  func(*WorkflowList)
		primary func(*WorkflowList) *components.Table
	}{
		{
			name:    "task queue pollers",
			kind:    listTaskQueues,
			pane:    focusPollers,
			visible: func(wl *WorkflowList) bool { return wl.pollersVisible },
			reopen:  func(wl *WorkflowList) { wl.setPollersVisible(true) },
			primary: func(wl *WorkflowList) *components.Table { return wl.taskQueues.queueTable },
		},
		{
			name:    "schedule details",
			kind:    listSchedules,
			pane:    focusScheduleDetail,
			visible: func(wl *WorkflowList) bool { return wl.scheduleDetailVisible },
			reopen:  func(wl *WorkflowList) { wl.setScheduleDetailVisible(true) },
			primary: func(wl *WorkflowList) *components.Table { return wl.schedules.table },
		},
		{
			name:    "schedule recent runs",
			kind:    listSchedules,
			pane:    focusScheduleRuns,
			visible: func(wl *WorkflowList) bool { return wl.scheduleDetailVisible },
			reopen:  func(wl *WorkflowList) { wl.setScheduleDetailVisible(true) },
			primary: func(wl *WorkflowList) *components.Table { return wl.schedules.table },
		},
		{
			name:    "worker detail",
			kind:    listWorkers,
			pane:    focusWorkerDetail,
			visible: func(wl *WorkflowList) bool { return wl.workerDetailVisible },
			reopen:  func(wl *WorkflowList) { wl.setWorkerDetailVisible(true) },
			primary: func(wl *WorkflowList) *components.Table { return wl.workers.table },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wl := NewWorkflowList(&App{}, "default")
			wl.setListKind(tc.kind)
			if !tc.visible(wl) {
				t.Fatal("the sidebar should be open by default")
			}
			if wl.mainFlex.GetItemCount() != 2 {
				t.Fatalf("sidebar should sit beside the list, got %d panes", wl.mainFlex.GetItemCount())
			}

			wl.setFocusPane(tc.pane)
			if !wl.HandleEscape() {
				t.Fatal("escape should be handled while the sidebar holds focus")
			}
			if tc.visible(wl) {
				t.Fatal("escape should close the sidebar")
			}
			if wl.focusPane != focusWorkflows {
				t.Fatalf("closing should return focus to the list, got %d", wl.focusPane)
			}
			if wl.mainFlex.GetItemCount() != 1 {
				t.Fatalf("closed sidebar should leave the list full width, got %d panes", wl.mainFlex.GetItemCount())
			}

			// Enter on the primary pane brings it back.
			capture := tc.primary(wl).GetInputCapture()
			if capture == nil {
				t.Fatal("the primary pane has no input capture")
			}
			if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
				t.Fatal("enter should be consumed while reopening the sidebar")
			}
			if !tc.visible(wl) || wl.mainFlex.GetItemCount() != 2 {
				t.Fatalf("enter should reopen the sidebar, visible=%v panes=%d", tc.visible(wl), wl.mainFlex.GetItemCount())
			}
			if wl.focusPane == focusWorkflows {
				t.Fatal("enter should also move focus into the reopened sidebar")
			}

			tc.reopen(wl)
			if !tc.visible(wl) || wl.mainFlex.GetItemCount() != 2 {
				t.Fatalf("sidebar should stay open, visible=%v panes=%d", tc.visible(wl), wl.mainFlex.GetItemCount())
			}
		})
	}
}

func TestEscapeClosesWorkflowPreview(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	if !wl.previewModeEnabled() {
		t.Fatal("preview should be open")
	}

	// Escape from a preview pane closes it, whichever pane holds focus.
	for _, pane := range []workflowFocusPane{focusEvents, focusEventDetail} {
		wl.setPreviewVisible(true)
		wl.focusPane = pane
		if !wl.HandleEscape() {
			t.Fatalf("escape should be handled from pane %d", pane)
		}
		if wl.previewModeEnabled() {
			t.Fatalf("escape from pane %d should close the preview", pane)
		}
		if wl.focusPane != focusWorkflows {
			t.Fatalf("closing should return focus to the list, got %d", wl.focusPane)
		}
	}

	// The preview panes' own capture closes it too, rather than only unfocusing.
	wl.setPreviewVisible(true)
	wl.focusPane = focusEvents
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Fatal("escape should be consumed by the preview")
	}
	if wl.previewModeEnabled() {
		t.Fatal("escape should close the preview from its own capture")
	}

	// With the preview closed, escape falls through to the list's own handling.
	wl.focusPane = focusWorkflows
	if wl.escapeFromPreview() {
		t.Fatal("escape should do nothing to a closed preview")
	}
}

// TestEveryScrollablePaneTakesAHorizontalWheel guards the rule that anything
// scrollable by key is also scrollable by mouse.
func TestEveryScrollablePaneTakesAHorizontalWheel(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.setListKind(listSchedules)
	wl.setListKind(listWorkers)
	wl.setListKind(listTaskQueues)

	panes := map[string]*components.Table{
		"workflows":       wl.table,
		"workflow detail": wl.workflowDetail,
		"activity detail": wl.activityDetail,
		"activities":      wl.eventTable,
		"task queues":     wl.taskQueues.queueTable,
		"pollers":         wl.taskQueues.pollerTable,
		"schedules":       wl.schedules.table,
		"schedule detail": wl.schedules.detail,
		"schedule runs":   wl.schedules.runsTable,
		"workers":         wl.workers.table,
		"worker detail":   wl.workers.detail,
	}
	for name, table := range panes {
		if table == nil {
			t.Fatalf("%s: missing table", name)
		}
		capture := table.GetMouseCapture()
		if capture == nil {
			t.Fatalf("%s: no mouse capture, so a horizontal wheel cannot scroll it", name)
		}
		// These panes are empty, so the wheel has nowhere to go and is dropped
		// rather than consumed. Either way the capture has to claim the
		// gesture instead of handing it back untouched.
		action, event := capture(tview.MouseScrollRight, tcell.NewEventMouse(0, 0, tcell.WheelRight, tcell.ModNone))
		if action != tview.MouseConsumed && event != nil {
			t.Fatalf("%s: horizontal wheel was not handled", name)
		}
	}

	// Standalone views wire their own tables.
	nl := NewNamespaceList(&App{})
	if nl.tableScroll == nil || nl.table.GetMouseCapture() == nil {
		t.Fatal("namespaces: no horizontal mouse scrolling")
	}
}

func TestGraphPanePansWithHorizontalWheel(t *testing.T) {
	wg := NewWorkflowGraphView(&App{}, "default", nil)
	capture := wg.graph.GetMouseCapture()
	if capture == nil {
		t.Fatal("the hierarchy graph pans with h/l, so it needs a horizontal wheel too")
	}
	action, _ := capture(tview.MouseScrollRight, tcell.NewEventMouse(0, 0, tcell.WheelRight, tcell.ModNone))
	if action != tview.MouseConsumed {
		t.Fatal("horizontal wheel should be consumed by the graph")
	}
	// A vertical wheel still belongs to the component.
	action, _ = capture(tview.MouseScrollDown, tcell.NewEventMouse(0, 0, tcell.WheelDown, tcell.ModNone))
	if action == tview.MouseConsumed {
		t.Fatal("vertical wheel should pass through to the graph")
	}
}
