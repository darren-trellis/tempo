package view

import (
	"testing"
	"time"

	"github.com/atterpac/jig/layout"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func init() {
	filterChangeDelay = 0
}

func TestFilterPromptReceivesKeys(t *testing.T) {
	a := &App{}
	a.ShowFilterMode("", FilterModeCallbacks{})
	if !a.handlePromptKey(tcell.NewEventKey(tcell.KeyRune, 'p', tcell.ModNone)) {
		t.Fatal("filter should consume typed keys")
	}
	if !a.handlePromptKey(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone)) {
		t.Fatal("filter should consume typed keys")
	}
	if got := a.prompt().input.GetText(); got != "pa" {
		t.Fatalf("typed %q", got)
	}
}

func TestFilterSubmitDoesNotReplayEmptyChange(t *testing.T) {
	a := &App{
		app: layout.NewApp(layout.AppConfig{}),
	}
	var changes []string
	var submitted string
	a.ShowFilterMode("", FilterModeCallbacks{
		OnChange: func(text string) { changes = append(changes, text) },
		OnSubmit: func(text string) { submitted = text },
	})

	input := a.prompt().input
	input.SetText("pay")
	if capture := input.GetInputCapture(); capture != nil {
		if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
			t.Fatal("enter should submit the filter")
		}
	} else {
		t.Fatal("expected command input capture")
	}

	if submitted != "pay" {
		t.Fatalf("submitted %q", submitted)
	}
	for _, change := range changes {
		if change == "" {
			t.Fatalf("submit should not fire OnChange(\"\"): %q", changes)
		}
	}
}

func TestFilterChangeDebouncesPaste(t *testing.T) {
	a := &App{}
	var calls []string
	orig := filterChangeDelay
	filterChangeDelay = 20 * time.Millisecond
	t.Cleanup(func() { filterChangeDelay = orig })

	a.ShowFilterMode("", FilterModeCallbacks{
		OnChange: func(text string) { calls = append(calls, text) },
	})
	onChange := func(text string) { calls = append(calls, text) }
	a.scheduleFilterChange("w", onChange)
	a.scheduleFilterChange("wf", onChange)
	a.scheduleFilterChange("wf-123", onChange)
	if len(calls) != 0 {
		t.Fatalf("paste should not apply on every character, got %v", calls)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && len(calls) == 0 {
		time.Sleep(5 * time.Millisecond)
		if a.filterDebounce == nil && len(calls) == 0 {
			break
		}
	}
	if len(calls) != 1 || calls[0] != "wf-123" {
		t.Fatalf("paste should apply once, got %v", calls)
	}
}

func TestWorkflowMatchesFilterIDOnly(t *testing.T) {
	w := temporal.Workflow{ID: "payment-xyz789", Type: "OrderWorkflow", Status: "Running", TaskFailure: true}
	if !workflowMatchesFilter(w, "pay") || !workflowMatchesFilter(w, "xyz") {
		t.Fatal("should match substrings of the workflow id")
	}
	if workflowMatchesFilter(w, "order") || workflowMatchesFilter(w, "running") || workflowMatchesFilter(w, "unhandled") {
		t.Fatal("should not match type or status")
	}
}

func TestFilterTreeKeepsDescendantsAndHoistsMatch(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	wl.filterText = "payment"
	wl.applyFilter()

	if len(wl.workflows) != 2 {
		t.Fatalf("payment match should keep its child, got %v", workflowIDs(wl.workflows))
	}
	if wl.workflows[0].ID != "payment-xyz789" || wl.workflowDepth(0) != 0 {
		t.Fatalf("matched workflow should be the result root, got %s depth %d", wl.workflows[0].ID, wl.workflowDepth(0))
	}
	if wl.workflows[1].ID != "fulfillment-ghi000" || wl.workflowDepth(1) != 1 {
		t.Fatalf("child should stay nested, got %s depth %d", wl.workflows[1].ID, wl.workflowDepth(1))
	}
	if wl.workflows[0].ParentID == nil || *wl.workflows[0].ParentID != "order-processing-abc123" {
		t.Fatal("hoisted match should still carry its real parent id")
	}

	foundParent := false
	for _, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnParentID {
			foundParent = true
			break
		}
	}
	if !foundParent {
		t.Fatal("tree filter should show the parent id column")
	}

	wl.filterText = "fulfillment"
	wl.applyFilter()
	if len(wl.workflows) != 1 || wl.workflows[0].ID != "fulfillment-ghi000" || wl.workflowDepth(0) != 0 {
		t.Fatalf("leaf match should hoist alone, got %v depth %d", workflowIDs(wl.workflows), wl.workflowDepth(0))
	}

	wl.workflowTreeMode = false
	wl.filterText = "payment"
	wl.applyFilter()
	if len(wl.workflows) != 1 || wl.workflows[0].ID != "payment-xyz789" {
		t.Fatalf("list filter should stay id-only, got %v", workflowIDs(wl.workflows))
	}
}

