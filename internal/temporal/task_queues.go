package temporal

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/taskqueue/v1"
	workerpb "go.temporal.io/api/worker/v1"
	"go.temporal.io/api/workflowservice/v1"
)

func addTaskQueueNames(names map[string]struct{}, queues ...string) {
	for _, name := range queues {
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}
}

func workerTaskQueues(workers []Worker) []string {
	queues := make([]string, 0, len(workers))
	for _, w := range workers {
		queues = append(queues, w.TaskQueue)
	}
	return queues
}

func sortedTaskQueueNames(names map[string]struct{}) []string {
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func workerTypesFromHeartbeat(hb *workerpb.WorkerHeartbeat) []string {
	if hb == nil {
		return nil
	}
	var types []string
	if hb.GetWorkflowPollerInfo() != nil || hb.GetWorkflowStickyPollerInfo() != nil {
		types = append(types, TaskQueueTypeWorkflow)
	}
	if hb.GetActivityPollerInfo() != nil {
		types = append(types, TaskQueueTypeActivity)
	}
	if hb.GetNexusPollerInfo() != nil {
		types = append(types, "Nexus")
	}
	return types
}

// DescribeTaskQueue returns task queue info and active pollers.
func (c *Client) DescribeTaskQueue(ctx context.Context, namespace, taskQueue string) (*TaskQueueInfo, []Poller, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, nil, err
	}
	type descResult struct {
		resp *workflowservice.DescribeTaskQueueResponse
		err  error
	}
	describe := func(tqType enums.TaskQueueType) descResult {
		resp, err := cl.WorkflowService().DescribeTaskQueue(ctx, &workflowservice.DescribeTaskQueueRequest{
			Namespace: namespace,
			TaskQueue: &taskqueue.TaskQueue{
				Name: taskQueue,
				Kind: enums.TASK_QUEUE_KIND_NORMAL,
			},
			TaskQueueType: tqType,
		})
		return descResult{resp: resp, err: err}
	}

	var wf, act descResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		wf = describe(enums.TASK_QUEUE_TYPE_WORKFLOW)
	}()
	go func() {
		defer wg.Done()
		act = describe(enums.TASK_QUEUE_TYPE_ACTIVITY)
	}()
	wg.Wait()
	if wf.err != nil && act.err != nil {
		return nil, nil, fmt.Errorf("failed to describe workflow task queue: %w", wf.err)
	}

	var pollers []Poller
	if wf.resp != nil {
		for _, p := range wf.resp.GetPollers() {
			pollers = append(pollers, Poller{
				Identity:       p.GetIdentity(),
				LastAccessTime: p.GetLastAccessTime().AsTime(),
				TaskQueueType:  TaskQueueTypeWorkflow,
				RatePerSecond:  p.GetRatePerSecond(),
			})
		}
	}
	if act.resp != nil {
		for _, p := range act.resp.GetPollers() {
			pollers = append(pollers, Poller{
				Identity:       p.GetIdentity(),
				LastAccessTime: p.GetLastAccessTime().AsTime(),
				TaskQueueType:  TaskQueueTypeActivity,
				RatePerSecond:  p.GetRatePerSecond(),
			})
		}
	}

	info := &TaskQueueInfo{
		Name:        taskQueue,
		Type:        "Combined",
		PollerCount: len(pollers),
		Backlog:     0,
	}

	return info, pollers, nil
}
