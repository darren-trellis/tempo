package view

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func filteredTreeList(t *testing.T, provider temporal.Provider, workflows []temporal.Workflow) *WorkflowList {
	t.Helper()
	wl := treeTestList(t, provider, workflows)
	wl.visibilityQuery = "WorkflowType = 'Order' OR WorkflowType = 'Refund'"
	return wl
}

// A match's children are shown under it whatever the filter says, and their
// children under them in turn.
func TestFilteredTreeShowsEveryDescendantOfAMatch(t *testing.T) {
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
	wl := filteredTreeList(t, provider, []temporal.Workflow{{ID: "root", Status: "Completed"}, {ID: "other", Status: "Completed"}})
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

// A filter that matched a child, such as a running one, shows the child under
// its ancestors with its own descendants, but not the siblings that did not
// match.
func TestFilteredTreeShowsAMatchedChildsAncestorsButNotItsSiblings(t *testing.T) {
	var queries []string
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		queries = append(queries, opts.Query)
		children := strings.HasPrefix(opts.Query, "ParentWorkflowId")
		switch {
		case opts.Query == "WorkflowId IN ('parent')":
			return []temporal.Workflow{{ID: "parent", ParentID: strptr("grand"), Status: "Completed"}}, "", nil
		case opts.Query == "WorkflowId IN ('grand')":
			return []temporal.Workflow{{ID: "grand", Status: "Completed"}}, "", nil
		case children && strings.Contains(opts.Query, "'running'"):
			return []temporal.Workflow{{ID: "below", ParentID: strptr("running"), Status: "Completed"}}, "", nil
		case children && (strings.Contains(opts.Query, "'parent'") || strings.Contains(opts.Query, "'grand'")):
			t.Fatalf("an ancestor's other children should not be looked up, query=%q", opts.Query)
		}
		return nil, "", nil
	}}
	wl := treeTestList(t, provider, []temporal.Workflow{
		{ID: "running", ParentID: strptr("parent"), Status: "Running"},
		{ID: "other", Status: "Running"},
	})
	wl.visibilityQuery = "ExecutionStatus = 'Running'"
	wl.applyFilter()

	if got := treeIDs(wl.workflows); got != "grand,parent,running,below,other" {
		t.Fatalf("order=%s queries=%v", got, queries)
	}
	if wl.workflowDepths[3] != 3 {
		t.Fatalf("the match's child should nest under it, depths=%v", wl.workflowDepths)
	}
}

func TestTreeLooksUpNoChildrenWithoutAFilter(t *testing.T) {
	provider := &pageListProvider{list: func(context.Context, string, temporal.ListOptions) ([]temporal.Workflow, string, error) {
		t.Fatal("no lookup expected")
		return nil, "", nil
	}}
	wl := treeTestList(t, provider, []temporal.Workflow{{ID: "root"}})
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "root" {
		t.Fatalf("order=%s", got)
	}

	wl.visibilityQuery = "WorkflowType = 'Order'"
	wl.workflowTreeMode = false
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "root" {
		t.Fatalf("the flat list should show just the matches, order=%s", got)
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
	wl := filteredTreeList(t, nil, []temporal.Workflow{{ID: "root"}})
	wl.treeChildren.reset("ns")
	stale := wl.treeChildren.gen
	wl.invalidateCaches()
	wl.treeChildren.accept(stale, []string{"root"}, map[string][]temporal.Workflow{"root": {{ID: "c"}}}, nil)
	if len(wl.treeChildren.found) != 0 {
		t.Fatal("a lookup started before a refresh should be ignored")
	}
}

func TestSearchShowsNoChildrenOfItsMatches(t *testing.T) {
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

// relativesTestList filters for running workflows, one of which is the child
// of a parent that is not running, and notes every query the tree makes.
func relativesTestList(t *testing.T, showParents, showChildren bool) (*WorkflowList, *[]string) {
	t.Helper()
	var queries []string
	provider := &pageListProvider{list: func(_ context.Context, _ string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
		queries = append(queries, opts.Query)
		switch {
		case opts.Query == "WorkflowId IN ('parent')":
			return []temporal.Workflow{{ID: "parent", Status: "Completed"}}, "", nil
		case strings.HasPrefix(opts.Query, "ParentWorkflowId") && strings.Contains(opts.Query, "'running'"):
			return []temporal.Workflow{{ID: "below", ParentID: strptr("running"), Status: "Completed"}}, "", nil
		}
		return nil, "", nil
	}}
	wl := treeTestList(t, provider, []temporal.Workflow{
		{ID: "running", ParentID: strptr("parent"), Status: "Running"},
		{ID: "other", Status: "Running"},
	})
	wl.app.config = &config.Config{FilterShowParents: &showParents, FilterShowChildren: &showChildren}
	wl.visibilityQuery = "ExecutionStatus = 'Running'"
	return wl, &queries
}

func countQueries(queries []string, prefix string) int {
	n := 0
	for _, q := range queries {
		if strings.HasPrefix(q, prefix) {
			n++
		}
	}
	return n
}

func TestFilterShowParentsOffLeavesOutUnmatchedAncestors(t *testing.T) {
	wl, queries := relativesTestList(t, false, true)
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "running,below,other" {
		t.Fatalf("order=%s", got)
	}
	if wl.workflowDepths[0] != 0 || wl.workflowDepths[1] != 1 {
		t.Fatalf("the match should be a root with its child under it, depths=%v", wl.workflowDepths)
	}
	if n := countQueries(*queries, "WorkflowId IN"); n != 0 {
		t.Fatalf("no parent should be looked up, queries=%v", *queries)
	}
}

func TestFilterShowChildrenOffLeavesOutUnmatchedDescendants(t *testing.T) {
	wl, queries := relativesTestList(t, true, false)
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "parent,running,other" {
		t.Fatalf("order=%s", got)
	}
	if n := countQueries(*queries, "ParentWorkflowId"); n != 0 {
		t.Fatalf("no children should be looked up, queries=%v", *queries)
	}
}

func TestFilterWithoutRelativesShowsOnlyTheMatches(t *testing.T) {
	wl, queries := relativesTestList(t, false, false)
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "running,other" || len(*queries) != 0 {
		t.Fatalf("order=%s queries=%v", got, *queries)
	}
}

// With no filter every workflow is a match, so the settings do not hide a
// parent that is only on a page not loaded yet.
func TestFilterRelativeSettingsLeaveTheUnfilteredTreeAlone(t *testing.T) {
	wl, queries := relativesTestList(t, false, false)
	wl.visibilityQuery = ""
	wl.applyFilter()
	if got := treeIDs(wl.workflows); got != "parent,running,other" || countQueries(*queries, "WorkflowId IN") != 1 {
		t.Fatalf("order=%s queries=%v", got, *queries)
	}
}

func TestFilterRelativeSettingsDefaultOn(t *testing.T) {
	for _, name := range []string{"filter_show_parents", "filter_show_children"} {
		s, ok := lookupTempoSetting(name)
		if !ok {
			t.Fatalf("%s should be a setting", name)
		}
		if got := s.get(config.DefaultConfig()); got != "on" {
			t.Fatalf("%s should default on, got %q", name, got)
		}
	}
}
