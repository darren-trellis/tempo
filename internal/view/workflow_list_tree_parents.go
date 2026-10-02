package view

import (
	"context"
	"strings"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

const (
	// treeParentBatch is how many missing parents one visibility query asks
	// for.
	treeParentBatch = 50
	// treeParentStale is how old a fetched parent that was still running may
	// get before it is fetched again, so its status keeps up.
	treeParentStale = 5 * time.Second
)

// treeParent is a parent workflow fetched for the tree because the list had
// not loaded it.
type treeParent struct {
	workflow temporal.Workflow
	fetched  time.Time
}

// treeParents holds the parents fetched for one namespace.
type treeParents struct {
	namespace string
	gen       uint64
	found     map[string]treeParent
	missing   map[string]bool // asked for, and the server has no such workflow
	pending   map[string]bool
}

func (t *treeParents) reset(namespace string) {
	*t = treeParents{namespace: namespace, gen: t.gen + 1}
}

// withTreeParents puts each workflow's unloaded parent, when it has been
// fetched, just ahead of its first child, and asks for the ones it does not
// have yet. The list is ordered by close time, so a parent that started a
// child and finished at once sits far below the child, often on a page that
// is not loaded, and the child would otherwise show with no parent. A text
// search hoists its matches to the top on purpose, so it is left alone.
func (wl *WorkflowList) withTreeParents(workflows []temporal.Workflow) []temporal.Workflow {
	if wl.filterText != "" {
		return workflows
	}
	if wl.treeParents.namespace != wl.namespace {
		wl.treeParents.reset(wl.namespace)
	}
	present := make(map[string]bool, len(workflows))
	for _, w := range workflows {
		present[w.ID] = true
	}
	out := make([]temporal.Workflow, 0, len(workflows))
	var fetch []string
	queued := make(map[string]bool)
	now := time.Now()

	var addAncestors func(id string, depth int)
	addAncestors = func(id string, depth int) {
		if id == "" || present[id] || depth > 16 {
			return
		}
		parent, ok := wl.treeParents.found[id]
		if !ok || (parent.workflow.Status == "Running" && now.Sub(parent.fetched) > treeParentStale) {
			if !wl.treeParents.missing[id] && !wl.treeParents.pending[id] && !queued[id] {
				queued[id] = true
				fetch = append(fetch, id)
			}
		}
		if !ok {
			return
		}
		if p := parent.workflow.ParentID; p != nil && *p != id {
			addAncestors(*p, depth+1)
		}
		present[id] = true
		out = append(out, parent.workflow)
	}

	for _, w := range workflows {
		if w.ParentID != nil && *w.ParentID != w.ID {
			addAncestors(*w.ParentID, 0)
		}
		out = append(out, w)
	}
	if len(fetch) > 0 && wl.fetchTreeParents(fetch) {
		return wl.withTreeParents(workflows)
	}
	return out
}

// fetchTreeParents looks the parents up by ID and redraws the tree once they
// are in. It reports whether it already found some, which only happens
// without a running UI, where the lookup cannot wait for one.
func (wl *WorkflowList) fetchTreeParents(ids []string) bool {
	if wl.app == nil || wl.app.Provider() == nil {
		return false
	}
	provider := wl.app.Provider()
	namespace := wl.namespace
	gen := wl.treeParents.gen
	if wl.treeParents.pending == nil {
		wl.treeParents.pending = make(map[string]bool)
	}
	for _, id := range ids {
		wl.treeParents.pending[id] = true
	}
	lookup := func() (map[string]temporal.Workflow, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return fetchWorkflowsByID(ctx, provider, namespace, ids)
	}
	// Without a running UI there is no thread to hand the result back to.
	if wl.app.JigApp() == nil {
		found, err := lookup()
		return wl.acceptTreeParents(gen, ids, found, err)
	}
	go func() {
		found, err := lookup()
		wl.app.JigApp().QueueUpdateDraw(func() {
			if wl.acceptTreeParents(gen, ids, found, err) && wl.workflowTreeMode {
				wl.applyFilter()
			}
		})
	}()
	return false
}

// acceptTreeParents records what a lookup found and reports whether the tree
// has anything new to show. IDs it did not find are not asked for again;
// after an error they may be.
func (wl *WorkflowList) acceptTreeParents(gen uint64, ids []string, found map[string]temporal.Workflow, err error) bool {
	if gen != wl.treeParents.gen {
		return false
	}
	if wl.treeParents.found == nil {
		wl.treeParents.found = make(map[string]treeParent)
	}
	if wl.treeParents.missing == nil {
		wl.treeParents.missing = make(map[string]bool)
	}
	now := time.Now()
	for _, id := range ids {
		delete(wl.treeParents.pending, id)
		if w, ok := found[id]; ok {
			wl.treeParents.found[id] = treeParent{workflow: w, fetched: now}
		} else if err == nil {
			wl.treeParents.missing[id] = true
		}
	}
	return len(found) > 0
}

// fetchWorkflowsByID returns the latest run of each workflow ID that exists.
func fetchWorkflowsByID(ctx context.Context, provider temporal.Provider, namespace string, ids []string) (map[string]temporal.Workflow, error) {
	found := make(map[string]temporal.Workflow, len(ids))
	for start := 0; start < len(ids); start += treeParentBatch {
		end := min(start+treeParentBatch, len(ids))
		quoted := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			quoted = append(quoted, quoteVisibilityValue(id))
		}
		workflows, _, err := provider.ListWorkflows(ctx, namespace, temporal.ListOptions{
			// Each ID can have several runs; the newest comes first.
			PageSize: 4 * (end - start),
			Query:    "WorkflowId IN (" + strings.Join(quoted, ", ") + ")",
		})
		if err != nil {
			return found, err
		}
		for _, w := range workflows {
			if _, seen := found[w.ID]; !seen {
				found[w.ID] = w
			}
		}
	}
	return found, nil
}
