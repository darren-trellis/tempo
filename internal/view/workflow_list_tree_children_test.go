package view

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func rootsOnlyTestList(t *testing.T, provider temporal.Provider, workflows []temporal.Workflow) *WorkflowList {
	t.Helper()
	wl := treeTestList(t, provider, workflows)
	on := true
	wl.app.config = &config.Config{FilterRootsOnly: &on}
	wl.visibilityQuery = "WorkflowType = 'Order' OR WorkflowType = 'Refund'"
	return wl
}

func TestRootsOnlyFilterAsksTheServerForRoots(t *testing.T) {
	wl := rootsOnlyTestList(t, nil, nil)
	got := wl.listQuery(wl.visibilityQuery)
	want := "(WorkflowType = 'Order' OR WorkflowType = 'Refund') AND ParentWorkflowId IS NULL"
	if got != want {
		t.Fatalf("query=%q", got)
	}

	wl.workflowTreeMode = false
	if got := wl.listQuery(wl.visibilityQuery); got != wl.visibilityQuery {
		t.Fatalf("the flat list should filter every workflow, query=%q", got)
	}
	wl.workflowTreeMode = true
	wl.visibilityQuery = ""
	if got := wl.listQuery(""); got != "" {
		t.Fatalf("no filter should stay no filter, query=%q", got)
	}
	wl.visibilityQuery = workflowIDFilterQuery("child-")
	if got := wl.listQuery(wl.visibilityQuery); got != wl.visibilityQuery {
		t.Fatalf("a search moved to the server should find children too, query=%q", got)
	}
}

func TestRootsOnlyFilterIsOffByDefault(t *testing.T) {
	wl := treeTestList(t, nil, nil)
	wl.visibilityQuery = "WorkflowType = 'Order'"
	if got := wl.listQuery(wl.visibilityQuery); got != wl.visibilityQuery {
		t.Fatalf("query=%q", got)
	}
}

// A root's children are shown under it whatever the filter says, and their
// children under them in turn.
func TestRootsOnlyFilterShowsEveryDescendant(t *testing.T) {
	base := time.Now()
	var queries []string
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		queries = append(queries, opts.Query)
		switch {
		case strings.Contains(opts.Query, "'root'"):
			return []temporal.Workflow{
				{ID: "late", ParentID: strptr("root"), StartTime: base.Add(2 * time.Second)},
				{ID: "early", ParentID: strptr("root"), StartTime: base.Add(time.Second)},
			}, "", nil
		case strings.Contains(opts.Query, "'early'"):
			return []temporal.Workflow{{ID: "grandchild", ParentID: strptr("early")}}, "", nil
		}
		return nil, "", nil
	}}
	wl := rootsOnlyTestList(t, provider, []temporal.Workflow{{ID: "root", Status: "Completed"}, {ID: "other", Status: "Completed"}})
	wl.applyFilter()

	if got := treeIDs(wl.workflows); got != "root,early,grandchild,late,other" {
		t.Fatalf("order=%s", got)
	}
	if wl.workflowDepths[2] != 2 || wl.workflowDepths[3] != 1 {
		t.Fatalf("depths=%v", wl.workflowDepths)
	}
	if !strings.HasPrefix(queries[0], "ParentWorkflowId IN ('root', 'other')") {
		t.Fatalf("queries=%v", queries)
	}
	asked := len(queries)
	wl.applyFilter()
	if len(queries) != asked {
		t.Fatalf("closed workflows' children should not be asked for again, queries=%v", queries)
	}
}

func TestChildrenAreNotLookedUpWithoutRootsOnly(t *testing.T) {
	provider := &pageListProvider{list: func(context.Context, string, temporal.ListOptions) ([]temporal.Workflow, string, error) {
		t.Fatal("no lookup expected")
		return nil, "", nil
	}}
	wl := treeTestList(t, provider, []temporal.Workflow{{ID: "root"}})
	wl.visibilityQuery = "WorkflowType = 'Order'"
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "root" {
		t.Fatalf("order=%s", got)
	}
}

