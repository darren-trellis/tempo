package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestPreviewActivitiesFromEvents(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)
	events := []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "ValidateOrder", ActivityID: "1", TaskQueue: "orders", Input: `{"id":1}`},
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
	if got[0].Type != "ValidateOrder" || got[0].Status != "Completed" || got[0].Result != `{"ok":true}` {
		t.Fatalf("first activity: %+v", got[0])
	}
	if got[0].Attempt != 1 || got[0].Identity != "worker-1" {
		t.Fatalf("first attempt/identity: %+v", got[0])
	}
	if got[1].Type != "Charge" || got[1].Status != "Failed" || got[1].Failure != "timeout" || got[1].Attempt != 2 {
		t.Fatalf("second activity: %+v", got[1])
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
	if desc := hintDescription(wl.Hints(), "enter"); desc != "Activities" {
		t.Fatalf("enter hint: got %q", desc)
	}

	wl.cyclePreviewKind(1)
	if wl.previewKind != previewEvents || wl.previewTabs.GetActive() != int(previewEvents) {
		t.Fatalf("next kind: %d tab %d", wl.previewKind, wl.previewTabs.GetActive())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "Events" {
		t.Fatalf("events enter hint: got %q", desc)
	}

	wl.cyclePreviewKind(1)
	if wl.previewKind != previewDetails || wl.previewTabs.GetActive() != int(previewDetails) {
		t.Fatalf("next kind: %d tab %d", wl.previewKind, wl.previewTabs.GetActive())
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
}
