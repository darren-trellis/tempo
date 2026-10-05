package view

import (
	"context"
	"strings"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

const (
	// rootsOnlyClause narrows a filter to workflows that no other workflow
	// started.
	rootsOnlyClause = "ParentWorkflowId IS NULL"
	// treeChildPageSize is how many children one page of a child lookup asks
	// for, and treeChildPages how many pages one batch of parents may take.
	treeChildPageSize = 1000
	treeChildPages    = 5
	// treeChildDepth is how many generations below a root are looked up.
	treeChildDepth = 16
)

// lookedUpChildren is the children a lookup found for one workflow ID.
type lookedUpChildren struct {
	children []temporal.Workflow
	fetched  time.Time
}

// childLookup caches the children of workflows, by workflow ID, for one
// namespace.
type childLookup struct {
	namespace string
	gen       uint64
	found     map[string]lookedUpChildren
	pending   map[string]bool
}

func (t *childLookup) reset(namespace string) {
	*t = childLookup{namespace: namespace, gen: t.gen + 1}
}

// due reports whether w's children should be fetched: they are not known, or
// w or one of them was still running when they were and that is now stale.
func (t *childLookup) due(w temporal.Workflow, now time.Time) bool {
	if t.pending[w.ID] {
		return false
	}
	entry, ok := t.found[w.ID]
	if !ok {
		return true
	}
	if now.Sub(entry.fetched) <= treeParentStale {
		return false
	}
	if w.Status == "Running" {
		return true
	}
	for _, c := range entry.children {
		if c.Status == "Running" {
			return true
		}
	}
	return false
}

// accept records what a lookup found and reports whether it changed anything
// worth a redraw. After an error the IDs may be asked for again.
func (t *childLookup) accept(gen uint64, ids []string, found map[string][]temporal.Workflow, err error) bool {
	if gen != t.gen {
		return false
	}
	for _, id := range ids {
		delete(t.pending, id)
	}
	if err != nil {
		return false
	}
	if t.found == nil {
		t.found = make(map[string]lookedUpChildren)
	}
	now := time.Now()
	changed := false
	for _, id := range ids {
		if len(found[id]) > 0 || len(t.found[id].children) > 0 {
			changed = true
		}
		t.found[id] = lookedUpChildren{children: found[id], fetched: now}
	}
	return changed
}

// rootsOnlyFilter reports whether the list's filter picks root workflows only,
// leaving their children to be looked up whatever the filter says. A / search
// that moved to the server is looking for a workflow, root or not, so it is
// left alone.
func (wl *WorkflowList) rootsOnlyFilter() bool {
	return wl != nil && wl.workflowTreeMode && wl.visibilityQuery != "" &&
		wl.app != nil && wl.app.Config().ShouldFilterRootsOnly() && !wl.searchOwnsQuery()
}

// listQuery is the query the server is asked for the list's pages and counts.
func (wl *WorkflowList) listQuery(resolved string) string {
	if !wl.rootsOnlyFilter() {
		return resolved
	}
	return andFilterQueries(resolved, rootsOnlyClause)
}

// withTreeRelatives adds the parents and, for a roots-only filter, the
// children of workflows that the tree needs and the list did not load.
func (wl *WorkflowList) withTreeRelatives(workflows []temporal.Workflow) []temporal.Workflow {
	return wl.withTreeChildren(wl.withTreeParents(workflows))
}

// withTreeChildren adds every descendant of workflows that has been fetched,
// and asks for the ones it does not have yet, when the filter picks roots
// only. The filter cannot match those children, so without this a root would
// show none.
func (wl *WorkflowList) withTreeChildren(workflows []temporal.Workflow) []temporal.Workflow {
	if !wl.rootsOnlyFilter() {
		return workflows
	}
	if wl.treeChildren.namespace != wl.namespace {
		wl.treeChildren.reset(wl.namespace)
	}
	present := make(map[string]bool, len(workflows))
	for _, w := range workflows {
		present[workflowIdentityKey(w)] = true
	}
	out := append([]temporal.Workflow(nil), workflows...)
	var fetch []string
	visited := make(map[string]bool)
	now := time.Now()

	var addChildren func(w temporal.Workflow, depth int)
	addChildren = func(w temporal.Workflow, depth int) {
		if depth > treeChildDepth || visited[w.ID] {
			return
		}
		visited[w.ID] = true
		if wl.treeChildren.due(w, now) {
			fetch = append(fetch, w.ID)
		}
		for _, c := range wl.treeChildren.found[w.ID].children {
			if c.ID == w.ID {
				continue
			}
			if key := workflowIdentityKey(c); !present[key] {
				present[key] = true
				out = append(out, c)
			}
			addChildren(c, depth+1)
		}
	}

	for _, w := range workflows {
		addChildren(w, 0)
	}
	if len(fetch) > 0 && wl.fetchTreeChildren(fetch) {
		return wl.withTreeChildren(workflows)
	}
	return out
}

// fetchTreeChildren looks the children of ids up and redraws the tree once
// they are in. It reports whether it already found some, which only happens
// without a running UI, where the lookup cannot wait for one.
func (wl *WorkflowList) fetchTreeChildren(ids []string) bool {
	if wl.app == nil || wl.app.Provider() == nil {
		return false
	}
	provider := wl.app.Provider()
	namespace := wl.namespace
	cache := &wl.treeChildren
	gen := cache.gen
	if cache.pending == nil {
		cache.pending = make(map[string]bool)
	}
	for _, id := range ids {
		cache.pending[id] = true
	}
	lookup := func() (map[string][]temporal.Workflow, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return fetchChildWorkflows(ctx, provider, namespace, ids)
	}
	if wl.app.JigApp() == nil {
		found, err := lookup()
		return cache.accept(gen, ids, found, err)
	}
	go func() {
		found, err := lookup()
		wl.app.JigApp().QueueUpdateDraw(func() {
			if cache.accept(gen, ids, found, err) && wl.workflowTreeMode {
				wl.applyFilter()
			}
		})
	}()
	return false
}

// fetchChildWorkflows returns the workflows each of ids started, by parent ID.
func fetchChildWorkflows(ctx context.Context, provider temporal.Provider, namespace string, ids []string) (map[string][]temporal.Workflow, error) {
	found := make(map[string][]temporal.Workflow, len(ids))
	for start := 0; start < len(ids); start += treeParentBatch {
		end := min(start+treeParentBatch, len(ids))
		quoted := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			quoted = append(quoted, quoteVisibilityValue(id))
		}
		query := "ParentWorkflowId IN (" + strings.Join(quoted, ", ") + ")"
		token := ""
		for page := 0; page < treeChildPages; page++ {
			workflows, next, err := provider.ListWorkflows(ctx, namespace, temporal.ListOptions{
				PageSize:  treeChildPageSize,
				PageToken: token,
				Query:     query,
			})
			if err != nil {
				return found, err
			}
			for _, w := range workflows {
				if w.ParentID != nil {
					found[*w.ParentID] = append(found[*w.ParentID], w)
				}
			}
			if next == "" {
				break
			}
			token = next
		}
	}
	return found, nil
}
