package temporal

import (
	"sort"

	workerpb "go.temporal.io/api/worker/v1"
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
