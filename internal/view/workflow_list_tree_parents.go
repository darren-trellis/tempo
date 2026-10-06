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

// lookedUpWorkflow is a workflow fetched by ID, such as a tree parent the
// list had not loaded.
type lookedUpWorkflow struct {
	workflow temporal.Workflow
	fetched  time.Time
}

// workflowLookup caches workflows fetched by ID for one namespace.
type workflowLookup struct {
	namespace string
	gen       uint64
	found     map[string]lookedUpWorkflow
	missing   map[string]bool // asked for, and the server has no such workflow
	pending   map[string]bool
}

func (t *workflowLookup) reset(namespace string) {
	*t = workflowLookup{namespace: namespace, gen: t.gen + 1}
}

// due reports whether id should be fetched: it is not known, or it was still
// running when fetched and that is now stale, and it is neither on its way
// nor known not to exist.
func (t *workflowLookup) due(id string, now time.Time) bool {
	if t.missing[id] || t.pending[id] {
		return false
	}
	w, ok := t.found[id]
	return !ok || (w.workflow.Status == "Running" && now.Sub(w.fetched) > treeParentStale)
}

// accept records what a lookup found and reports whether it found anything.
// IDs it did not find are not asked for again; after an error they may be.
func (t *workflowLookup) accept(gen uint64, ids []string, found map[string]temporal.Workflow, err error) bool {
	if gen != t.gen {
		return false
	}
	if t.found == nil {
		t.found = make(map[string]lookedUpWorkflow)
	}
	if t.missing == nil {
		t.missing = make(map[string]bool)
	}
	now := time.Now()
	for _, id := range ids {
		delete(t.pending, id)
		if w, ok := found[id]; ok {
			t.found[id] = lookedUpWorkflow{workflow: w, fetched: now}
		} else if err == nil {
			t.missing[id] = true
		}
	}
	return len(found) > 0
}

// withTreeParents puts each workflow's unloaded parent, when it has been
// fetched, just ahead of its first child, and asks for the ones it does not
// have yet. The list is ordered by close time, so a parent that started a
// child and finished at once sits far below the child, often on a page that
// is not loaded, and the child would otherwise show with no parent. A text
// search hoists its matches to the top on purpose, so it is left alone, and
// filter_show_parents turns it off for a filter, whose matches then show
// without the ancestors that do not match.
func (wl *WorkflowList) withTreeParents(workflows []temporal.Workflow) []temporal.Workflow {
	if wl.filterText != "" || (wl.treeFilterActive() && !wl.config().ShouldFilterShowParents()) {
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
		if wl.treeParents.due(id, now) && !queued[id] {
			queued[id] = true
			fetch = append(fetch, id)
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
	return wl.lookupWorkflows(&wl.treeParents, ids, func() {
		if wl.workflowTreeMode {
			wl.applyFilter()
		}
	})
}

// acceptTreeParents records what a tree parent lookup found.
func (wl *WorkflowList) acceptTreeParents(gen uint64, ids []string, found map[string]temporal.Workflow, err error) bool {
	return wl.treeParents.accept(gen, ids, found, err)
}

// lookupWorkflows fetches workflows by ID into cache and calls redraw on the
// UI thread when it found some. Without a running UI there is no thread to
// hand the result back to, so it looks them up in place and reports whether
// it found any instead.
func (wl *WorkflowList) lookupWorkflows(cache *workflowLookup, ids []string, redraw func()) bool {
	if wl.app == nil || wl.app.Provider() == nil {
		return false
	}
	provider := wl.app.Provider()
	namespace := wl.namespace
	gen := cache.gen
	if cache.pending == nil {
		cache.pending = make(map[string]bool)
	}
	for _, id := range ids {
		cache.pending[id] = true
	}
	lookup := func() (map[string]temporal.Workflow, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return fetchWorkflowsByID(ctx, provider, namespace, ids)
	}
	if wl.app.JigApp() == nil {
		found, err := lookup()
		return cache.accept(gen, ids, found, err)
	}
	go func() {
		found, err := lookup()
		wl.app.JigApp().QueueUpdateDraw(func() {
			if cache.accept(gen, ids, found, err) {
				redraw()
			}
		})
	}()
	return false
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
