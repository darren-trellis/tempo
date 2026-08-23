package temporal

import "testing"

func TestGetActivityStatus(t *testing.T) {
	if GetActivityStatus("Scheduled") != StatusScheduled {
		t.Fatal("Scheduled should use the pending activity status")
	}
	if GetActivityStatus("Completed") != StatusCompleted {
		t.Fatal("Completed should match workflow status")
	}
	if StatusScheduled.Icon() == "" {
		t.Fatal("scheduled status should have an icon")
	}
}
