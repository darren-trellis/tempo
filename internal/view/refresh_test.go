package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"gopkg.in/yaml.v3"
)

func TestPopulateTablePreservesHighlightedWorkflow(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflows = []temporal.Workflow{
		{ID: "wf-1", RunID: "run-1", Type: "T", Status: "Running"},
		{ID: "wf-2", RunID: "run-2", Type: "T", Status: "Running"},
		{ID: "wf-3", RunID: "run-3", Type: "T", Status: "Running"},
	}
	wl.allWorkflows = wl.workflows
	wl.populateTable()
	wl.table.SelectRow(2)
	wl.rememberHighlightedWorkflow()

	wl.workflows = []temporal.Workflow{
		{ID: "wf-new", RunID: "run-new", Type: "T", Status: "Running"},
		{ID: "wf-1", RunID: "run-1", Type: "T", Status: "Running"},
		{ID: "wf-2", RunID: "run-2", Type: "T", Status: "Running"},
		{ID: "wf-3", RunID: "run-3", Type: "T", Status: "Running"},
	}
	wl.populateTable()
	row := wl.table.SelectedRow()
	if row < 0 || row >= len(wl.workflows) || wl.workflows[row].ID != "wf-3" {
		t.Fatalf("refresh should keep wf-3 highlighted, row=%d", row)
	}
}

func TestRefreshIntervalDefault(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if got := wl.refreshInterval(); got != config.DefaultRefreshRate {
		t.Fatalf("default interval = %s", got)
	}
}

func TestRefreshIntervalFromConfig(t *testing.T) {
	cfg := &config.Config{}
	if err := yaml.Unmarshal([]byte("refresh_rate: 2s\n"), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wl := NewWorkflowList(&App{config: cfg}, "default")
	if got := wl.refreshInterval(); got != 2*time.Second {
		t.Fatalf("configured interval = %s", got)
	}
}

func TestRenderPreviewEventsKeepsSelection(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.previewKind = previewEvents
	wl.previewEvents = []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)},
		{ID: 5, Type: "ActivityTaskScheduled", Time: time.Date(2026, 8, 23, 12, 0, 1, 0, time.UTC)},
	}
	wl.renderPreviewEvents(temporal.Workflow{ID: "wf"})
	wl.eventTable.SelectRow(1)
	wl.renderPreviewEvents(temporal.Workflow{ID: "wf"})
	if wl.eventTable.SelectedRow() != 1 {
		t.Fatalf("live refresh should keep the selected event, row=%d", wl.eventTable.SelectedRow())
	}
}

func TestPreviewCacheClear(t *testing.T) {
	c := newPreviewCache(2)
	c.put("a", "1", []temporal.EnhancedHistoryEvent{{ID: 1}})
	c.clear()
	if _, ok := c.get("a", "1"); ok {
		t.Fatal("clear should drop cached history")
	}
	c.put("b", "2", []temporal.EnhancedHistoryEvent{{ID: 2}})
	if _, ok := c.get("b", "2"); !ok {
		t.Fatal("cache should still be usable after clear")
	}
}

func TestTaskQueueCacheClearAndRemove(t *testing.T) {
	c := newTaskQueueCache(2)
	c.put("ns", "alpha", taskQueueCacheEntry{pollerCount: 1})
	c.put("ns", "beta", taskQueueCacheEntry{pollerCount: 2})

	c.remove("ns", "alpha")
	if _, ok := c.get("ns", "alpha"); ok {
		t.Fatal("remove should drop that queue")
	}
	if _, ok := c.get("ns", "beta"); !ok {
		t.Fatal("remove should leave other queues alone")
	}

	c.put("ns", "gamma", taskQueueCacheEntry{pollerCount: 3})
	if _, ok := c.get("ns", "beta"); !ok {
		t.Fatal("removing a queue should free its slot rather than evict a live entry")
	}

	c.clear()
	if _, ok := c.get("ns", "beta"); ok {
		t.Fatal("clear should drop cached queues")
	}
}

func TestWorkflowListRefreshInvalidatesPreview(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.previewCache.put("wf", "run", []temporal.EnhancedHistoryEvent{{ID: 1}})
	wl.previewWorkflowID = "wf"
	wl.previewRunID = "run"
	wl.previewEvents = []temporal.EnhancedHistoryEvent{{ID: 1}}
	wl.previewActivities = []previewActivity{{ScheduledID: 1}}

	wl.invalidateCaches()

	if _, ok := wl.previewCache.get("wf", "run"); ok {
		t.Fatal("refresh should drop the cached preview history")
	}
	if wl.previewWorkflowID != "" || wl.previewRunID != "" {
		t.Fatal("refresh should forget which workflow is previewed")
	}
	if wl.previewEvents != nil || wl.previewActivities != nil {
		t.Fatal("refresh should drop the events held for the preview")
	}
}

func TestTaskQueueForcedPollersDropCache(t *testing.T) {
	tq := NewTaskQueueView(&App{})
	tq.queues = []taskQueueEntry{{Name: "alpha", Type: "Combined"}}
	tq.cache.put(tq.namespace(), "alpha", taskQueueCacheEntry{pollerCount: 7})

	tq.schedulePollers(0, true)

	entry, ok := tq.cache.get(tq.namespace(), "alpha")
	if ok && entry.pollerCount == 7 {
		t.Fatal("a forced poller load should drop the cached entry, not answer from it")
	}
}

func TestGraphViewInvalidateDropsRelationships(t *testing.T) {
	wg := NewWorkflowGraphView(&App{}, "default", nil)
	wg.relationships = &temporal.WorkflowRelationships{}
	wg.loading = true
	gen := wg.loadGen

	wg.Invalidate()

	if wg.relationships != nil {
		t.Fatal("invalidate should drop the relationship graph")
	}
	if wg.loading {
		t.Fatal("invalidate should clear the loading flag so the next load can run")
	}
	if wg.loadGen == gen {
		t.Fatal("invalidate should abandon the load already in flight")
	}
}
