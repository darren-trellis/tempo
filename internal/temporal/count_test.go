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
		Count: 19,
		Groups: []*workflowservice.CountWorkflowExecutionsResponse_AggregationGroup{
			{GroupValues: []*commonpb.Payload{payload("Running")}, Count: 4},
			{GroupValues: []*commonpb.Payload{payload("Completed")}, Count: 6},
			{GroupValues: []*commonpb.Payload{payload("Failed")}, Count: 1},
			{GroupValues: []*commonpb.Payload{payload("Canceled")}, Count: 2},
			{GroupValues: []*commonpb.Payload{payload("Terminated")}, Count: 2},
			{GroupValues: []*commonpb.Payload{payload("TimedOut")}, Count: 3},
			{GroupValues: []*commonpb.Payload{payload("ContinuedAsNew")}, Count: 1},
		},
	})
	if got.Running != 4 || got.Completed != 6 || got.Failed != 1 || got.Canceled != 2 || got.Terminated != 2 || got.TimedOut != 3 || got.ContinuedAsNew != 1 {
		t.Fatalf("counts=%+v", got)
	}
	if got.Total != 19 {
		t.Fatalf("total=%d", got.Total)
	}
}

func TestVisibilityGroupValues(t *testing.T) {
	dc := converter.GetDefaultDataConverter()
	payload := func(v any) *commonpb.Payload {
		p, err := dc.ToPayload(v)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	got := visibilityGroupValues(&workflowservice.CountWorkflowExecutionsResponse{
		Groups: []*workflowservice.CountWorkflowExecutionsResponse_AggregationGroup{
			{GroupValues: []*commonpb.Payload{payload("OrderWorkflow")}},
			{GroupValues: []*commonpb.Payload{payload(" PaymentWorkflow ")}},
			{GroupValues: []*commonpb.Payload{payload("")}},
		},
	})
	if len(got) != 2 || got[0] != "OrderWorkflow" || got[1] != "PaymentWorkflow" {
		t.Fatalf("groups=%v", got)
	}
	if visibilityGroupByQuery("WorkflowType") != "GROUP BY WorkflowType" {
		t.Fatal("group-by query")
	}
}

func TestNormalizeCountStatus(t *testing.T) {
	if normalizeCountStatus("2") != "Completed" {
		t.Fatal("numeric status should map through the proto enum")
	}
	if normalizeCountStatus("RUNNING") != "Running" {
		t.Fatal("status names should be case-insensitive")
	}
	if normalizeCountStatus("timed_out") != "TimedOut" {
		t.Fatal("timed out should stay distinct")
	}
	if normalizeCountStatus("ContinuedAsNew") != "ContinuedAsNew" {
		t.Fatal("continued as new should stay distinct")
	}
	if normalizeCountStatus("6") != "ContinuedAsNew" {
		t.Fatal("numeric continued-as-new should stay distinct")
	}
	if normalizeCountStatus("7") != "TimedOut" {
		t.Fatal("numeric timed-out should stay distinct")
	}
}
