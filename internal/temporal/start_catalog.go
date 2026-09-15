package temporal

import (
	"context"
)

func collectStartCatalog(typeGroups, queueGroups []string, workflows []Workflow, workers []Worker, schedules []Schedule) (types, queues []string) {
	typeSet := map[string]struct{}{}
	queueSet := map[string]struct{}{}
	addTaskQueueNames(typeSet, typeGroups...)
	addTaskQueueNames(queueSet, queueGroups...)
	for _, wf := range workflows {
		addTaskQueueNames(typeSet, wf.Type)
		addTaskQueueNames(queueSet, wf.TaskQueue)
	}
	addTaskQueueNames(queueSet, workerTaskQueues(workers)...)
	for _, s := range schedules {
		addTaskQueueNames(typeSet, s.WorkflowType)
		addTaskQueueNames(queueSet, s.TaskQueue)
	}
	return sortedTaskQueueNames(typeSet), sortedTaskQueueNames(queueSet)
}

func (c *Client) ListStartCatalog(ctx context.Context, namespace string) (types, queues []string, err error) {
	var typeGroups, queueGroups []string
	var workflows []Workflow
	var workers []Worker
	var schedules []Schedule
	ok := false
	var firstErr error

	if groups, groupErr := c.listVisibilityDistinct(ctx, namespace, "WorkflowType"); groupErr == nil {
		ok = true
		typeGroups = groups
	} else if firstErr == nil {
		firstErr = groupErr
	}
	if groups, groupErr := c.listVisibilityDistinct(ctx, namespace, "TaskQueue"); groupErr == nil {
		ok = true
		queueGroups = groups
	} else if firstErr == nil {
		firstErr = groupErr
	}

	token := ""
	for {
		if ctx.Err() != nil {
			break
		}
		page, next, pageErr := c.ListWorkflows(ctx, namespace, ListOptions{PageSize: 1000, PageToken: token})
		if pageErr != nil {
			if firstErr == nil {
				firstErr = pageErr
			}
			break
		}
		ok = true
		workflows = append(workflows, page...)
		if next == "" {
			break
		}
		token = next
	}

	if listed, listErr := c.ListWorkers(ctx, namespace); listErr == nil {
		ok = true
		workers = listed
	} else if firstErr == nil {
		firstErr = listErr
	}

	if listed, _, listErr := c.ListSchedules(ctx, namespace, ListOptions{PageSize: 100}); listErr == nil {
		ok = true
		schedules = listed
	} else if firstErr == nil {
		firstErr = listErr
	}

	types, queues = collectStartCatalog(typeGroups, queueGroups, workflows, workers, schedules)
	if !ok && firstErr != nil {
		return nil, nil, firstErr
	}
	return types, queues, nil
}

func (c *Client) ListTaskQueueNames(ctx context.Context, namespace string) ([]string, error) {
	_, queues, err := c.ListStartCatalog(ctx, namespace)
	return queues, err
}

func (c *Client) ListWorkflowTypes(ctx context.Context, namespace string) ([]string, error) {
	types, _, err := c.ListStartCatalog(ctx, namespace)
	return types, err
}
