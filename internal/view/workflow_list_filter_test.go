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
	if got := a.prompt().input.GetText(); got != "" {
		t.Fatalf("/ should open an empty prompt, got %q", got)
	}
	if ev := a.prompt().capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Fatal("esc should close the filter prompt")
	}
	if a.IsFilterMode() {
		t.Fatal("esc should leave filter mode")
	}
	if wl.filterText != "missing-id" || wl.visibilityQuery != workflowIDFilterQuery("missing-id") {
		t.Fatalf("esc in the prompt should keep the search, filter=%q query=%q", wl.filterText, wl.visibilityQuery)
	}

	if !wl.HandleEscape() {
		t.Fatal("esc on the workflows tab should clear the search")
	}
	if wl.filterText != "" || wl.visibilityQuery != "" {
		t.Fatalf("esc should clear the search, filter=%q query=%q", wl.filterText, wl.visibilityQuery)
	}
	if len(wl.allWorkflows) == 1 && wl.allWorkflows[0].ID == "only-match" {
		t.Fatal("esc should not restore the filtered snapshot")
	}
}

func submitPrompt(t *testing.T, a *App, text string) {
	t.Helper()
	a.prompt().input.SetText(text)
	if ev := a.prompt().capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Fatal("enter should submit the search")
	}
}

func cancelPrompt(t *testing.T, a *App) {
	t.Helper()
	if ev := a.prompt().capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Fatal("esc should close the search")
	}
}

func TestSearchPromptStartsEmptyAndRestoresOnEscape(t *testing.T) {
	a := &App{}
	query := "charge"
	var applied []string
	apply := func(text string) {
		query = text
		applied = append(applied, text)
	}

	a.ShowSearchPrompt(query, apply, nil)
	if got := a.prompt().input.GetText(); got != "" {
		t.Fatalf("prompt should start empty, got %q", got)
	}
	cancelPrompt(t, a)
	if query != "charge" || len(applied) != 0 {
		t.Fatalf("esc with nothing typed should leave the search alone, query=%q applied=%q", query, applied)
	}

	a.ShowSearchPrompt(query, apply, nil)
	a.prompt().input.SetText("refund")
	cancelPrompt(t, a)
	if query != "charge" {
		t.Fatalf("esc should put the previous search back, got %q", query)
	}

	a.ShowSearchPrompt(query, apply, nil)
	submitPrompt(t, a, "refund")
	if query != "refund" {
		t.Fatalf("enter should apply the search, got %q", query)
	}
}

func TestSearchPromptKeepsTheSavedFilterAndSearch(t *testing.T) {
	a := &App{}
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	wl.loadMockData()
	wl.applySavedFilter(config.SavedFilter{Name: "Running", Query: "ExecutionStatus = 'Running'"})

	wl.showFilter()
	submitPrompt(t, a, "payment")
	if wl.filterText != "payment" || len(wl.workflows) == 0 {
		t.Fatalf("search should apply, filter=%q rows=%d", wl.filterText, len(wl.workflows))
	}

	wl.showFilter()
	if got := a.prompt().input.GetText(); got != "" {
		t.Fatalf("/ should open an empty prompt, got %q", got)
	}
	cancelPrompt(t, a)
	if wl.activeFilterName != "Running" || wl.visibilityQuery != "ExecutionStatus = 'Running'" {
		t.Fatalf("saved filter should stay, name=%q query=%q", wl.activeFilterName, wl.visibilityQuery)
	}
	if wl.filterText != "payment" {
		t.Fatalf("esc in the prompt should keep the search, got %q", wl.filterText)
	}

	wl.showFilter()
	a.prompt().input.SetText("fulfill")
	if wl.filterText != "fulfill" {
		t.Fatalf("typing should preview the search, got %q", wl.filterText)
	}
	cancelPrompt(t, a)
	if wl.filterText != "payment" || len(wl.workflows) == 0 || wl.workflows[0].ID != "payment-xyz789" {
		t.Fatalf("esc should put the previous search back, filter=%q rows=%v", wl.filterText, workflowIDs(wl.workflows))
	}

	wl.showFilter()
	submitPrompt(t, a, "")
	if wl.filterText != "payment" || wl.activeFilterName != "Running" {
		t.Fatalf("an empty enter should keep the search, filter=%q name=%q", wl.filterText, wl.activeFilterName)
	}

	if !wl.HandleEscape() {
		t.Fatal("esc on the workflows tab should clear the search")
	}
	if wl.filterText != "" {
		t.Fatalf("search should clear, got %q", wl.filterText)
	}
	if wl.activeFilterName != "Running" || wl.visibilityQuery != "ExecutionStatus = 'Running'" {
		t.Fatalf("clearing the search should keep the saved filter, name=%q query=%q", wl.activeFilterName, wl.visibilityQuery)
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

func TestEscapeKeepsTheSelectedSavedFilter(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.loadMockData()
	wl.applySavedFilter(config.SavedFilter{Name: "Running", Query: "ExecutionStatus = 'Running'"})

	if wl.HandleEscape() {
		t.Fatal("escape should not treat a saved filter as something to clear")
	}
	if wl.activeFilterName != "Running" || wl.visibilityQuery != "ExecutionStatus = 'Running'" {
		t.Fatalf("saved filter should stay, name=%q query=%q", wl.activeFilterName, wl.visibilityQuery)
	}
}

func TestEscapeClearsAnUnsavedVisibilityQuery(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.loadMockData()
	wl.applyVisibilityQuery("WorkflowType = 'Order'")

	if !wl.HandleEscape() {
		t.Fatal("escape should clear an unsaved query")
	}
	if wl.visibilityQuery != "" || wl.activeFilterName != "" {
		t.Fatalf("unsaved query should clear, name=%q query=%q", wl.activeFilterName, wl.visibilityQuery)
	}
}

func TestEscapePeelsAdHocClausesOffASavedFilter(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.loadMockData()
	wl.applySavedFilter(config.SavedFilter{Name: "Running", Query: "ExecutionStatus = 'Running'"})
	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})

	if !wl.HandleEscape() {
		t.Fatal("escape should peel the ad-hoc clauses")
	}
	if wl.adHoc.active() {
		t.Fatal("ad-hoc clauses should be gone")
	}
	if wl.activeFilterName != "Running" || wl.visibilityQuery != "ExecutionStatus = 'Running'" {
		t.Fatalf("the saved filter should remain, name=%q query=%q", wl.activeFilterName, wl.visibilityQuery)
	}
}
