package temporal

import (
	"encoding/json"
	"testing"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	failurepb "go.temporal.io/api/failure/v1"
	historypb "go.temporal.io/api/history/v1"
)

func TestWorkflowFailedEventCarriesTheWholeFailure(t *testing.T) {
	event := &historypb.HistoryEvent{
		EventId:   19,
		EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED,
		Attributes: &historypb.HistoryEvent_WorkflowExecutionFailedEventAttributes{
			WorkflowExecutionFailedEventAttributes: &historypb.WorkflowExecutionFailedEventAttributes{
				RetryState:                   enums.RETRY_STATE_RETRY_POLICY_NOT_SET,
				WorkflowTaskCompletedEventId: 18,
				Failure: &failurepb.Failure{
					Message: "Activity task failed",
					Cause: &failurepb.Failure{
						Message:    "Encoded failure",
						Source:     "TypeScriptSDK",
						StackTrace: "",
						EncodedAttributes: &commonpb.Payload{
							Metadata: map[string][]byte{"encoding": []byte("json/plain")},
							Data:     []byte(`{"message":"Missing required fields","stack_trace":"ApplicationFailure: Missing required fields"}`),
						},
						FailureInfo: &failurepb.Failure_ApplicationFailureInfo{
							ApplicationFailureInfo: &failurepb.ApplicationFailureInfo{
								Type:         "MissingFields",
								NonRetryable: true,
								Details: &commonpb.Payloads{Payloads: []*commonpb.Payload{{
									Metadata: map[string][]byte{"encoding": []byte("json/plain")},
									Data:     []byte(`{"fields":["npi"]}`),
								}}},
							},
						},
					},
					FailureInfo: &failurepb.Failure_ActivityFailureInfo{
						ActivityFailureInfo: &failurepb.ActivityFailureInfo{
							ScheduledEventId: 13,
							StartedEventId:   14,
							ActivityType:     &commonpb.ActivityType{Name: "buildRequestActivity"},
							ActivityId:       "2",
							RetryState:       enums.RETRY_STATE_NON_RETRYABLE_FAILURE,
						},
					},
				},
			},
		},
	}

	he := extractEnhancedEvent(event)

	if he.Failure != "Activity task failed" {
		t.Fatalf("Failure = %q", he.Failure)
	}
	var got struct {
		Failure struct {
			Message string `json:"message"`
			Cause   struct {
				Message                string `json:"message"`
				StackTrace             string `json:"stackTrace"`
				EncodedAttributes      any    `json:"encodedAttributes"`
				ApplicationFailureInfo struct {
					Type         string `json:"type"`
					NonRetryable bool   `json:"nonRetryable"`
					Details      struct {
						Payloads []map[string][]string `json:"payloads"`
					} `json:"details"`
				} `json:"applicationFailureInfo"`
			} `json:"cause"`
			ActivityFailureInfo struct {
				ScheduledEventID string `json:"scheduledEventId"`
				ActivityType     struct {
					Name string `json:"name"`
				} `json:"activityType"`
				RetryState string `json:"retryState"`
			} `json:"activityFailureInfo"`
		} `json:"failure"`
		RetryState                   string `json:"retryState"`
		WorkflowTaskCompletedEventID string `json:"workflowTaskCompletedEventId"`
	}
	if err := json.Unmarshal([]byte(he.FailureJSON), &got); err != nil {
		t.Fatalf("FailureJSON is not JSON: %v\n%s", err, he.FailureJSON)
	}
	cause := got.Failure.Cause
	if cause.Message != "Missing required fields" || cause.StackTrace != "ApplicationFailure: Missing required fields" || cause.EncodedAttributes != nil {
		t.Fatalf("cause was not decoded: %s", he.FailureJSON)
	}
	if cause.ApplicationFailureInfo.Type != "MissingFields" || !cause.ApplicationFailureInfo.NonRetryable {
		t.Fatalf("application failure info missing: %s", he.FailureJSON)
	}
	if details := cause.ApplicationFailureInfo.Details.Payloads; len(details) != 1 || details[0]["fields"][0] != "npi" {
		t.Fatalf("details payload was not decoded: %s", he.FailureJSON)
	}
	activity := got.Failure.ActivityFailureInfo
	if activity.ScheduledEventID != "13" || activity.ActivityType.Name != "buildRequestActivity" || activity.RetryState != "RETRY_STATE_NON_RETRYABLE_FAILURE" {
		t.Fatalf("activity failure info missing: %s", he.FailureJSON)
	}
	if got.RetryState != "RETRY_STATE_RETRY_POLICY_NOT_SET" || got.WorkflowTaskCompletedEventID != "18" {
		t.Fatalf("event attributes missing: %s", he.FailureJSON)
	}
	if event.GetWorkflowExecutionFailedEventAttributes().GetFailure().GetCause().GetEncodedAttributes() == nil {
		t.Fatal("decoding mutated the history event")
	}
}
