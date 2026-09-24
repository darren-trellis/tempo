package temporal

import (
	"reflect"
	"testing"
	"time"

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

func TestWorkflowFromExecutionInfoKeepsCustomSearchAttributes(t *testing.T) {
	dc := converter.GetDefaultDataConverter()
	customer, err := dc.ToPayload("acme")
	if err != nil {
		t.Fatal(err)
	}
	amount, err := dc.ToPayload(int64(42))
	if err != nil {
		t.Fatal(err)
	}
	tags, err := dc.ToPayload([]string{"gold", "vip"})
	if err != nil {
		t.Fatal(err)
	}
	flag, err := dc.ToPayload(true)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 3, 2, 15, 4, 5, 0, time.UTC)
	closed, err := dc.ToPayload(when)
	if err != nil {
		t.Fatal(err)
	}
	problems, err := dc.ToPayload([]string{reportedProblemTaskFailed})
	if err != nil {
		t.Fatal(err)
	}
	info := &workflowpb.WorkflowExecutionInfo{
		Execution: &commonpb.WorkflowExecution{WorkflowId: "wf", RunId: "run"},
		Status:    enums.WORKFLOW_EXECUTION_STATUS_COMPLETED,
		SearchAttributes: &commonpb.SearchAttributes{
			IndexedFields: map[string]*commonpb.Payload{
				"CustomerId":                 customer,
				"Amount":                     amount,
				"Tags":                       tags,
				"Active":                     flag,
				"ClosedAt":                   closed,
				temporalReportedProblemsAttr: problems,
				"WorkflowId":                 customer,
			},
		},
	}
	wf := workflowFromExecutionInfo(info, "default")
	if wf.SearchAttributes["CustomerId"] != "acme" {
		t.Fatalf("CustomerId=%q", wf.SearchAttributes["CustomerId"])
	}
	if wf.SearchAttributes["Amount"] != "42" {
		t.Fatalf("Amount=%q", wf.SearchAttributes["Amount"])
	}
	if wf.SearchAttributes["Tags"] != "gold, vip" {
		t.Fatalf("Tags=%q", wf.SearchAttributes["Tags"])
	}
	if wf.SearchAttributes["Active"] != "true" {
		t.Fatalf("Active=%q", wf.SearchAttributes["Active"])
	}
	if wf.SearchAttributes["ClosedAt"] != when.Format(time.RFC3339Nano) {
		t.Fatalf("ClosedAt=%q", wf.SearchAttributes["ClosedAt"])
	}
	if _, ok := wf.SearchAttributes[temporalReportedProblemsAttr]; ok {
		t.Fatal("system search attributes should not become columns")
	}
	if _, ok := wf.SearchAttributes["WorkflowId"]; ok {
		t.Fatal("built-in search attributes should not become columns")
	}
}

func TestSearchAttributeValuesKeepTheirTypes(t *testing.T) {
	dc := converter.GetDefaultDataConverter()
	typed := func(value any, typ string) *commonpb.Payload {
		payload, err := dc.ToPayload(value)
		if err != nil {
			t.Fatal(err)
		}
		if typ != "" {
			payload.Metadata["type"] = []byte(typ)
		}
		return payload
	}
	when := time.Date(2026, 3, 2, 15, 4, 5, 0, time.UTC)
	info := &workflowpb.WorkflowExecutionInfo{
		Execution: &commonpb.WorkflowExecution{WorkflowId: "wf", RunId: "run"},
		SearchAttributes: &commonpb.SearchAttributes{
			IndexedFields: map[string]*commonpb.Payload{
				"CustomerId": typed("acme", "Keyword"),
				"Amount":     typed(int64(42), "Int"),
				"Score":      typed(1.5, "Double"),
				"Tags":       typed([]string{"gold", "vip"}, "KeywordList"),
				"Active":     typed(true, "Bool"),
				"ClosedAt":   typed(when, "Datetime"),
				"Untyped":    typed(int64(7), ""),
			},
		},
	}
	got := workflowFromExecutionInfo(info, "default").SearchAttributeValues
	want := map[string]any{
		"CustomerId": "acme",
		"Amount":     42,
		"Score":      1.5,
		"Tags":       []any{"gold", "vip"},
		"Active":     true,
		"ClosedAt":   when,
		"Untyped":    float64(7),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
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
