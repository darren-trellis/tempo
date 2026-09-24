package temporal

import (
	"context"
	"fmt"
	"strings"
)

// GetChildWorkflows returns immediate child workflows spawned by a workflow.
// This parses the workflow history for ChildWorkflowExecutionStarted events.
func (c *Client) GetChildWorkflows(ctx context.Context, namespace, workflowID, runID string) ([]Workflow, error) {
	events, err := c.GetEnhancedWorkflowHistory(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow history: %w", err)
	}
	return c.getChildWorkflowsFromEvents(ctx, namespace, events)
}

// getChildWorkflowsFromEvents extracts child workflows from pre-fetched events.
// This avoids duplicate history fetches when called from GetWorkflowRelationships.
func (c *Client) getChildWorkflowsFromEvents(ctx context.Context, namespace string, events []EnhancedHistoryEvent) ([]Workflow, error) {
	// Track child workflow IDs
	var childIDs []string
	seen := make(map[string]bool)

	for _, event := range events {
		// ChildWorkflowExecutionStarted contains the child's run ID
		if strings.Contains(event.Type, "ChildWorkflowExecutionStarted") {
			if event.ChildWorkflowID != "" && !seen[event.ChildWorkflowID] {
				childIDs = append(childIDs, event.ChildWorkflowID)
				seen[event.ChildWorkflowID] = true
			}
		}
	}

	if len(childIDs) == 0 {
		return nil, nil
	}

	// Fetch child workflows in parallel (limit concurrency to avoid overwhelming the server)
	type result struct {
		workflow Workflow
		err      error
	}

	results := make(chan result, len(childIDs))
	sem := make(chan struct{}, 5) // Max 5 concurrent requests

	for _, childID := range childIDs {
		go func(id string) {
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			query := fmt.Sprintf("WorkflowId = '%s'", id)
			workflows, _, err := c.ListWorkflows(ctx, namespace, ListOptions{
				PageSize: 1,
				Query:    query,
			})
			if err != nil {
				results <- result{err: err}
				return
			}
			if len(workflows) > 0 {
				results <- result{workflow: workflows[0]}
			} else {
				results <- result{}
			}
		}(childID)
	}

	found := make([]Workflow, 0, len(childIDs))
	for range childIDs {
		r := <-results
		if r.err == nil && r.workflow.ID != "" {
			found = append(found, r.workflow)
		}
	}

	return workflowsInIDsOrder(childIDs, found), nil
}

func workflowsInIDsOrder(ids []string, found []Workflow) []Workflow {
	byID := make(map[string]Workflow, len(found))
	for _, w := range found {
		if w.ID == "" {
			continue
		}
		byID[w.ID] = w
	}
	children := make([]Workflow, 0, len(ids))
	for _, id := range ids {
		if w, ok := byID[id]; ok {
			children = append(children, w)
		}
	}
	return children
}

// GetWorkflowRelationships returns the complete relationship graph for a workflow.
// depth controls how many levels of children to fetch (1 = immediate children only).
func (c *Client) GetWorkflowRelationships(ctx context.Context, namespace, workflowID, runID string, depth int) (*WorkflowRelationships, error) {
	// Get the current workflow details
	current, err := c.GetWorkflow(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	result := &WorkflowRelationships{
		Current: current,
	}

	// Get parent workflow if exists (in parallel with history fetch)
	type parentResult struct {
		parent *Workflow
	}
	parentChan := make(chan parentResult, 1)

	go func() {
		if current.ParentID != nil && *current.ParentID != "" {
			query := fmt.Sprintf("WorkflowId = '%s'", *current.ParentID)
			parents, _, err := c.ListWorkflows(ctx, namespace, ListOptions{
				PageSize: 1,
				Query:    query,
			})
			if err == nil && len(parents) > 0 {
				parentChan <- parentResult{parent: &parents[0]}
				return
			}
		}
		parentChan <- parentResult{}
	}()

	// Get enhanced history for relationship extraction
	events, err := c.GetEnhancedWorkflowHistory(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow history: %w", err)
	}

	// Wait for parent lookup
	parentRes := <-parentChan
	result.Parent = parentRes.parent

	// Extract signals from events (single pass)
	for _, event := range events {
		if strings.Contains(event.Type, "SignalExternalWorkflowExecutionInitiated") {
			signal := WorkflowSignal{
				FromWorkflowID: workflowID,
				ToWorkflowID:   event.ChildWorkflowID,
				Time:           event.Time,
			}
			if strings.Contains(event.Details, "SignalName:") {
				parts := strings.Split(event.Details, "SignalName: ")
				if len(parts) > 1 {
					signalPart := strings.Split(parts[1], ",")[0]
					signal.SignalName = strings.TrimSpace(signalPart)
				}
			}
			result.OutgoingSignals = append(result.OutgoingSignals, signal)
		} else if strings.Contains(event.Type, "WorkflowExecutionSignaled") {
			signal := WorkflowSignal{
				ToWorkflowID: workflowID,
				Time:         event.Time,
			}
			if strings.Contains(event.Details, "SignalName:") {
				parts := strings.Split(event.Details, "SignalName: ")
				if len(parts) > 1 {
					signalPart := strings.Split(parts[1], ",")[0]
					signal.SignalName = strings.TrimSpace(signalPart)
				}
			}
			result.IncomingSignals = append(result.IncomingSignals, signal)
		}
	}

	// Get child workflows with depth control (reuse events, don't fetch again)
	if depth > 0 {
		children, err := c.getChildWorkflowsFromEvents(ctx, namespace, events)
		if err == nil {
			for _, child := range children {
				node := &WorkflowNode{
					Workflow: child,
					EdgeType: "child",
					Depth:    1,
				}

				// Recursively fetch children if depth allows
				if depth > 1 {
					childRel, err := c.GetWorkflowRelationships(ctx, namespace, child.ID, child.RunID, depth-1)
					if err == nil && childRel != nil {
						node.Children = childRel.Children
					}
				}

				result.Children = append(result.Children, node)
			}
		}
	}

	return result, nil
}
