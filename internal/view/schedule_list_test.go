package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestScheduleInfoRows(t *testing.T) {
	next := time.Now().Add(time.Hour)
	last := time.Now().Add(-time.Hour)
	rows := scheduleInfoRows(time.Now(), temporal.Schedule{
		ID: "nightly", WorkflowType: "ReportWorkflow", WorkflowID: "report-1",
		TaskQueue: "reports", Spec: "every 24h", OverlapPolicy: "Skip",
		NextRunTime: &next, LastRunTime: &last, LastRunStatus: "Completed",
		TotalActions: 12, RecentActions: 2, Notes: "owned by data",
	})
	want := map[string]string{
		"id":            "nightly",
		"status":        "Active",
		"type":          "ReportWorkflow",
		"workflowid":    "report-1",
		"taskqueue":     "reports",
		"spec":          "every 24h",
		"overlap":       "Skip",
		"laststatus":    "Completed",
		"actions":       "12",
		"recentactions": "2",
		"notes":         "owned by data",
	}
	for key, value := range want {
		if got := infoRowValue(rows, key); got != value {
			t.Fatalf("row %q: got %q want %q", key, got, value)
		}
	}

	paused := scheduleInfoRows(time.Now(), temporal.Schedule{ID: "nightly", Paused: true})
	if got := infoRowValue(paused, "status"); got != "Paused" {
		t.Fatalf("paused status: %q", got)
	}
	if got := infoRowValue(paused, "nextrun"); got != "-" {
		t.Fatalf("a schedule with no next run should show a dash, got %q", got)
	}
}

func TestScheduleRunsPaneNewestFirst(t *testing.T) {
	sl := NewScheduleList(&App{}, "default")
	now := time.Now()
	sl.updatePreview(temporal.Schedule{
		ID: "nightly",
		RecentRuns: []temporal.ScheduleRun{
			{WorkflowID: "run-old", RunID: "a", ActualTime: now.Add(-2 * time.Hour)},
			{WorkflowID: "run-new", RunID: "b", ActualTime: now.Add(-time.Minute)},
		},
	})
	if len(sl.runs) != 2 || sl.runs[0].WorkflowID != "run-new" {
		t.Fatalf("runs should be newest first: %+v", sl.runs)
	}
	if sl.runsTable.RowCount() != 2 {
		t.Fatalf("runs table rows: %d", sl.runsTable.RowCount())
	}
	run, ok := sl.selectedRun()
	if !ok || run.WorkflowID != "run-new" {
		t.Fatalf("selected run: %+v ok=%v", run, ok)
	}
	if got := sl.runsTable.GetCell(1, 2).Text; got != "run-new" {
		t.Fatalf("first row workflow id: %q", got)
	}

	// A schedule with no runs empties the pane.
	sl.updatePreview(temporal.Schedule{ID: "nightly"})
	if len(sl.runs) != 0 || sl.runsTable.RowCount() != 0 {
		t.Fatalf("runs should clear, got %d rows", sl.runsTable.RowCount())
	}
}

func TestSchedulesTabCyclesFocusThroughBothPanes(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.setListKind(listSchedules)
	if !wl.schedulesActive() {
		t.Fatal("schedules tab should be active")
	}

	tab := func(capture func(*tcell.EventKey) *tcell.EventKey) {
		t.Helper()
		if capture == nil {
			t.Fatal("pane has no input capture, so tab cannot cycle out of it")
		}
		if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
			t.Fatal("tab should be consumed by the focus cycle")
		}
	}

	tab(wl.schedules.table.GetInputCapture())
	if wl.focusPane != focusScheduleDetail {
		t.Fatalf("tab from the list should focus the details pane, got %d", wl.focusPane)
	}
	tab(wl.schedules.detail.GetInputCapture())
	if wl.focusPane != focusScheduleRuns {
		t.Fatalf("tab from the details pane should focus recent runs, got %d", wl.focusPane)
	}
	tab(wl.schedules.runsTable.GetInputCapture())
	if wl.focusPane != focusWorkflows {
		t.Fatalf("tab from recent runs should return to the list, got %d", wl.focusPane)
	}

	// Escape from either pane closes the sidebar; enter brings it back.
	wl.setFocusPane(focusScheduleRuns)
	if !wl.HandleEscape() || wl.scheduleDetailVisible || wl.focusPane != focusWorkflows || !wl.schedulesActive() {
		t.Fatalf("escape from recent runs should close the sidebar, visible=%v pane=%d", wl.scheduleDetailVisible, wl.focusPane)
	}
	if wl.mainFlex.GetItemCount() != 1 {
		t.Fatalf("a closed sidebar should leave the list full width, got %d panes", wl.mainFlex.GetItemCount())
	}
	if order := wl.previewFocusOrder(); len(order) != 1 {
		t.Fatalf("a closed sidebar should drop out of the focus cycle, got %v", order)
	}

	capture := wl.schedules.table.GetInputCapture()
	if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Fatal("enter should be consumed")
	}
	if !wl.scheduleDetailVisible || wl.focusPane != focusScheduleDetail {
		t.Fatalf("enter should reopen the sidebar on the details pane, visible=%v pane=%d", wl.scheduleDetailVisible, wl.focusPane)
	}
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("reopened sidebar should sit beside the list, got %d panes", wl.mainFlex.GetItemCount())
	}

	wl.setFocusPane(focusScheduleDetail)
	if !wl.HandleEscape() || wl.scheduleDetailVisible {
		t.Fatal("escape from the details pane should close the sidebar too")
	}
	wl.setScheduleDetailVisible(true)

	// The sidebar keeps its state across tab switches.
	wl.setScheduleDetailVisible(false)
	wl.setListKind(listWorkflows)
	wl.setListKind(listSchedules)
	if wl.scheduleDetailVisible {
		t.Fatal("a closed sidebar should stay closed after a tab switch")
	}
	wl.setScheduleDetailVisible(true)

	if hintDescription(wl.Hints(), "Enter") != "" {
		t.Fatalf("list hints should not advertise enter: %q", hintDescription(wl.Hints(), "Enter"))
	}
	wl.focusPane = focusScheduleRuns
	if hintDescription(wl.Hints(), "Enter") != "Open Run" {
		t.Fatalf("runs hints: %q", hintDescription(wl.Hints(), "Enter"))
	}
}
