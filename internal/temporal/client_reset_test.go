package temporal

import (
	"testing"
)

func TestResetWorkflowRequestSetsRequestID(t *testing.T) {
	req := resetWorkflowRequest("ns", "wf", "run", 12, "because")
	if req.GetRequestId() == "" {
		t.Fatal("request id must be set")
	}
	if req.GetNamespace() != "ns" {
		t.Fatalf("namespace=%q", req.GetNamespace())
	}
	if req.GetWorkflowTaskFinishEventId() != 12 {
		t.Fatalf("event id=%d", req.GetWorkflowTaskFinishEventId())
	}
	if req.GetReason() != "because" {
		t.Fatalf("reason=%q", req.GetReason())
	}
	exec := req.GetWorkflowExecution()
	if exec == nil || exec.GetWorkflowId() != "wf" || exec.GetRunId() != "run" {
		t.Fatalf("execution=%+v", exec)
	}

	other := resetWorkflowRequest("ns", "wf", "run", 12, "because")
	if other.GetRequestId() == req.GetRequestId() {
		t.Fatal("each reset request needs a unique request id")
	}
}
