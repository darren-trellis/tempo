package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestPreviewActivitiesFromEvents(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)
	events := []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "ValidateOrder", ActivityID: "1", TaskQueue: "orders", Details: `ActivityType: ValidateOrder, Input: {"id":1}`},
		{ID: 6, Type: "ActivityTaskStarted", Time: start.Add(time.Second), ScheduledEventID: 5, Attempt: 1, Identity: "worker-1"},
		{ID: 7, Type: "ActivityTaskCompleted", Time: end, ScheduledEventID: 5, Result: `{"ok":true}`},
		{ID: 8, Type: "ActivityTaskScheduled", Time: start.Add(3 * time.Second), ActivityType: "Charge", ActivityID: "2"},
		{ID: 9, Type: "ActivityTaskStarted", Time: start.Add(4 * time.Second), ScheduledEventID: 8, Attempt: 2},
		{ID: 10, Type: "ActivityTaskFailed", Time: start.Add(5 * time.Second), ScheduledEventID: 8, Failure: "timeout"},
	}

	got := previewActivitiesFromEvents(events)
	if len(got) != 2 {
		t.Fatalf("got %d activities", len(got))
	}
	if got[0].Type != "ValidateOrder" || got[0].Status != "Completed" || got[0].Input != `{"id":1}` || got[0].Result != `{"ok":true}` {
		t.Fatalf("first activity: %+v", got[0])
	}
	if got[0].Attempt != 1 || got[0].Identity != "worker-1" {
		t.Fatalf("first attempt/identity: %+v", got[0])
	}
	if got[1].Type != "Charge" || got[1].Status != "Failed" || got[1].Failure != "timeout" || got[1].Attempt != 2 {
		t.Fatalf("second activity: %+v", got[1])
	}
}

func TestActivityInfoRowsOmitsPayloads(t *testing.T) {
	end := time.Date(2026, 8, 23, 12, 0, 2, 0, time.UTC)
	a := previewActivity{
		Type:        "ValidateOrder",
		ActivityID:  "1",
		Status:      "Completed",
		StartTime:   time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		EndTime:     &end,
		TaskQueue:   "orders",
		Identity:    "worker-1",
		Input:       `{"id":1}`,
		Result:      `{"ok":true}`,
		Failure:     "should not appear",
		ScheduledID: 5,
	}
	rows := activityInfoRows(a)
	if workflowInfoRowIndex(rows, activityInfoName) < 0 || workflowInfoRowIndex(rows, activityInfoTaskQueue) < 0 {
		t.Fatalf("details should keep metadata: %+v", rows)
	}
	for _, key := range []string{"Input", "Result", "Failure"} {
		if workflowInfoRowIndex(rows, key) >= 0 {
			t.Fatalf("details should omit %q: %+v", key, rows)
		}
	}
	if in := formatActivityInput(a); !strings.Contains(in, `"id"`) {
		t.Fatalf("input tab: %s", in)
	}
	if out := formatActivityOutput(a); !strings.Contains(out, `"ok"`) {
		t.Fatalf("output tab: %s", out)
	}
	failed := a
	failed.Result = ""
	if out := formatActivityOutput(failed); !strings.Contains(out, "should not appear") {
		t.Fatalf("failed output should show failure: %s", out)
	}
}

func TestActivityDetailRendersTable(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	end := time.Date(2026, 8, 23, 12, 0, 2, 0, time.UTC)
	wl.previewActivities = []previewActivity{{
		Type:      "ValidateOrder",
		Status:    "Completed",
		StartTime: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		EndTime:   &end,
		TaskQueue: "orders",
		Input:     `{"id":1}`,
		Result:    `{"ok":true}`,
	}}
	wl.renderSelectedActivityDetail()
	if len(wl.activityDetailRows) == 0 {
		t.Fatal("details tab should populate the table")
	}
	if workflowInfoRowIndex(wl.activityDetailRows, activityInfoName) < 0 {
		t.Fatalf("missing activity row: %+v", wl.activityDetailRows)
	}
	if workflowInfoRowIndex(wl.activityDetailRows, "Input") >= 0 {
		t.Fatal("table should not include payloads")
	}

	wl.focusPane = focusEventDetail
	if desc := hintDescription(wl.Hints(), "y"); desc != "Yank" {
		t.Fatalf("details table should show yank, got %q", desc)
	}
	if ev := wl.handleActivityDetailKeys(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ev != nil {
		t.Fatal("] should still switch activity detail tabs from the table")
	}
	if wl.activityDetailKind != activityDetailInput {
		t.Fatalf("] should go to input, got %d", wl.activityDetailKind)
	}
	if desc := hintDescription(wl.Hints(), "y"); desc != "" {
		t.Fatalf("input tab should not show yank, got %q", desc)
	}
}

func TestActivityDetailTabsDefault(t *testing.T) {
	if activityDetailDetails.title() != "Details" || activityDetailInput.title() != "Input" || activityDetailOutput.title() != "Output" {
		t.Fatal("unexpected activity detail tab titles")
	}
	wl := NewWorkflowList(&App{}, "default")
	if wl.activityDetailTabs == nil {
		t.Fatal("expected activity detail tabs")
	}
	if wl.activityDetailTabs.GetActive() != int(activityDetailDetails) {
		t.Fatalf("default tab: %d", wl.activityDetailTabs.GetActive())
	}
}

func TestActivityDetailTabKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.previewKind = previewActivities
	wl.focusPane = focusEventDetail
	wl.previewActivities = []previewActivity{{
		Type:   "ValidateOrder",
		Status: "Completed",
		Input:  `{"id":1}`,
		Result: `{"ok":true}`,
	}}

	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ev != nil {
		t.Fatal("] should switch activity detail tabs")
	}
	if wl.activityDetailKind != activityDetailInput {
		t.Fatalf("] should go to input, got %d", wl.activityDetailKind)
	}
	if wl.previewKind != previewActivities {
		t.Fatal("] from activity details should not change the preview tab")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '3', 0)); ev != nil {
		t.Fatal("3 should select output")
	}
	if wl.activityDetailKind != activityDetailOutput {
		t.Fatalf("3 should select output, got %d", wl.activityDetailKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '1', 0)); ev != nil {
		t.Fatal("1 should select details")
	}
	if wl.activityDetailKind != activityDetailDetails {
		t.Fatalf("1 should select details, got %d", wl.activityDetailKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '[', 0)); ev != nil {
		t.Fatal("[ should cycle activity detail tabs")
	}
	if wl.activityDetailKind != activityDetailOutput {
		t.Fatalf("[ should wrap to output, got %d", wl.activityDetailKind)
	}
}

func TestPreviewTabKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.focusPane = focusEvents

	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'h', 0)); ev == nil {
		t.Fatal("h should not switch preview tabs")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)); ev == nil {
		t.Fatal("left should not switch preview tabs")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'l', 0)); ev == nil {
		t.Fatal("l should not switch preview tabs")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)); ev == nil {
		t.Fatal("right should not switch preview tabs")
	}

	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ev != nil {
		t.Fatal("] should switch preview tabs")
	}
	if wl.previewKind != previewEvents {
		t.Fatalf("] should go to events, got %d", wl.previewKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '1', 0)); ev != nil {
		t.Fatal("1 should select details")
	}
	if wl.previewKind != previewDetails {
		t.Fatalf("1 should select details, got %d", wl.previewKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '2', 0)); ev != nil {
		t.Fatal("2 should select activities")
	}
	if wl.previewKind != previewActivities {
		t.Fatalf("2 should select activities, got %d", wl.previewKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '3', 0)); ev != nil {
		t.Fatal("3 should select events")
	}
	if wl.previewKind != previewEvents {
		t.Fatalf("3 should select events, got %d", wl.previewKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '4', 0)); ev != nil {
		t.Fatal("4 should select hierarchy")
	}
	if wl.previewKind != previewHierarchy {
		t.Fatalf("4 should select hierarchy, got %d", wl.previewKind)
	}
}

func TestPreviewKindCycle(t *testing.T) {
	if previewActivities.title() != "Activities" {
		t.Fatalf("default kind title: %q", previewActivities.title())
	}
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	if wl.previewKind != previewActivities {
		t.Fatal("preview should default to activities")
	}
	if wl.previewTabs.GetActive() != int(previewActivities) {
		t.Fatalf("activities tab index: %d", wl.previewTabs.GetActive())
	}
	if wl.rightFlex.GetItemCount() != 2 {
		t.Fatalf("activities should show a sibling detail pane, got %d items", wl.rightFlex.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "" {
		t.Fatalf("enter should stay off the footer, got %q", desc)
	}

	wl.cyclePreviewKind(1)
	if wl.previewKind != previewEvents || wl.previewTabs.GetActive() != int(previewEvents) {
		t.Fatalf("next kind: %d tab %d", wl.previewKind, wl.previewTabs.GetActive())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "" {
		t.Fatalf("events enter should stay off the footer, got %q", desc)
	}

	wl.cyclePreviewKind(1)
	if wl.previewKind != previewHierarchy || wl.previewTabs.GetActive() != int(previewHierarchy) {
		t.Fatalf("next kind: %d tab %d", wl.previewKind, wl.previewTabs.GetActive())
	}
	if wl.rightFlex.GetItemCount() != 2 {
		t.Fatalf("hierarchy should show the graph pane, got %d items", wl.rightFlex.GetItemCount())
	}

	wl.cyclePreviewKind(1)
	if wl.previewKind != previewDetails || wl.previewTabs.GetActive() != int(previewDetails) {
		t.Fatalf("next kind: %d tab %d", wl.previewKind, wl.previewTabs.GetActive())
	}
	if wl.rightFlex.GetItemCount() != 1 {
		t.Fatalf("details should hide the sibling pane, got %d items", wl.rightFlex.GetItemCount())
	}

	wl.cyclePreviewKind(1)
	if wl.previewKind != previewActivities {
		t.Fatal("cycle should wrap to activities")
	}
}

func TestPreviewTabHit(t *testing.T) {
	if kind, ok := previewTabAtX(0, 1); !ok || kind != previewDetails {
		t.Fatalf("details tab: kind=%d ok=%v", kind, ok)
	}
	if kind, ok := previewTabAtX(0, previewTabWidth(previewDetails)+2); !ok || kind != previewActivities {
		t.Fatalf("activities tab: kind=%d ok=%v", kind, ok)
	}
	eventsX := previewTabWidth(previewDetails) + 1 + previewTabWidth(previewActivities) + 2
	if kind, ok := previewTabAtX(0, eventsX); !ok || kind != previewEvents {
		t.Fatalf("events tab: kind=%d ok=%v", kind, ok)
	}
	hierarchyX := eventsX + previewTabWidth(previewEvents) + 1
	if kind, ok := previewTabAtX(0, hierarchyX+2); !ok || kind != previewHierarchy {
		t.Fatalf("hierarchy tab: kind=%d ok=%v", kind, ok)
	}
}

func TestSyncFocusKeepsActivitiesPane(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.previewMode = true
	wl.focusPane = focusEvents
	wl.syncFocusFromPrimitives()
	if wl.focusPane != focusEvents {
		t.Fatalf("focus should stay on activities, got %d", wl.focusPane)
	}
}
