package view

import (
	"context"
	"strings"
	"sync"
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

func TestTaskQueueUsesCachedCatalog(t *testing.T) {
	a := &App{currentNS: "default", provider: &fakeWorkerProvider{queues: []string{"from-server"}}}
	a.catalog.putIfCurrent("default", a.catalog.beginFetch("default"), startCatalog{
		queues: []string{"orders", "payments"},
	})
	tq := NewTaskQueueView(a)
	tq.loadData()
	if len(tq.allQueues) != 2 || tq.allQueues[0].Name != "orders" || tq.allQueues[1].Name != "payments" {
		t.Fatalf("task queues should use the cached catalog, got %+v", tq.allQueues)
	}
}

func TestMergeQueueNamesKeepsPollerCounts(t *testing.T) {
	tq := NewTaskQueueView(&App{})
	tq.suppressSelect = true
	tq.allQueues = []taskQueueEntry{
		{Name: "orders", Type: "Combined", PollerCount: 4, Backlog: 7},
		{Name: "old-queue", Type: "Combined", PollerCount: 1},
	}
	tq.applyFilter("")
	tq.queueTable.SelectRow(0)
	tq.selectedQueue = "orders"
	tq.suppressSelect = false
	for i := range tq.allQueues {
		if tq.allQueues[i].Name == "orders" {
			tq.allQueues[i].PollerCount = 4
			tq.allQueues[i].Backlog = 7
		}
	}

	tq.mergeQueueNames([]string{"payments", "orders"})

	if len(tq.allQueues) != 2 {
		t.Fatalf("got %d queues: %+v", len(tq.allQueues), tq.allQueues)
	}
	if tq.allQueues[0].Name != "payments" || tq.allQueues[0].PollerCount != 0 {
		t.Fatalf("new queue should start at 0 pollers, got %+v", tq.allQueues[0])
	}
	if tq.allQueues[1].Name != "orders" || tq.allQueues[1].PollerCount != 4 || tq.allQueues[1].Backlog != 7 {
		t.Fatalf("existing queue should keep stats, got %+v", tq.allQueues[1])
	}
	if tq.selectedQueue != "orders" || tq.queueTable.SelectedRow() != 1 {
		t.Fatalf("selection should follow orders, row=%d name=%s", tq.queueTable.SelectedRow(), tq.selectedQueue)
	}
}

type countingQueueProvider struct {
	fakeWorkerProvider
	mu    sync.Mutex
	calls int
	first context.Context
}

func (p *countingQueueProvider) ListTaskQueueNames(ctx context.Context, namespace string) ([]string, error) {
	p.mu.Lock()
	p.calls++
	if p.calls == 1 {
		p.first = ctx
	}
	p.mu.Unlock()
	return p.fakeWorkerProvider.ListTaskQueueNames(ctx, namespace)
}

func TestAutoRefreshSkipsWhileTaskQueuesAreRefreshing(t *testing.T) {
	hold := make(chan struct{})
	provider := &countingQueueProvider{fakeWorkerProvider: fakeWorkerProvider{
		queues:         []string{"orders"},
		listQueuesHold: hold,
	}}
	tq := NewTaskQueueView(&App{provider: provider})
	tq.loadQueueList(true)

	deadline := time.Now().Add(2 * time.Second)
	for {
		provider.mu.Lock()
		n := provider.calls
		provider.mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("manual refresh did not start a fetch")
		}
		time.Sleep(5 * time.Millisecond)
	}

	tq.liveRefresh()
	time.Sleep(50 * time.Millisecond)
	close(hold)

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.calls != 1 || provider.first.Err() != nil {
		t.Fatalf("auto-refresh should leave the manual refresh running, calls=%d canceled=%v", provider.calls, provider.first.Err() != nil)
	}
}

func TestTaskQueueLiveRefreshDoesNotZeroCounts(t *testing.T) {
	hold := make(chan struct{})
	provider := &fakeWorkerProvider{
		queues:         []string{"orders", "payments"},
		listQueuesHold: hold,
		pollers: map[string][]temporal.Poller{
			"orders":   {{Identity: "w1"}, {Identity: "w2"}},
			"payments": {{Identity: "w3"}},
		},
	}
	tq := NewTaskQueueView(&App{provider: provider})
	tq.allQueues = []taskQueueEntry{
		{Name: "orders", Type: "Combined", PollerCount: 9, Backlog: 3},
	}
	tq.applyFilter("")
	tq.liveRefresh()
	if !tq.loading {
		t.Fatal("live refresh should show the spinner while the fetch is in flight")
	}
	if got := tq.app.loadingText(); got == "" || strings.Contains(got, "Loading") {
		t.Fatalf("auto-refresh should show only the spinner, got %q", got)
	}
	close(hold)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !tq.liveBusy && len(tq.allQueues) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if tq.liveBusy {
		t.Fatal("live refresh did not finish")
	}
	if tq.loading {
		t.Fatal("live refresh should clear the spinner when it finishes")
	}

	found := map[string]taskQueueEntry{}
	for _, q := range tq.allQueues {
		found[q.Name] = q
	}
	if found["orders"].PollerCount != 2 {
		t.Fatalf("orders pollers: %+v", found["orders"])
	}
	if found["payments"].PollerCount != 1 {
		t.Fatalf("payments pollers: %+v", found["payments"])
	}
}
