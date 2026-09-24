package temporal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

// ListWorkflows returns workflows for a namespace with optional filtering.
func (c *Client) ListWorkflows(ctx context.Context, namespace string, opts ListOptions) ([]Workflow, string, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, "", err
	}

	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	req := &workflowservice.ListWorkflowExecutionsRequest{
		Namespace:     namespace,
		PageSize:      int32(pageSize),
		NextPageToken: []byte(opts.PageToken),
	}

	if opts.Query != "" {
		req.Query = opts.Query
	}

	resp, err := cl.WorkflowService().ListWorkflowExecutions(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list workflows: %w", err)
	}

	decodePayloadsInMessages(c.payloadCodec(namespace), asProtoMessages(resp.GetExecutions())...)

	var workflows []Workflow
	for _, exec := range resp.GetExecutions() {
		workflows = append(workflows, workflowFromExecutionInfo(exec, namespace))
	}

	return workflows, string(resp.GetNextPageToken()), nil
}

// GetWorkflow returns details for a specific workflow execution.
func (c *Client) GetWorkflow(ctx context.Context, namespace, workflowID, runID string) (*Workflow, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}

	resp, err := cl.WorkflowService().DescribeWorkflowExecution(ctx, &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: namespace,
		Execution: &commonpb.WorkflowExecution{
			WorkflowId: workflowID,
			RunId:      runID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe workflow: %w", err)
	}

	wf := workflowFromExecutionInfo(resp.GetWorkflowExecutionInfo(), namespace)
	if !wf.TaskFailure && wf.Status == "Running" {
		if task := resp.GetPendingWorkflowTask(); task != nil && task.GetAttempt() > 1 {
			wf.TaskFailure = true
		}
	}

	// Note: Input/Output are populated separately from event history
	// to avoid redundant API calls. See workflow_detail.go loadData().

	return &wf, nil
}

// CancelWorkflow requests graceful cancellation of a workflow execution.
func (c *Client) CancelWorkflow(ctx context.Context, namespace, workflowID, runID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	return cl.CancelWorkflow(ctx, workflowID, runID)
}

// TerminateWorkflow forcefully terminates a workflow execution immediately.
func (c *Client) TerminateWorkflow(ctx context.Context, namespace, workflowID, runID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	return cl.TerminateWorkflow(ctx, workflowID, runID, reason)
}

// SignalWorkflow sends a signal to a running workflow execution.
func (c *Client) SignalWorkflow(ctx context.Context, namespace, workflowID, runID, signalName string, input []byte) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	return cl.SignalWorkflow(ctx, workflowID, runID, signalName, json.RawMessage(input))
}

// StartWorkflow starts a new workflow execution.
func (c *Client) StartWorkflow(ctx context.Context, namespace string, req StartWorkflowRequest) (string, error) {
	cl, err := c.conn()
	if err != nil {
		return "", err
	}
	opts := client.StartWorkflowOptions{
		ID:        req.WorkflowID,
		TaskQueue: req.TaskQueue,
	}

	args := []interface{}{}
	if len(req.Input) > 0 {
		args = append(args, json.RawMessage(req.Input))
	}

	run, err := cl.ExecuteWorkflow(ctx, opts, req.WorkflowType, args...)
	if err != nil {
		return "", fmt.Errorf("failed to start workflow: %w", err)
	}
	return run.GetRunID(), nil
}

// SignalWithStartWorkflow starts a workflow if it doesn't exist and sends a signal to it.
func (c *Client) SignalWithStartWorkflow(ctx context.Context, namespace string, req SignalWithStartRequest) (string, error) {
	cl, err := c.conn()
	if err != nil {
		return "", err
	}
	opts := client.StartWorkflowOptions{
		ID:        req.WorkflowID,
		TaskQueue: req.TaskQueue,
	}

	run, err := cl.SignalWithStartWorkflow(
		ctx,
		req.WorkflowID,
		req.SignalName,
		json.RawMessage(req.SignalInput),
		opts,
		req.WorkflowType,
		json.RawMessage(req.WorkflowInput),
	)
	if err != nil {
		return "", fmt.Errorf("failed to signal with start workflow: %w", err)
	}
	return run.GetRunID(), nil
}

// DeleteWorkflow permanently deletes a workflow execution and its history.
func (c *Client) DeleteWorkflow(ctx context.Context, namespace, workflowID, runID string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	_, err = cl.WorkflowService().DeleteWorkflowExecution(ctx,
		&workflowservice.DeleteWorkflowExecutionRequest{
			Namespace: namespace,
			WorkflowExecution: &commonpb.WorkflowExecution{
				WorkflowId: workflowID,
				RunId:      runID,
			},
		})
	return err
}

// ResetWorkflow resets a workflow to a previous state, creating a new run.
func (c *Client) ResetWorkflow(ctx context.Context, namespace, workflowID, runID string, eventID int64, reason string) (string, error) {
	cl, err := c.conn()
	if err != nil {
		return "", err
	}
	resp, err := cl.WorkflowService().ResetWorkflowExecution(ctx, resetWorkflowRequest(namespace, workflowID, runID, eventID, reason))
	if err != nil {
		return "", err
	}
	return resp.GetRunId(), nil
}

