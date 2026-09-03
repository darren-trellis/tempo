package temporal

import (
	"testing"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
)

func TestCountGroupByQuery(t *testing.T) {
	if got := countGroupByQuery(""); got != "GROUP BY ExecutionStatus" {
		t.Fatalf("empty query: %q", got)
	}
	if got := countGroupByQuery("WorkflowType='Order'"); got != "WorkflowType='Order' GROUP BY ExecutionStatus" {
		t.Fatalf("user query: %q", got)
	}
}

func TestCountStatusQuery(t *testing.T) {
	if got := countStatusQuery("", "Running"); got != `ExecutionStatus="Running"` {
		t.Fatalf("empty query: %q", got)
	}
	if got := countStatusQuery("WorkflowType='Order'", "Failed"); got != `(WorkflowType='Order') AND ExecutionStatus="Failed"` {
		t.Fatalf("user query: %q", got)
	}
}

func TestWorkflowCountsFromGroups(t *testing.T) {
	dc := converter.GetDefaultDataConverter()
	payload := func(v any) *commonpb.Payload {
		p, err := dc.ToPayload(v)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	got := workflowCountsFromGroups(&workflowservice.CountWorkflowExecutionsResponse{
		Count: 16,
		Groups: []*workflowservice.CountWorkflowExecutionsResponse_AggregationGroup{
			{GroupValues: []*commonpb.Payload{payload("Running")}, Count: 4},
			{GroupValues: []*commonpb.Payload{payload("Completed")}, Count: 6},
			{GroupValues: []*commonpb.Payload{payload("Failed")}, Count: 1},
			{GroupValues: []*commonpb.Payload{payload("Canceled")}, Count: 2},
			{GroupValues: []*commonpb.Payload{payload("Terminated")}, Count: 2},
			{GroupValues: []*commonpb.Payload{payload("ContinuedAsNew")}, Count: 1},
		},
	})
	if got.Running != 4 || got.Completed != 7 || got.Failed != 1 || got.Canceled != 2 || got.Terminated != 2 {
		t.Fatalf("counts=%+v", got)
	}
	if got.Total != 16 {
		t.Fatalf("total=%d", got.Total)
	}
}

func TestNormalizeCountStatus(t *testing.T) {
	if normalizeCountStatus("2") != "Completed" {
		t.Fatal("numeric status should map through the proto enum")
	}
	if normalizeCountStatus("RUNNING") != "Running" {
		t.Fatal("status names should be case-insensitive")
	}
}
