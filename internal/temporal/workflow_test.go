package temporal

import (
	"testing"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/sdk/converter"
)

func TestExecutionHasTaskFailure(t *testing.T) {
	dc := converter.GetDefaultDataConverter()
	payload, err := dc.ToPayload([]string{reportedProblemTaskFailed})
	if err != nil {
		t.Fatal(err)
	}
	info := &workflowpb.WorkflowExecutionInfo{
		Status: enums.WORKFLOW_EXECUTION_STATUS_RUNNING,
		SearchAttributes: &commonpb.SearchAttributes{
			IndexedFields: map[string]*commonpb.Payload{
				temporalReportedProblemsAttr: payload,
			},
		},
	}
	if !executionHasTaskFailure(info) {
		t.Fatal("running workflow with WorkflowTaskFailed should be marked")
	}

	info.Status = enums.WORKFLOW_EXECUTION_STATUS_COMPLETED
	if executionHasTaskFailure(info) {
		t.Fatal("closed workflows should not be marked")
	}

	other, err := dc.ToPayload([]string{"category=SomeOtherProblem"})
	if err != nil {
		t.Fatal(err)
	}
	info.Status = enums.WORKFLOW_EXECUTION_STATUS_RUNNING
	info.SearchAttributes.IndexedFields[temporalReportedProblemsAttr] = other
	if executionHasTaskFailure(info) {
		t.Fatal("unrelated reported problems should not mark task failure")
	}
}

func TestWorkflowFromExecutionInfoMarksTaskFailure(t *testing.T) {
	dc := converter.GetDefaultDataConverter()
	payload, err := dc.ToPayload([]string{reportedProblemTaskTimedOut})
	if err != nil {
		t.Fatal(err)
	}
	info := &workflowpb.WorkflowExecutionInfo{
		Execution: &commonpb.WorkflowExecution{WorkflowId: "wf", RunId: "run"},
		Type:      &commonpb.WorkflowType{Name: "Order"},
		Status:    enums.WORKFLOW_EXECUTION_STATUS_RUNNING,
		TaskQueue: "orders",
		SearchAttributes: &commonpb.SearchAttributes{
			IndexedFields: map[string]*commonpb.Payload{
				temporalReportedProblemsAttr: payload,
			},
		},
	}
	wf := workflowFromExecutionInfo(info, "default")
	if wf.ID != "wf" || wf.Type != "Order" || wf.Status != "Running" || !wf.TaskFailure {
		t.Fatalf("workflow: %+v", wf)
	}
	label, status := WorkflowDisplayStatus(wf)
	if label != "Unhandled Failure" || status != StatusUnhandledFailure {
		t.Fatalf("display %q status=%v", label, status)
	}
}

func TestExecutionHasTaskFailureCloudKeywordList(t *testing.T) {
	info := &workflowpb.WorkflowExecutionInfo{
		Status: enums.WORKFLOW_EXECUTION_STATUS_RUNNING,
		SearchAttributes: &commonpb.SearchAttributes{
			IndexedFields: map[string]*commonpb.Payload{
				temporalReportedProblemsAttr: {
					Metadata: map[string][]byte{
						"encoding": []byte("json/plain"),
						"type":     []byte("KeywordList"),
					},
					Data: []byte(`["category=WorkflowTaskFailed","cause=WorkflowTaskFailedCauseWorkflowWorkerUnhandledFailure"]`),
				},
			},
		},
	}
	if !executionHasTaskFailure(info) {
		t.Fatal("cloud TemporalReportedProblems payload should mark unhandled failure")
	}
}

func TestHistoryHasTaskFailure(t *testing.T) {
	if !HistoryHasTaskFailure([]EnhancedHistoryEvent{
		{Type: "WorkflowTaskCompleted"},
		{Type: "WorkflowTaskFailed"},
	}) {
		t.Fatal("last failed workflow task should count")
	}
	if !HistoryHasTaskFailure([]EnhancedHistoryEvent{
		{Type: "WorkflowTaskCompleted"},
		{Type: "WorkflowTaskTimedOut"},
	}) {
		t.Fatal("last timed out workflow task should count")
	}
	if HistoryHasTaskFailure([]EnhancedHistoryEvent{
		{Type: "WorkflowTaskFailed"},
		{Type: "WorkflowTaskCompleted"},
	}) {
		t.Fatal("recovered workflow task should not count")
	}
	if HistoryHasTaskFailure(nil) {
		t.Fatal("empty history should not count")
	}
}
