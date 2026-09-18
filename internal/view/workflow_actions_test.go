package view

import (
	"testing"
	"time"

	"github.com/atterpac/jig/layout"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestWorkflowActionStatus(t *testing.T) {
	if !workflowIsRunning("Running") || workflowIsRunning("Completed") {
		t.Fatal("running status")
	}
	if !workflowCanReset("Failed") || workflowCanReset("Running") {
		t.Fatal("reset status")
	}
}

func TestFormatResetPointLabel(t *testing.T) {
	got := formatResetPointLabel(temporal.ResetPoint{
		EventID:     12,
		EventType:   "WorkflowTaskCompleted",
		Timestamp:   time.Date(2026, 9, 17, 15, 4, 5, 0, time.UTC),
		Description: "Workflow task completed at event 12",
	})
	if got != "#12  15:04:05  Workflow task completed at event 12" {
		t.Fatalf("label=%q", got)
	}
	got = formatResetPointLabel(temporal.ResetPoint{
		EventID:   4,
		EventType: "WorkflowTaskCompleted",
		Timestamp: time.Date(2026, 9, 17, 15, 4, 5, 0, time.UTC),
	})
	if got != "#4  15:04:05  WorkflowTaskCompleted" {
		t.Fatalf("fallback label=%q", got)
	}
}

func TestResetPointLabelsKeepOrder(t *testing.T) {
	labels := resetPointLabels([]temporal.ResetPoint{
		{EventID: 4, Description: "first", Timestamp: time.Unix(0, 0)},
		{EventID: 4, Description: "same event id", Timestamp: time.Unix(1, 0)},
	})
	if len(labels) != 2 || labels[0] == labels[1] {
		t.Fatalf("labels=%v", labels)
	}
	if resetPointIndex(labels, labels[1]) != 1 {
		t.Fatalf("index for second label=%d", resetPointIndex(labels, labels[1]))
	}
}

func TestResetFormDefaults(t *testing.T) {
	selected, reason := resetFormDefaults(nil, 3)
	if selected != 0 || reason != config.DefaultResetReason {
		t.Fatalf("default selected=%d reason=%q", selected, reason)
	}
	app := &App{config: &config.Config{ResetPoint: config.ResetPointLast, ResetReason: "retry"}}
	selected, reason = resetFormDefaults(app, 3)
	if selected != 2 || reason != "retry" {
		t.Fatalf("last selected=%d reason=%q", selected, reason)
	}
}

func TestTerminateUnavailableMessage(t *testing.T) {
	if got := terminateUnavailableMessage("Completed"); got != "Cannot terminate a completed workflow" {
		t.Fatalf("got %q", got)
	}
	if got := terminateUnavailableMessage(""); got != "Cannot terminate this workflow" {
		t.Fatalf("empty status: %q", got)
	}
}

func TestTerminateCompletedShowsStatus(t *testing.T) {
	a := &App{menu: layout.NewMenu()}
	wl := NewWorkflowList(a, "default")
	wl.workflows = []temporal.Workflow{{ID: "wf-1", RunID: "run-1", Status: "Completed"}}
	wl.allWorkflows = wl.workflows
	wl.populateTable()
	wl.table.SelectRow(0)
	wl.showTerminateSelected()
	if got := a.hintBarMessage(); got != "Cannot terminate a completed workflow" {
		t.Fatalf("list: %q", got)
	}
}

func TestFilterHistoryEvents(t *testing.T) {
	events := []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Details: "id: 1"},
		{ID: 5, Type: "ActivityTaskScheduled", ActivityType: "ValidateOrder"},
		{ID: 9, Type: "ActivityTaskFailed", Failure: "timeout"},
	}
	got := filterHistoryEvents(events, "validate")
	if len(got) != 1 || got[0].ID != 5 {
		t.Fatalf("activity filter: %+v", got)
	}
	got = filterHistoryEvents(events, "timeout")
	if len(got) != 1 || got[0].ID != 9 {
		t.Fatalf("failure filter: %+v", got)
	}
	if n := len(filterHistoryEvents(events, "")); n != 3 {
		t.Fatalf("empty query should keep all, got %d", n)
	}
}

