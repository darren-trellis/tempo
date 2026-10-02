package view

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func strptr(s string) *string { return &s }

func treeTestList(t *testing.T, provider temporal.Provider, workflows []temporal.Workflow) *WorkflowList {
	t.Helper()
	wl := NewWorkflowList(&App{provider: provider}, "ns")
	wl.workflowTreeMode = true
	wl.allWorkflows = workflows
	return wl
}

func treeIDs(workflows []temporal.Workflow) string {
	out := make([]string, len(workflows))
	for i, w := range workflows {
		out[i] = w.ID
	}
	return strings.Join(out, ",")
}

// The list is ordered by close time, so a parent that started a child and
// finished at once lands pages below it. Once fetched it nests the child
// where the child was, not at the end of the list.
func TestTreeShowsAFetchedParentWhereItsChildWas(t *testing.T) {
	workflows := []temporal.Workflow{
		{ID: "newest"},
		{ID: "child", ParentID: strptr("parent")},
		{ID: "oldest"},
	}
	wl := treeTestList(t, nil, workflows)
	wl.treeParents.reset("ns")
	wl.acceptTreeParents(wl.treeParents.gen, []string{"parent"}, map[string]temporal.Workflow{
		"parent": {ID: "parent", Status: "Completed"},
	}, nil)
	wl.applyFilter()

	if got := treeIDs(wl.workflows); got != "newest,parent,child,oldest" {
		t.Fatalf("order=%s", got)
	}
	if wl.workflowDepths[2] != 1 || !wl.workflowHasChildAt(1) {
		t.Fatalf("the child should nest under its parent, depths=%v", wl.workflowDepths)
	}
}

func TestTreeDoesNotDuplicateALoadedParent(t *testing.T) {
	workflows := []temporal.Workflow{
		{ID: "child", ParentID: strptr("parent")},
		{ID: "parent", Status: "Completed"},
	}
	wl := treeTestList(t, nil, workflows)
	wl.treeParents.reset("ns")
	wl.acceptTreeParents(wl.treeParents.gen, []string{"parent"}, map[string]temporal.Workflow{
		"parent": {ID: "parent", Status: "Completed"},
	}, nil)
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "parent,child" {
		t.Fatalf("a loaded parent should appear once, order=%s", got)
	}
}

func TestTreeChainsFetchedGrandparents(t *testing.T) {
	wl := treeTestList(t, nil, []temporal.Workflow{{ID: "child", ParentID: strptr("parent")}})
	wl.treeParents.reset("ns")
	wl.acceptTreeParents(wl.treeParents.gen, []string{"parent", "grand"}, map[string]temporal.Workflow{
		"parent": {ID: "parent", ParentID: strptr("grand")},
		"grand":  {ID: "grand"},
	}, nil)
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "grand,parent,child" || wl.workflowDepths[2] != 2 {
		t.Fatalf("order=%s depths=%v", got, wl.workflowDepths)
	}
}

func TestTreeLooksUpMissingParents(t *testing.T) {
	var queries []string
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		queries = append(queries, opts.Query)
		if strings.Contains(opts.Query, "'p'") {
			return []temporal.Workflow{{ID: "p", Status: "Completed"}}, "", nil
		}
		return nil, "", nil
	}}
	wl := treeTestList(t, provider, []temporal.Workflow{
		{ID: "a", ParentID: strptr("p")},
		{ID: "b", ParentID: strptr("p")},
		{ID: "orphan", ParentID: strptr("deleted")},
	})
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "p,a,b,orphan" {
		t.Fatalf("the looked-up parent should nest its children, order=%s", got)
	}
	queriesAfterFirst := len(queries)
	wl.applyFilter()
	wl.applyFilter()
	if len(queries) != queriesAfterFirst {
		t.Fatalf("found and unknown parents should not be asked for again, queries=%v", queries)
	}
}

func TestTreeParentsFromAnOldListAreDropped(t *testing.T) {
	wl := treeTestList(t, nil, []temporal.Workflow{{ID: "child", ParentID: strptr("parent")}})
	wl.treeParents.reset("ns")
	stale := wl.treeParents.gen
	wl.invalidateCaches()
	wl.acceptTreeParents(stale, []string{"parent"}, map[string]temporal.Workflow{"parent": {ID: "parent"}}, nil)
	if len(wl.treeParents.found) != 0 {
		t.Fatal("a lookup started before a refresh should be ignored")
	}
}

func TestFetchWorkflowsByIDBatchesAndKeepsNewestRun(t *testing.T) {
	var queries []string
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		queries = append(queries, opts.Query)
		if strings.Contains(opts.Query, "'id-0'") {
			return []temporal.Workflow{{ID: "id-0", RunID: "newest"}, {ID: "id-0", RunID: "older"}}, "", nil
		}
		return nil, "", nil
	}}
	var want []string
	for i := 0; i < treeParentBatch+5; i++ {
		want = append(want, fmt.Sprintf("id-%d", i))
	}
	want = append(want, "it's")
	found, err := fetchWorkflowsByID(context.Background(), provider, "ns", want)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 {
		t.Fatalf("%d IDs should take two queries, got %d", len(want), len(queries))
	}
	if !strings.HasPrefix(queries[0], "WorkflowId IN ('id-0', 'id-1'") || !strings.Contains(queries[1], "'it''s'") {
		t.Fatalf("queries=%v", queries)
	}
	if found["id-0"].RunID != "newest" {
		t.Fatalf("the newest run should win, got %+v", found["id-0"])
	}
}
