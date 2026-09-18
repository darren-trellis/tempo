package temporal

import (
	"context"
	"sync"
)

const startCatalogMaxWorkflowPages = 5

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

func startCatalogNeedsWorkflowScan(typeErr, queueErr error) bool {
	return typeErr != nil || queueErr != nil
}

func (c *Client) ListStartCatalog(ctx context.Context, namespace string) (types, queues []string, err error) {
	var (
		typeGroups, queueGroups []string
		typeErr, queueErr       error
		workers                 []Worker
		workerErr               error
		schedules               []Schedule
		schedErr                error
		wg                      sync.WaitGroup
	)

	wg.Add(4)
	go func() {
		defer wg.Done()
		typeGroups, typeErr = c.listVisibilityDistinct(ctx, namespace, "WorkflowType")
	}()
	go func() {
		defer wg.Done()
		queueGroups, queueErr = c.listVisibilityDistinct(ctx, namespace, "TaskQueue")
	}()
	go func() {
		defer wg.Done()
		workers, workerErr = c.ListWorkers(ctx, namespace)
	}()
	go func() {
		defer wg.Done()
		var listErr error
		schedules, _, listErr = c.ListSchedules(ctx, namespace, ListOptions{PageSize: 100})
		schedErr = listErr
	}()
	wg.Wait()

	var workflows []Workflow
	if startCatalogNeedsWorkflowScan(typeErr, queueErr) {
		workflows = c.scanWorkflowsForCatalog(ctx, namespace)
	}

	ok := typeErr == nil || queueErr == nil || workerErr == nil || schedErr == nil || len(workflows) > 0
	firstErr := firstCatalogErr(typeErr, queueErr, workerErr, schedErr)
	types, queues = collectStartCatalog(typeGroups, queueGroups, workflows, workers, schedules)
	if !ok && firstErr != nil {
		return nil, nil, firstErr
	}
	return types, queues, nil
}

func firstCatalogErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) scanWorkflowsForCatalog(ctx context.Context, namespace string) []Workflow {
	var workflows []Workflow
	token := ""
	for page := 0; page < startCatalogMaxWorkflowPages; page++ {
		if ctx.Err() != nil {
			break
		}
		batch, next, err := c.ListWorkflows(ctx, namespace, ListOptions{PageSize: 1000, PageToken: token})
		if err != nil {
			break
		}
		workflows = append(workflows, batch...)
		if next == "" {
			break
		}
		token = next
	}
	return workflows
}

func (c *Client) ListTaskQueueNames(ctx context.Context, namespace string) ([]string, error) {
	_, queues, err := c.ListStartCatalog(ctx, namespace)
	return queues, err
}

func (c *Client) ListWorkflowTypes(ctx context.Context, namespace string) ([]string, error) {
	types, _, err := c.ListStartCatalog(ctx, namespace)
	return types, err
}