func resetWorkflowRequest(namespace, workflowID, runID string, eventID int64, reason string) *workflowservice.ResetWorkflowExecutionRequest {
	return &workflowservice.ResetWorkflowExecutionRequest{
		Namespace: namespace,
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: workflowID,
			RunId:      runID,
		},
		Reason:                    reason,
		WorkflowTaskFinishEventId: eventID,
		RequestId:                 uuid.NewString(),
	}
}

// QueryWorkflow executes a query against a running workflow and returns the result.
func (c *Client) QueryWorkflow(ctx context.Context, namespace, workflowID, runID, queryType string, args []byte) (*QueryResult, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	// Build query input if args provided
	var queryArgs interface{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &queryArgs); err != nil {
			// If not valid JSON, pass as raw string
			queryArgs = string(args)
		}
	}

	// Execute the query
	response, err := cl.QueryWorkflow(ctx, workflowID, runID, queryType, queryArgs)
	if err != nil {
		return &QueryResult{
			QueryType: queryType,
			Error:     err.Error(),
		}, nil
	}

	// Decode the result
	var result interface{}
	if err := response.Get(&result); err != nil {
		return &QueryResult{
			QueryType: queryType,
			Error:     fmt.Sprintf("failed to decode query result: %v", err),
		}, nil
	}

	// Format result as JSON for display
	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &QueryResult{
			QueryType: queryType,
			Result:    fmt.Sprintf("%v", result),
		}, nil
	}

	return &QueryResult{
		QueryType: queryType,
		Result:    string(resultJSON),
	}, nil
}

// CancelWorkflows cancels multiple workflows and returns results for each.
func (c *Client) CancelWorkflows(ctx context.Context, namespace string, workflows []WorkflowIdentifier) ([]BatchResult, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	results := make([]BatchResult, len(workflows))

	for i, wf := range workflows {
		err := cl.CancelWorkflow(ctx, wf.WorkflowID, wf.RunID)
		results[i] = BatchResult{
			WorkflowID: wf.WorkflowID,
			RunID:      wf.RunID,
			Success:    err == nil,
		}
		if err != nil {
			results[i].Error = err.Error()
		}
	}

	return results, nil
}

// TerminateWorkflows terminates multiple workflows and returns results for each.
func (c *Client) TerminateWorkflows(ctx context.Context, namespace string, workflows []WorkflowIdentifier, reason string) ([]BatchResult, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	results := make([]BatchResult, len(workflows))

	for i, wf := range workflows {
		err := cl.TerminateWorkflow(ctx, wf.WorkflowID, wf.RunID, reason)
		results[i] = BatchResult{
			WorkflowID: wf.WorkflowID,
			RunID:      wf.RunID,
			Success:    err == nil,
		}
		if err != nil {
			results[i].Error = err.Error()
		}
	}

	return results, nil
}

// GetResetPoints returns valid reset points for a workflow execution.
func (c *Client) GetResetPoints(ctx context.Context, namespace, workflowID, runID string) ([]ResetPoint, error) {
	// Get workflow history to find reset points
	events, err := c.GetEnhancedWorkflowHistory(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, err
	}

	var resetPoints []ResetPoint

	// Track activity/timer state for building descriptions
	activityInfo := make(map[int64]string) // scheduledEventID -> activity type
	timerInfo := make(map[int64]string)    // startedEventID -> timer ID

	for _, event := range events {
		// Track activity scheduled events
		if strings.Contains(event.Type, "ActivityTaskScheduled") {
			activityInfo[event.ID] = event.ActivityType
		}

		// Track timer started events
		if strings.Contains(event.Type, "TimerStarted") {
			timerInfo[event.ID] = event.TimerID
		}

		// WorkflowTaskCompleted events are valid reset points
		if strings.Contains(event.Type, "WorkflowTaskCompleted") {
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ID,
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Workflow task completed at event %d", event.ID),
				Reason:      "Reset to this workflow task",
			})
		}

		// ActivityTaskFailed - reset to before the failure
		if strings.Contains(event.Type, "ActivityTaskFailed") {
			actType := activityInfo[event.ScheduledEventID]
			if actType == "" {
				actType = "Unknown"
			}
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ScheduledEventID - 1, // Reset to workflow task before activity was scheduled
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Activity '%s' failed: %s", actType, truncateString(event.Failure, 50)),
				Reason:      "Reset to retry failed activity",
			})
		}

		// ActivityTaskTimedOut - reset to before the timeout
		if strings.Contains(event.Type, "ActivityTaskTimedOut") {
			actType := activityInfo[event.ScheduledEventID]
			if actType == "" {
				actType = "Unknown"
			}
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ScheduledEventID - 1,
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Activity '%s' timed out", actType),
				Reason:      "Reset to retry timed out activity",
			})
		}

		// WorkflowTaskFailed - this is a good reset point
		if strings.Contains(event.Type, "WorkflowTaskFailed") {
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ScheduledEventID - 1,
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Workflow task failed: %s", truncateString(event.Failure, 50)),
				Reason:      "Reset to retry failed workflow task",
			})
		}
	}

	return resetPoints, nil
}

// truncateString truncates a string to maxLen and adds ellipsis if needed.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