func TestFilterPreviewActivities(t *testing.T) {
	activities := []previewActivity{
		{Type: "ValidateOrder", Status: "Completed", ActivityID: "1"},
		{Type: "ChargeCard", Status: "Failed", Failure: "timeout"},
		{Type: "NotifyUser", Status: "Running", TaskQueue: "notify"},
	}
	got := filterPreviewActivities(activities, "charge")
	if len(got) != 1 || got[0].Type != "ChargeCard" {
		t.Fatalf("type filter: %+v", got)
	}
	got = filterPreviewActivities(activities, "timeout")
	if len(got) != 1 || got[0].Type != "ChargeCard" {
		t.Fatalf("failure filter: %+v", got)
	}
	got = filterPreviewActivities(activities, "notify")
	if len(got) != 1 || got[0].Type != "NotifyUser" {
		t.Fatalf("queue filter: %+v", got)
	}
	if n := len(filterPreviewActivities(activities, "")); n != 3 {
		t.Fatalf("empty query should keep all, got %d", n)
	}
}

func TestPreviewActivitySearchFiltersTable(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.previewKind = previewActivities
	wl.previewActivities = []previewActivity{
		{ScheduledID: 1, Type: "ValidateOrder", Status: "Completed"},
		{ScheduledID: 5, Type: "ChargeCard", Status: "Failed"},
	}
	wl.applyPreviewActivitySearch("charge")
	visible := wl.visiblePreviewActivities()
	if len(visible) != 1 || visible[0].ScheduledID != 5 {
		t.Fatalf("visible: %+v", visible)
	}
}

func TestPreviewActivityHintsIncludeSearch(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.setPreviewKind(previewActivities)
	wl.setFocusPane(focusEvents)
	if desc := hintDescription(wl.Hints(), "/"); desc != "Search" {
		t.Fatalf("activities search hint: %q", desc)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '/', 0)); ev != nil {
		t.Fatal("/ should open activity search")
	}
}

func TestPreviewEventSearchFiltersTable(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.previewKind = previewEvents
	wl.previewEvents = []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted"},
		{ID: 5, Type: "ActivityTaskScheduled", ActivityType: "Charge"},
	}
	wl.applyPreviewEventSearch("charge")
	visible := wl.visiblePreviewEvents()
	if len(visible) != 1 || visible[0].ID != 5 {
		t.Fatalf("visible: %+v", visible)
	}
}

func TestPreviewEventHintsOmitEventModal(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.setPreviewKind(previewEvents)
	wl.setFocusPane(focusEvents)
	if desc := hintDescription(wl.Hints(), "d"); desc != "" && desc != "Delete" {
		t.Fatalf("preview should not hint event modal, got %q", desc)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'd', 0)); ev == nil {
		t.Fatal("d should not open an event modal from preview")
	}
	if desc := hintDescription(wl.Hints(), "/"); desc != "Search" {
		t.Fatalf("events search hint: %q", desc)
	}
}

func TestListHintsIncludeWorkflowActions(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	running := false
	for i, w := range wl.workflows {
		if w.Status == "Running" {
			wl.table.SelectRow(i)
			running = true
			break
		}
	}
	if !running {
		t.Fatal("expected a running mock workflow")
	}
	if desc := hintDescription(wl.Hints(), "c"); desc != "Cancel" {
		t.Fatalf("list cancel hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "s"); desc != "Signal" {
		t.Fatalf("list signal hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "Q"); desc != "Query" {
		t.Fatalf("list query hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "D"); desc != "Delete" {
		t.Fatalf("list delete hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "/"); desc != "Search" {
		t.Fatalf("list search hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "F"); desc != "" {
		t.Fatalf("filter builder should not be a root key, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "f"); desc != "Filters" {
		t.Fatalf("list filters hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "d"); desc != "" {
		t.Fatalf("date range should be gone, got %q", desc)
	}

	resettable := false
	for i, w := range wl.workflows {
		if workflowCanReset(w.Status) {
			wl.table.SelectRow(i)
			resettable = true
			break
		}
	}
	if !resettable {
		t.Fatal("expected a resettable mock workflow")
	}
	if desc := hintDescription(wl.Hints(), "R"); desc != "Reset" {
		t.Fatalf("list reset hint: %q", desc)
	}
}

func TestPreviewOmitsWorkflowActions(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	for i, w := range wl.workflows {
		if w.Status == "Running" {
			wl.table.SelectRow(i)
			break
		}
	}
	wl.togglePreviewMode()
	wl.setPreviewKind(previewEvents)
	wl.setFocusPane(focusEvents)
	for _, key := range []string{"c", "X", "s", "Q", "R", "D", "N"} {
		if desc := hintDescription(wl.Hints(), key); desc != "" {
			t.Fatalf("preview should not hint %s, got %q", key, desc)
		}
	}
	for _, r := range []rune{'c', 'X', 's', 'Q', 'R', 'D', 'N'} {
		if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, r, 0)); ev == nil {
			t.Fatalf("%c should not run a workflow action from preview", r)
		}
	}
}