func TestWorkflowIDFilterQuery(t *testing.T) {
	if got := workflowIDFilterQuery("pay"); got != "WorkflowId STARTS_WITH 'pay'" {
		t.Fatalf("query: %q", got)
	}
	if got := searchTermFromVisibilityQuery(workflowIDFilterQuery("pay")); got != "pay" {
		t.Fatalf("term: %q", got)
	}
	if searchTermFromVisibilityQuery("ExecutionStatus = 'Running'") != "" {
		t.Fatal("complex queries should not look like a / search")
	}
}

func TestConvertedSearchStaysEditableAndClearsOnEscape(t *testing.T) {
	a := &App{}
	wl := NewWorkflowList(a, "default")
	wl.loadMockData()
	wl.filterText = "missing-id"
	wl.convertFilterToVisibilityQuery()
	if wl.filterText != "missing-id" {
		t.Fatalf("search term should stay visible, got %q", wl.filterText)
	}
	if wl.visibilityQuery != workflowIDFilterQuery("missing-id") {
		t.Fatalf("query=%q", wl.visibilityQuery)
	}

	wl.allWorkflows = []temporal.Workflow{{ID: "only-match"}}
	wl.originalWorkflows = wl.allWorkflows
	wl.showFilter()
	if got := a.prompt().input.GetText(); got != "missing-id" {
		t.Fatalf("/ should reopen the search term, got %q", got)
	}
	if ev := a.prompt().capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Fatal("esc should close the filter prompt")
	}
	if a.IsFilterMode() {
		t.Fatal("esc should leave filter mode")
	}
	if wl.filterText != "" || wl.visibilityQuery != "" {
		t.Fatalf("esc should clear the search, filter=%q query=%q", wl.filterText, wl.visibilityQuery)
	}
	if len(wl.allWorkflows) == 1 && wl.allWorkflows[0].ID == "only-match" {
		t.Fatal("esc should not restore the filtered snapshot")
	}
}

func TestEmptyFilterSubmitClearsConvertedQuery(t *testing.T) {
	a := &App{}
	wl := NewWorkflowList(a, "default")
	wl.loadMockData()
	wl.filterText = "missing-id"
	wl.convertFilterToVisibilityQuery()
	wl.commitFilter("")
	if wl.filterText != "" || wl.visibilityQuery != "" {
		t.Fatalf("empty submit should clear, filter=%q query=%q", wl.filterText, wl.visibilityQuery)
	}
}

func TestNamespaceEscapeClearsSearch(t *testing.T) {
	nl := NewNamespaceList(&App{})
	nl.allNamespaces = []temporal.Namespace{{Name: "default"}, {Name: "prod"}}
	nl.namespaces = nl.allNamespaces
	nl.SetSearchText("prod")
	nl.applyFilter("prod")
	if len(nl.namespaces) != 1 {
		t.Fatalf("filtered=%d", len(nl.namespaces))
	}
	if !nl.HandleEscape() {
		t.Fatal("escape should clear namespace search")
	}
	if nl.GetSearchText() != "" || len(nl.namespaces) != 2 {
		t.Fatalf("search=%q n=%d", nl.GetSearchText(), len(nl.namespaces))
	}
}

func TestTaskQueueEscapeClearsSearch(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.setListKind(listTaskQueues)
	wl.taskQueues.allQueues = []taskQueueEntry{{Name: "orders"}, {Name: "payments"}}
	wl.taskQueues.applyFilter("pay")
	if len(wl.taskQueues.queues) != 1 {
		t.Fatalf("filtered=%d", len(wl.taskQueues.queues))
	}
	if !wl.HandleEscape() {
		t.Fatal("escape should clear task queue search")
	}
	if wl.taskQueues.searchText != "" || len(wl.taskQueues.queues) != 2 {
		t.Fatalf("search=%q n=%d", wl.taskQueues.searchText, len(wl.taskQueues.queues))
	}
}
