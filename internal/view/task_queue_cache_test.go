package view

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestTaskQueueCacheFIFO(t *testing.T) {
	c := newTaskQueueCache(2)
	c.put("ns", "a", taskQueueCacheEntry{pollerCount: 1})
	c.put("ns", "b", taskQueueCacheEntry{pollerCount: 2})
	c.put("ns", "a", taskQueueCacheEntry{pollerCount: 11})
	c.put("ns", "c", taskQueueCacheEntry{pollerCount: 3})

	if _, ok := c.get("ns", "a"); ok {
		t.Fatal("FIFO should evict the first inserted queue")
	}
	if entry, ok := c.get("ns", "b"); !ok || entry.pollerCount != 2 {
		t.Fatal("second insert should still be present")
	}
	if entry, ok := c.get("ns", "c"); !ok || entry.pollerCount != 3 {
		t.Fatal("newest insert should be present")
	}
}

func TestTaskQueueCacheDisabled(t *testing.T) {
	c := newTaskQueueCache(0)
	c.put("ns", "a", taskQueueCacheEntry{pollers: []temporal.Poller{{Identity: "w"}}})
	if _, ok := c.get("ns", "a"); ok {
		t.Fatal("size 0 should not cache")
	}
}

func TestTaskQueueViewKeepsListAndPollers(t *testing.T) {
	tq := NewTaskQueueView(&App{})
	tq.allQueues = []taskQueueEntry{{Name: "keep-me", Type: "Combined"}}
	tq.applyFilter("")
	tq.Start()
	if len(tq.allQueues) != 1 || tq.allQueues[0].Name != "keep-me" {
		t.Fatalf("start should keep the cached queue list, got %+v", tq.allQueues)
	}

	now := time.Now()
	tq.cache.put("", "keep-me", taskQueueCacheEntry{
		pollers:     []temporal.Poller{{Identity: "worker-1", LastAccessTime: now, TaskQueueType: "Workflow"}},
		pollerCount: 4,
		backlog:     7,
	})
	tq.loadPollers(0)
	if len(tq.pollers) != 1 || tq.pollers[0].Identity != "worker-1" {
		t.Fatalf("pollers should come from cache, got %+v", tq.pollers)
	}
	if tq.queues[0].PollerCount != 4 || tq.queues[0].Backlog != 7 {
		t.Fatalf("queue stats should come from cache, got %+v", tq.queues[0])
	}
}

func TestTaskQueuePrefetchFillsPollerCountsWithoutHighlight(t *testing.T) {
	provider := &fakeWorkerProvider{
		pollers: map[string][]temporal.Poller{
			"orders":   {{Identity: "w1"}, {Identity: "w2"}},
			"payments": {{Identity: "w3"}},
			"shipping": nil,
		},
	}
	tq := NewTaskQueueView(&App{provider: provider})
	tq.allQueues = []taskQueueEntry{
		{Name: "orders", Type: "Combined"},
		{Name: "payments", Type: "Combined"},
		{Name: "shipping", Type: "Combined"},
	}
	tq.applyFilter("")
	tq.prefetchQueueStats()

	if got := int(atomic.LoadInt32(&provider.describeCalls)); got != 3 {
		t.Fatalf("every queue should be described, got %d", got)
	}
	if tq.queues[0].PollerCount != 2 || tq.queues[1].PollerCount != 1 || tq.queues[2].PollerCount != 0 {
		t.Fatalf("poller counts should fill without highlighting a row, got %+v", tq.queues)
	}
	if cells := tq.queueTable.GetRowData(1); len(cells) < 3 || cells[2] != "1" {
		t.Fatalf("payments poller cell should show 1, got %v", cells)
	}
}
