package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestMergeWorkerPollers(t *testing.T) {
	now := time.Now()
	byIdentity := map[string]*workerEntry{}
	mergeWorkerPollers(byIdentity, "order-tasks", []temporal.Poller{
		{Identity: "worker-1", TaskQueueType: "Workflow", LastAccessTime: now.Add(-10 * time.Second)},
		{Identity: "worker-1", TaskQueueType: "Activity", LastAccessTime: now.Add(-2 * time.Second)},
	})
	mergeWorkerPollers(byIdentity, "payment-tasks", []temporal.Poller{
		{Identity: "worker-1", TaskQueueType: "Workflow", LastAccessTime: now.Add(-1 * time.Second)},
		{Identity: "worker-2", TaskQueueType: "Activity", LastAccessTime: now},
	})

	workers := workerEntriesFromMap(byIdentity)
	if len(workers) != 2 {
		t.Fatalf("workers: %d", len(workers))
	}
	if workers[0].Identity != "worker-2" {
		t.Fatalf("newest worker first: %q", workers[0].Identity)
	}
	if workers[1].Identity != "worker-1" {
		t.Fatalf("worker-1: %q", workers[1].Identity)
	}
	if got := strings.Join(workers[1].Queues, ", "); got != "order-tasks, payment-tasks" {
		t.Fatalf("queues: %q", got)
	}
	if got := strings.Join(workers[1].Types, ", "); got != "Activity, Workflow" {
		t.Fatalf("types: %q", got)
	}
}
