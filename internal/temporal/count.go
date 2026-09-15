package temporal

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
)

const executionStatusGroupBy = "GROUP BY ExecutionStatus"

func countGroupByQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return executionStatusGroupBy
	}
	return query + " " + executionStatusGroupBy
}

func countStatusQuery(query, status string) string {
	clause := fmt.Sprintf(`ExecutionStatus="%s"`, status)
	query = strings.TrimSpace(query)
	if query == "" {
		return clause
	}
	return "(" + query + ") AND " + clause
}

// CountWorkflows returns visibility counts grouped by execution status.
func (c *Client) CountWorkflows(ctx context.Context, namespace, query string) (WorkflowCounts, error) {
	cl, err := c.conn()
	if err != nil {
		return WorkflowCounts{}, err
	}

	resp, err := cl.WorkflowService().CountWorkflowExecutions(ctx, &workflowservice.CountWorkflowExecutionsRequest{
		Namespace: namespace,
		Query:     countGroupByQuery(query),
	})
	if err == nil {
		return workflowCountsFromGroups(resp), nil
	}
	if !countGroupByUnsupported(err) {
		return WorkflowCounts{}, fmt.Errorf("failed to count workflows: %w", err)
	}

	return c.countWorkflowsByStatus(ctx, cl.WorkflowService(), namespace, query)
}

func (c *Client) countWorkflowsByStatus(ctx context.Context, svc workflowservice.WorkflowServiceClient, namespace, query string) (WorkflowCounts, error) {
	statuses := []string{"Running", "Completed", "Failed", "Canceled", "Terminated", "TimedOut", "ContinuedAsNew"}
	counts := make([]int, len(statuses))
	errs := make([]error, len(statuses))
	var wg sync.WaitGroup
	for i, status := range statuses {
		wg.Add(1)
		go func(i int, status string) {
			defer wg.Done()
			resp, err := svc.CountWorkflowExecutions(ctx, &workflowservice.CountWorkflowExecutionsRequest{
				Namespace: namespace,
				Query:     countStatusQuery(query, status),
			})
			if err != nil {
				errs[i] = err
				return
			}
			counts[i] = int(resp.GetCount())
		}(i, status)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return WorkflowCounts{}, fmt.Errorf("failed to count workflows: %w", err)
		}
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	return WorkflowCounts{
		Running:        counts[0],
		Completed:      counts[1],
		Failed:         counts[2],
		Canceled:       counts[3],
		Terminated:     counts[4],
		TimedOut:       counts[5],
		ContinuedAsNew: counts[6],
		Total:          total,
	}, nil
}

func visibilityGroupByQuery(field string) string {
	return "GROUP BY " + field
}

func visibilityGroupValues(resp *workflowservice.CountWorkflowExecutionsResponse) []string {
	if resp == nil {
		return nil
	}
	out := make([]string, 0, len(resp.GetGroups()))
	for _, group := range resp.GetGroups() {
		if name := decodeCountGroupString(group.GetGroupValues()); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func decodeCountGroupString(values []*commonpb.Payload) string {
	if len(values) == 0 {
		return ""
	}
	payload := values[0]
	var text string
	if err := converter.GetDefaultDataConverter().FromPayload(payload, &text); err == nil {
		return strings.TrimSpace(text)
	}
	if payload != nil {
		return strings.Trim(string(payload.GetData()), `"`)
	}
	return ""
}

func (c *Client) listVisibilityDistinct(ctx context.Context, namespace, field string) ([]string, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	resp, err := cl.WorkflowService().CountWorkflowExecutions(ctx, &workflowservice.CountWorkflowExecutionsRequest{
		Namespace: namespace,
		Query:     visibilityGroupByQuery(field),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list %s values: %w", field, err)
	}
	return visibilityGroupValues(resp), nil
}

func countGroupByUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "group by") || strings.Contains(msg, "groupby")
}

func workflowCountsFromGroups(resp *workflowservice.CountWorkflowExecutionsResponse) WorkflowCounts {
	var counts WorkflowCounts
	if resp == nil {
		return counts
	}
	for _, group := range resp.GetGroups() {
		status := decodeCountGroupStatus(group.GetGroupValues())
		n := int(group.GetCount())
		switch status {
		case "Running":
			counts.Running += n
		case "Completed":
			counts.Completed += n
		case "Failed":
			counts.Failed += n
		case "Canceled":
			counts.Canceled += n
		case "Terminated":
			counts.Terminated += n
		case "TimedOut":
			counts.TimedOut += n
		case "ContinuedAsNew":
			counts.ContinuedAsNew += n
		}
		counts.Total += n
	}
	if counts.Total == 0 {
		counts.Total = int(resp.GetCount())
	}
	return counts
}

func decodeCountGroupStatus(values []*commonpb.Payload) string {
	if len(values) == 0 {
		return ""
	}
	payload := values[0]
	var text string
	if err := converter.GetDefaultDataConverter().FromPayload(payload, &text); err == nil {
		if status := normalizeCountStatus(text); status != "" {
			return status
		}
	}
	var number int64
	if err := converter.GetDefaultDataConverter().FromPayload(payload, &number); err == nil {
		return mapCountStatus(enums.WorkflowExecutionStatus(number))
	}
	if payload != nil {
		return normalizeCountStatus(strings.Trim(string(payload.GetData()), `"`))
	}
	return ""
}

func normalizeCountStatus(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return mapCountStatus(enums.WorkflowExecutionStatus(n))
	}
	switch strings.ToLower(s) {
	case "running":
		return "Running"
	case "completed":
		return "Completed"
	case "failed":
		return "Failed"
	case "canceled", "cancelled":
		return "Canceled"
	case "terminated":
		return "Terminated"
	case "timedout", "timed_out", "timed out":
		return "TimedOut"
	case "continuedasnew", "continued_as_new", "continued as new":
		return "ContinuedAsNew"
	default:
		return ""
	}
}

func mapCountStatus(status enums.WorkflowExecutionStatus) string {
	if status == enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW {
		return "ContinuedAsNew"
	}
	return MapWorkflowStatus(status)
}