func TestFetchChildWorkflowsFollowsPages(t *testing.T) {
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		if opts.PageToken == "" {
			return []temporal.Workflow{{ID: "a", ParentID: strptr("p")}}, "next", nil
		}
		return []temporal.Workflow{{ID: "b", ParentID: strptr("p")}}, "", nil
	}}
	found, err := fetchChildWorkflows(context.Background(), provider, "ns", []string{"p"})
	if err != nil {
		t.Fatal(err)
	}
	if got := treeIDs(found["p"]); got != "a,b" {
		t.Fatalf("children=%s", got)
	}
}

func TestChildLookupsFromAnOldListAreDropped(t *testing.T) {
	wl := rootsOnlyTestList(t, nil, []temporal.Workflow{{ID: "root"}})
	wl.treeChildren.reset("ns")
	stale := wl.treeChildren.gen
	wl.invalidateCaches()
	wl.treeChildren.accept(stale, []string{"root"}, map[string][]temporal.Workflow{"root": {{ID: "c"}}}, nil)
	if len(wl.treeChildren.found) != 0 {
		t.Fatal("a lookup started before a refresh should be ignored")
	}
}

// A filter that matched only some of a parent's children, such as the running
// ones, fetches the parent, and the parent then shows all of its children.
func TestFetchedParentShowsEveryChild(t *testing.T) {
	var queries []string
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		queries = append(queries, opts.Query)
		switch {
		case opts.Query == "WorkflowId IN ('parent')":
			return []temporal.Workflow{{ID: "parent", Status: "Completed"}}, "", nil
		case opts.Query == "ParentWorkflowId IN ('parent')":
			return []temporal.Workflow{
				{ID: "running", ParentID: strptr("parent"), Status: "Running"},
				{ID: "done", ParentID: strptr("parent"), Status: "Completed"},
			}, "", nil
		case strings.HasPrefix(opts.Query, "ParentWorkflowId") && strings.Contains(opts.Query, "'done'"):
			return []temporal.Workflow{{ID: "grandchild", ParentID: strptr("done"), Status: "Completed"}}, "", nil
		}
		return nil, "", nil
	}}
	wl := treeTestList(t, provider, []temporal.Workflow{
		{ID: "running", ParentID: strptr("parent"), Status: "Running"},
		{ID: "other", Status: "Running"},
	})
	wl.visibilityQuery = "ExecutionStatus = 'Running'"
	wl.applyFilter()

	got := treeIDs(wl.workflows)
	for _, id := range []string{"parent", "running", "done", "grandchild", "other"} {
		if !strings.Contains(","+got+",", ","+id+",") {
			t.Fatalf("%s should show, order=%s", id, got)
		}
	}
	if strings.Count(got, "running") != 1 {
		t.Fatalf("a loaded child should show once, order=%s", got)
	}
	for _, q := range queries {
		if strings.Contains(q, "'other'") {
			t.Fatalf("only the fetched parent's family should be looked up, queries=%v", queries)
		}
	}
}

func TestSearchDoesNotShowAFetchedParentsOtherChildren(t *testing.T) {
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		if strings.HasPrefix(opts.Query, "ParentWorkflowId") {
			t.Fatalf("no child lookup expected, query=%q", opts.Query)
		}
		if opts.Query == "WorkflowId IN ('parent')" {
			return []temporal.Workflow{{ID: "parent", Status: "Completed"}}, "", nil
		}
		return nil, "", nil
	}}
	wl := treeTestList(t, provider, []temporal.Workflow{{ID: "child-1", ParentID: strptr("parent")}})
	wl.visibilityQuery = workflowIDFilterQuery("child-")
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "parent,child-1" {
		t.Fatalf("order=%s", got)
	}
}
