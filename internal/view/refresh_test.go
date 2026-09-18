package view

import (
	"fmt"
	"testing"
	"time"

	"github.com/atterpac/jig/layout"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"gopkg.in/yaml.v3"
)

func TestPopulateTablePreservesScrollOffset(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflows = make([]temporal.Workflow, 40)
	for i := range wl.workflows {
		wl.workflows[i] = temporal.Workflow{ID: fmt.Sprintf("wf-%d", i), RunID: "run", Type: "T", Status: "Running"}
	}
	wl.allWorkflows = wl.workflows
	wl.populateTable()
	wl.table.SelectRow(3)
	wl.table.SetOffset(12, 0)
	if wl.tableScroll != nil {
		wl.tableScroll.scrollTo(5)
	}
	wl.rememberHighlightedWorkflow()

	wl.populateTable()
	row, col := wl.table.GetOffset()
	if row != 12 || col != 0 {
		t.Fatalf("refresh should keep table scroll, offset=%d,%d", row, col)
	}
	if wl.tableScroll != nil && wl.tableScroll.offset != 5 {
		t.Fatalf("refresh should keep horizontal scroll, offset=%d", wl.tableScroll.offset)
	}
	if wl.table.SelectedRow() != 3 {
		t.Fatalf("refresh should keep the highlighted row, row=%d", wl.table.SelectedRow())
	}
}

