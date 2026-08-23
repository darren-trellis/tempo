package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestSelectByScheduledID(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end1 := start.Add(2 * time.Second)
	end2 := start.Add(5 * time.Second)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: end1, ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: start.Add(3 * time.Second), ActivityType: "Second"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: end2, ScheduledEventID: 8},
	}))
	if tv.LaneCount() != 2 {
		t.Fatalf("lanes=%d", tv.LaneCount())
	}
	if !tv.SelectByScheduledID(8) {
		t.Fatal("should select second activity")
	}
	lane := tv.SelectedLane()
	if lane == nil || timelineLaneScheduledID(*lane) != 8 {
		t.Fatal("selected lane should be the second activity")
	}
}
