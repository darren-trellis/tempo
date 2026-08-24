package view

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

// fakeWorkerProvider answers only the calls the worker loader makes.
type fakeWorkerProvider struct {
	temporal.Provider
	heartbeats    []temporal.Worker
	heartbeatErr  error
	queues        []string
	queuesErr     error
	pollers       map[string][]temporal.Poller
	describeErr   error
	describeCalls int32
}

func (f *fakeWorkerProvider) ListWorkers(context.Context, string) ([]temporal.Worker, error) {
	return f.heartbeats, f.heartbeatErr
}

func (f *fakeWorkerProvider) ListTaskQueueNames(context.Context, string) ([]string, error) {
	return f.queues, f.queuesErr
}

func (f *fakeWorkerProvider) DescribeTaskQueue(_ context.Context, _, taskQueue string) (*temporal.TaskQueueInfo, []temporal.Poller, error) {
	atomic.AddInt32(&f.describeCalls, 1)
	if f.describeErr != nil {
		return nil, nil, f.describeErr
	}
	return nil, f.pollers[taskQueue], nil
}

func TestLoadWorkersMergesHeartbeatsWithPollRegistries(t *testing.T) {
	now := time.Now()
	provider := &fakeWorkerProvider{
		heartbeats: []temporal.Worker{
			// Stopped a few minutes ago; the server still returns the record.
			{Identity: "49027@laptop", TaskQueue: "orders", Status: temporal.WorkerStatusRunning, LastHeartbeat: now.Add(-5 * time.Minute)},
		},
		queues: []string{"orders", "payments"},
		pollers: map[string][]temporal.Poller{
			"orders":   {{Identity: "51067@laptop", TaskQueueType: "Workflow", LastAccessTime: now.Add(-time.Second)}},
			"payments": {{Identity: "51067@laptop", TaskQueueType: "Activity", LastAccessTime: now.Add(-2 * time.Second)}},
		},
	}

	workers, err := loadWorkers(context.Background(), provider, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(workers) != 3 {
		t.Fatalf("expected the heartbeat plus both polled instances, got %d: %+v", len(workers), workers)
	}
	if got := int(atomic.LoadInt32(&provider.describeCalls)); got != 2 {
		t.Fatalf("every task queue should be swept once, got %d calls", got)
	}

	byKey := map[string]temporal.Worker{}
	for _, w := range workers {
		byKey[temporal.WorkerKey(w)] = w
	}
	if got, ok := byKey["51067@laptop|orders"]; !ok || got.Status != temporal.WorkerStatusPolling {
		t.Fatalf("the live poller should be listed as polling: %+v", got)
	}
	if got := byKey["49027@laptop|orders"]; got.Status != temporal.WorkerStatusStale {
		t.Fatalf("a heartbeat that stopped moving should read as stale: %+v", got)
	}
}

func TestLoadWorkersSurvivesEitherSourceFailing(t *testing.T) {
	now := time.Now()
	boom := errors.New("boom")

	// Heartbeats unavailable: the poll registries still answer.
	polledOnly := &fakeWorkerProvider{
		heartbeatErr: boom,
		queues:       []string{"orders"},
		pollers: map[string][]temporal.Poller{
			"orders": {{Identity: "51067@laptop", TaskQueueType: "Workflow", LastAccessTime: now}},
		},
	}
	workers, err := loadWorkers(context.Background(), polledOnly, "default")
	if err != nil || len(workers) != 1 || workers[0].Identity != "51067@laptop" {
		t.Fatalf("workers=%+v err=%v", workers, err)
	}

	// Task queues unavailable: the heartbeats still answer.
	beatsOnly := &fakeWorkerProvider{
		heartbeats: []temporal.Worker{
			{Identity: "49027@laptop", TaskQueue: "orders", Status: temporal.WorkerStatusRunning, LastHeartbeat: now},
		},
		queuesErr: boom,
	}
	workers, err = loadWorkers(context.Background(), beatsOnly, "default")
	if err != nil || len(workers) != 1 || workers[0].Status != temporal.WorkerStatusRunning {
		t.Fatalf("workers=%+v err=%v", workers, err)
	}

	// A single queue failing to describe does not sink the sweep.
	partial := &fakeWorkerProvider{
		heartbeats:  nil,
		queues:      []string{"orders"},
		describeErr: boom,
	}
	if workers, err = loadWorkers(context.Background(), partial, "default"); err != nil || len(workers) != 0 {
		t.Fatalf("workers=%+v err=%v", workers, err)
	}

	// Both sources down is an error.
	if _, err = loadWorkers(context.Background(), &fakeWorkerProvider{heartbeatErr: boom, queuesErr: boom}, "default"); err == nil {
		t.Fatal("expected an error when neither source answers")
	}
}

func TestWorkerViewStartRereadsInsteadOfTrustingItsCache(t *testing.T) {
	wv := NewWorkerView(&App{})
	wv.allWorkers = []temporal.Worker{
		{Identity: "gone", Host: "retired-host", TaskQueue: "orders", Status: temporal.WorkerStatusRunning},
	}
	wv.applyFilter("")
	if !wv.hasInstance("gone") {
		t.Fatal("the stale worker should be showing before the reload")
	}

	wv.Start()
	if wv.hasInstance("gone") {
		t.Fatal("entering the tab should re-read, dropping a worker the server no longer reports")
	}
	if len(wv.workers) == 0 {
		t.Fatal("the reload should have populated the list")
	}
}

// hasInstance reports whether an identity is currently listed.
func (wv *WorkerView) hasInstance(identity string) bool {
	for _, w := range wv.workers {
		if w.Identity == identity {
			return true
		}
	}
	return false
}