func TestPopulateTableShiftsScrollWhenPagesPrepend(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	first := make([]temporal.Workflow, 20)
	for i := range first {
		first[i] = temporal.Workflow{ID: fmt.Sprintf("old-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.workflows = first
	wl.allWorkflows = first
	wl.populateTable()
	// Away from the first row, so the highlight rides along with its workflow
	// rather than staying pinned to the top of the list.
	wl.table.SelectRow(5)
	wl.table.SetOffset(0, 0)
	wl.rememberHighlightedWorkflow()
	if wl.tableScroll != nil {
		wl.tableScroll.scrollTo(4)
	}

	prepend := make([]temporal.Workflow, 10)
	for i := range prepend {
		prepend[i] = temporal.Workflow{ID: fmt.Sprintf("new-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.rememberListAnchor()
	wl.workflows = append(append([]temporal.Workflow{}, prepend...), first...)
	wl.allWorkflows = wl.workflows
	wl.populateTable()

	row, _ := wl.table.GetOffset()
	if row != 10 {
		t.Fatalf("prepend should keep the same rows on screen, offset=%d", row)
	}
	if wl.table.SelectedRow() != 15 || wl.workflows[wl.table.SelectedRow()].ID != "old-5" {
		t.Fatalf("should keep old-5 highlighted, row=%d", wl.table.SelectedRow())
	}
	if wl.tableScroll != nil && wl.tableScroll.offset != 4 {
		t.Fatalf("horizontal scroll should stay put, offset=%d", wl.tableScroll.offset)
	}
}

// Resting on the first row means "show me the newest", so newer workflows
// arriving on a refresh should not push the highlight down the list.
func TestFirstRowStaysHighlightedWhenNewerWorkflowsArrive(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	first := make([]temporal.Workflow, 20)
	for i := range first {
		first[i] = temporal.Workflow{ID: fmt.Sprintf("old-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.workflows = first
	wl.allWorkflows = first
	wl.populateTable()
	wl.table.SelectRow(0)
	wl.table.SetOffset(0, 0)
	wl.rememberHighlightedWorkflow()

	if wl.listEdgePin != listEdgeStart {
		t.Fatal("resting on the first row should pin to the start")
	}

	arrived := make([]temporal.Workflow, 3)
	for i := range arrived {
		arrived[i] = temporal.Workflow{ID: fmt.Sprintf("new-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.rememberListAnchor()
	wl.workflows = append(append([]temporal.Workflow{}, arrived...), first...)
	wl.allWorkflows = wl.workflows
	wl.populateTable()

	if got := wl.table.SelectedRow(); got != 0 {
		t.Fatalf("highlight should stay on the first row, row=%d", got)
	}
	if got := wl.workflows[wl.table.SelectedRow()].ID; got != "new-0" {
		t.Fatalf("first row should be the newest workflow, got %q", got)
	}
	if row, _ := wl.table.GetOffset(); row != 0 {
		t.Fatalf("the list should stay scrolled to the top, offset=%d", row)
	}
}

// Moving off the first row gives up the pin, so the highlight goes back to
// following its own workflow.
func TestMovingOffFirstRowReleasesThePin(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	items := make([]temporal.Workflow, 20)
	for i := range items {
		items[i] = temporal.Workflow{ID: fmt.Sprintf("old-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.workflows = items
	wl.allWorkflows = items
	wl.populateTable()

	wl.table.SelectRow(0)
	if wl.listEdgePin != listEdgeStart {
		t.Fatal("first row should pin")
	}
	wl.table.SelectRow(3)
	if wl.listEdgePin != listEdgeNone {
		t.Fatalf("moving away should release the pin, pin=%v", wl.listEdgePin)
	}
	wl.rememberHighlightedWorkflow()

	arrived := []temporal.Workflow{{ID: "new-0", RunID: "r", Type: "T", Status: "Running"}}
	wl.rememberListAnchor()
	wl.workflows = append(arrived, items...)
	wl.allWorkflows = wl.workflows
	wl.populateTable()

	if got := wl.workflows[wl.table.SelectedRow()].ID; got != "old-3" {
		t.Fatalf("highlight should follow its workflow, got %q at row %d", got, wl.table.SelectedRow())
	}
}

func TestPopulateTableShiftsScrollWhenPagesTrimFront(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	items := make([]temporal.Workflow, 30)
	for i := range items {
		items[i] = temporal.Workflow{ID: fmt.Sprintf("wf-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.workflows = items
	wl.allWorkflows = items
	wl.populateTable()
	wl.table.SelectRow(29)
	wl.table.SetOffset(20, 0)
	wl.rememberHighlightedWorkflow()

	wl.rememberListAnchor()
	wl.workflows = append([]temporal.Workflow{}, items[10:]...)
	wl.allWorkflows = wl.workflows
	wl.populateTable()

	row, _ := wl.table.GetOffset()
	if row != 10 {
		t.Fatalf("dropping earlier rows should pull the viewport with them, offset=%d", row)
	}
	if wl.table.SelectedRow() != 19 || wl.workflows[wl.table.SelectedRow()].ID != "wf-29" {
		t.Fatalf("should keep wf-29 highlighted, row=%d", wl.table.SelectedRow())
	}
}

func TestJumpWorkflowListEdgeStaysOnCurrentWindow(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	items := make([]temporal.Workflow, 40)
	for i := range items {
		items[i] = temporal.Workflow{ID: fmt.Sprintf("wf-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.workflows = items
	wl.allWorkflows = items
	wl.populateTable()
	wl.table.SelectRow(12)
	wl.rememberHighlightedWorkflow()

	wl.jumpWorkflowListEdge(true)
	if wl.table.SelectedRow() != 39 || wl.workflows[wl.table.SelectedRow()].ID != "wf-39" {
		t.Fatalf("G should highlight the last loaded workflow, row=%d", wl.table.SelectedRow())
	}
	if wl.listEdgePin != listEdgeEnd {
		t.Fatal("G should pin to the end so a sliding window cannot yank the highlight back")
	}

	wl.jumpWorkflowListEdge(false)
	if wl.table.SelectedRow() != 0 || wl.workflows[0].ID != "wf-0" {
		t.Fatalf("g should highlight the first loaded workflow, row=%d", wl.table.SelectedRow())
	}
	if wl.listEdgePin != listEdgeStart {
		t.Fatal("g should pin to the start")
	}
}

func TestPopulateTableKeepsEdgePinWhenWindowSlides(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	items := make([]temporal.Workflow, 20)
	for i := range items {
		items[i] = temporal.Workflow{ID: fmt.Sprintf("wf-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.workflows = items
	wl.allWorkflows = items
	wl.populateTable()
	wl.table.SelectRow(19)
	wl.table.SetOffset(10, 0)
	wl.rememberHighlightedWorkflow()
	wl.listEdgePin = listEdgeEnd

	next := make([]temporal.Workflow, 10)
	for i := range next {
		next[i] = temporal.Workflow{ID: fmt.Sprintf("more-%d", i), RunID: "r", Type: "T", Status: "Running"}
	}
	wl.workflows = append(append([]temporal.Workflow{}, items[10:]...), next...)
	wl.allWorkflows = wl.workflows
	wl.populateTable()

	if got := wl.workflows[wl.table.SelectedRow()].ID; got != "more-9" {
		t.Fatalf("G should stay on the new last workflow after a page slides, got %s row=%d", got, wl.table.SelectedRow())
	}

	wl.workflows = append(next, items[10:]...)
	wl.allWorkflows = wl.workflows
	wl.listEdgePin = listEdgeStart
	wl.highlightedWorkflowID = "wf-10"
	wl.highlightedRunID = "r"
	wl.populateTable()
	if got := wl.workflows[wl.table.SelectedRow()].ID; got != "more-0" {
		t.Fatalf("g should stay on the new first workflow after a page slides, got %s", got)
	}
}

func TestMaybeFetchPagesSkipsWhilePinnedToEdge(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.listEdgePin = listEdgeEnd
	wl.pager.reset("")
	wl.pager.accept(0, "", "next", []temporal.Workflow{{ID: "wf"}})
	wl.allWorkflows = wl.pager.items()
	wl.workflows = wl.allWorkflows
	wl.maybeFetchPages()
	if wl.pageBusy {
		t.Fatal("g/G should not slide the loaded window")
	}
}

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

func TestToggleAutoRefreshShowsStatus(t *testing.T) {
	a := &App{menu: layout.NewMenu()}
	wl := NewWorkflowList(a, "default")
	t.Cleanup(wl.stopAutoRefresh)

	wl.toggleAutoRefresh()
	if !wl.autoRefresh {
		t.Fatal("expected auto-refresh on")
	}
	if a.hintBarMessage() != "Auto-refresh on" {
		t.Fatalf("on: %q", a.hintBarMessage())
	}

	wl.toggleAutoRefresh()
	if wl.autoRefresh {
		t.Fatal("expected auto-refresh off")
	}
	if a.hintBarMessage() != "Auto-refresh off" {
		t.Fatalf("off: %q", a.hintBarMessage())
	}
}

func TestSelectModeSuspendsAutoRefresh(t *testing.T) {
	a := &App{menu: layout.NewMenu()}
	wl := NewWorkflowList(a, "default")
	t.Cleanup(wl.stopAutoRefresh)

	wl.toggleAutoRefresh()
	if wl.refreshTicker == nil {
		t.Fatal("auto-refresh should start a ticker")
	}

	wl.toggleSelectionMode()
	if !wl.selectionMode {
		t.Fatal("expected select mode")
	}
	if !wl.autoRefresh {
		t.Fatal("select mode should keep auto-refresh enabled")
	}
	if wl.refreshTicker != nil {
		t.Fatal("select mode should suspend the auto-refresh ticker")
	}

	wl.liveRefresh()
	if wl.liveBusy {
		t.Fatal("live refresh should no-op while select mode is on")
	}

	wl.toggleSelectionMode()
	if wl.selectionMode {
		t.Fatal("expected select mode off")
	}
	if wl.refreshTicker == nil {
		t.Fatal("leaving select mode should resume the auto-refresh ticker")
	}
}

func TestTaskQueueToggleAutoRefreshShowsStatus(t *testing.T) {
	a := &App{menu: layout.NewMenu()}
	tq := NewTaskQueueView(a)
	t.Cleanup(tq.stopAutoRefresh)

	tq.toggleAutoRefresh()
	if !tq.autoRefresh {
		t.Fatal("expected auto-refresh on")
	}
	if a.hintBarMessage() != "Auto-refresh on" {
		t.Fatalf("on: %q", a.hintBarMessage())
	}

	tq.toggleAutoRefresh()
	if tq.autoRefresh {
		t.Fatal("expected auto-refresh off")
	}
	if a.hintBarMessage() != "Auto-refresh off" {
		t.Fatalf("off: %q", a.hintBarMessage())
	}
}

func TestNamespaceToggleAutoRefreshShowsStatus(t *testing.T) {
	a := &App{menu: layout.NewMenu()}
	nl := NewNamespaceList(a)
	t.Cleanup(nl.stopAutoRefresh)

	nl.toggleAutoRefresh()
	if nl.autoRefresh {
		t.Fatal("expected auto-refresh off")
	}
	if a.hintBarMessage() != "Auto-refresh off" {
		t.Fatalf("off: %q", a.hintBarMessage())
	}

	nl.toggleAutoRefresh()
	if !nl.autoRefresh {
		t.Fatal("expected auto-refresh on")
	}
	if a.hintBarMessage() != "Auto-refresh on" {
		t.Fatalf("on: %q", a.hintBarMessage())
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
