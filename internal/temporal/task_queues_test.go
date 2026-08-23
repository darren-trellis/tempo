package temporal

import (
	"reflect"
	"testing"

	workerpb "go.temporal.io/api/worker/v1"
)

func TestWorkerTypesFromHeartbeat(t *testing.T) {
	got := workerTypesFromHeartbeat(&workerpb.WorkerHeartbeat{
		WorkflowStickyPollerInfo: &workerpb.WorkerPollerInfo{},
		ActivityPollerInfo:       &workerpb.WorkerPollerInfo{},
	})
	want := []string{TaskQueueTypeWorkflow, TaskQueueTypeActivity}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("types: %v, want %v", got, want)
	}
}

func TestSortedTaskQueueNamesUnionsSources(t *testing.T) {
	names := map[string]struct{}{}
	addTaskQueueNames(names, workerTaskQueues([]Worker{
		{TaskQueue: "orders"},
		{TaskQueue: "payments"},
		{TaskQueue: ""},
	})...)
	addTaskQueueNames(names, "orders", "shipping")
	addTaskQueueNames(names, "payments")

	got := sortedTaskQueueNames(names)
	want := []string{"orders", "payments", "shipping"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("queues: %v, want %v", got, want)
	}
}
