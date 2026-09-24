package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestFormatJSONPrettyFormatsCommaSeparatedPayloads(t *testing.T) {
	raw := `{"asset_id":null,"customer_id":"cust_2xwKbQfhYpBntVUxzQWN2xmSriK","description":"\"Notes\" updated from \"9/17/26 Started auth request on HPN portal. Pending Ref #W13382162. Patient has met DED has not met OOP. -AM\" to \"9/21/26 Auth has been approved called patient to go over financials. Left VM for her to call me back. -AM\n9/17/26 Started auth request on HPN portal. Pending Ref #W13382162. Patient has met DED has not met OOP. -AM\"","entity_id":"entity_2xwRPZow77UBcMKjHEXhBCfmMSx","event_filter_metadata":null,"event_metadata":null,"event_name":"row_updated","event_source":"api","field_id":"ef_2xxffk8QvNtEgnDAhLGh1NZVhF0","increment_id":0,"new_value":{"value":"9/21/26 Auth has been approved called patient to go over financials. Left VM for her to call me back. -AM\n9/17/26 Started auth request on HPN portal. Pending Ref #W13382162. Patient has met DED has not met OOP. -AM"},"old_value":{"value":"9/17/26 Started auth request on HPN portal. Pending Ref #W13382162. Patient has met DED has not met OOP. -AM"},"parent_row_id":null,"project_id":"proj_2xwQcbGjXmIy7QMbyIibhEHhdg4","row_id":"row_3JNXgEobLAZLJoyVAyoXTONfOpj","title":"Orders updated","transform_id":null,"user_id":"user_2zvF6WG7miwkRhR5zXMP2vyHmwo"}, false`
	got := formatJSONPretty(raw)
	if got == raw {
		t.Fatal("comma-separated Temporal payloads should be pretty-printed")
	}
	if !strings.Contains(got, "\n  \"asset_id\": null") {
		t.Fatalf("object should be indented, got %q", got)
	}
	if !strings.Contains(got, "\n  \"new_value\": {") {
		t.Fatalf("nested object should be indented, got %q", got)
	}
	if !strings.HasSuffix(got, ",\nfalse") {
		t.Fatalf("trailing boolean payload should stay after the object, got %q", got)
	}
}

func TestFormatJSONPrettyLeavesPlainText(t *testing.T) {
	raw := "not json, false"
	if got := formatJSONPretty(raw); got != raw {
		t.Fatalf("plain text should be unchanged, got %q", got)
	}
}

func TestWorkflowInfoRowsIncludeFullValues(t *testing.T) {
	parent := "parent-workflow"
	runID := "run-abcdefghijklmnopqrstuvwxyz-full"
	rows := workflowInfoRows(time.Now(), temporal.Workflow{
		ID:        "child-workflow",
		RunID:     runID,
		Type:      "ChildType",
		Status:    "Running",
		TaskQueue: "very-long-task-queue-name",
		StartTime: time.Now(),
		ParentID:  &parent,
	})
	if idx := workflowInfoRowIndex(rows, workflowInfoParent); idx < 0 || rows[idx].Value != parent {
		t.Fatalf("parent row: %+v", rows)
	}
	if idx := workflowInfoRowIndex(rows, workflowInfoRunID); idx < 0 || rows[idx].Value != runID {
		t.Fatalf("run id row: %+v", rows)
	}
	if got := workflowInfoContentWidth(rows); got < len("Task Queue")+1+len("very-long-task-queue-name") {
		t.Fatalf("content width too small: %d", got)
	}
}

func TestWorkflowInfoRowsMarkUnhandledFailure(t *testing.T) {
	rows := workflowInfoRows(time.Now(), temporal.Workflow{
		ID:          "wf",
		Status:      "Running",
		TaskFailure: true,
		StartTime:   time.Now(),
	})
	idx := workflowInfoRowIndex(rows, workflowInfoStatus)
	if idx < 0 {
		t.Fatal("missing status row")
	}
	if rows[idx].Value != "Unhandled Failure" {
		t.Fatalf("value=%q", rows[idx].Value)
	}
	if !strings.Contains(rows[idx].Display, "Unhandled Failure") {
		t.Fatalf("display=%q", rows[idx].Display)
	}
}
