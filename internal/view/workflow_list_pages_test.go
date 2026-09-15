package view

import (
	"context"
	"fmt"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"gopkg.in/yaml.v3"
)

func TestWorkflowPagerWindowsAndDeloads(t *testing.T) {
	var p workflowPager
	p.reset("")
	for i := 0; i < 6; i++ {
		token := ""
		if i > 0 {
			token = fmt.Sprintf("p%d", i)
		}
		next := fmt.Sprintf("p%d", i+1)
		p.accept(i, token, next, []temporal.Workflow{{ID: fmt.Sprintf("wf-%d", i)}})
	}
	if p.firstPage != 2 {
		t.Fatalf("firstPage=%d want 2 after deload", p.firstPage)
	}
	if len(p.pages) != workflowMaxPages {
		t.Fatalf("pages=%d want %d", len(p.pages), workflowMaxPages)
	}
	items := p.items()
	if items[0].ID != "wf-2" || items[len(items)-1].ID != "wf-5" {
		t.Fatalf("window=%v", ids(items))
	}
	if !p.hasPrev() {
		t.Fatal("should be able to reload earlier pages")
	}
	token, ok := p.prevToken()
	if !ok || token != "p1" {
		t.Fatalf("prev token=%q ok=%v", token, ok)
	}

	p.accept(1, "p1", "p2", []temporal.Workflow{{ID: "wf-1"}})
	if p.firstPage != 1 {
		t.Fatalf("firstPage=%d after prepend", p.firstPage)
	}
	if p.items()[0].ID != "wf-1" {
		t.Fatalf("prepended window=%v", ids(p.items()))
	}
	if len(p.pages) != workflowMaxPages {
		t.Fatalf("pages=%d after prepend deload", len(p.pages))
	}
}

func TestWorkflowPagerRefreshInPlace(t *testing.T) {
	var p workflowPager
	p.reset("")
	p.accept(0, "", "p1", []temporal.Workflow{{ID: "old"}})
	p.accept(0, "", "p1", []temporal.Workflow{{ID: "new"}})
	if got := p.items(); len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("refresh should replace the loaded page, got %v", ids(got))
	}
}

func TestWorkflowListPageSizeUsesConfig(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.pageSize() != config.DefaultWorkflowPageSize {
		t.Fatalf("default page size=%d", wl.pageSize())
	}
	size := 25
	wl.app.config = &config.Config{WorkflowPageSize: &size}
	if wl.pageSize() != 25 {
		t.Fatalf("configured page size=%d", wl.pageSize())
	}
}

func TestWorkflowListPreviewLoadDelayUsesConfig(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.previewLoadDelay() != config.DefaultPreviewLoadDelay {
		t.Fatalf("default preview load delay=%s", wl.previewLoadDelay())
	}
	var cfg config.Config
	if err := yaml.Unmarshal([]byte("preview_load_delay: 0\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	wl.app.config = &cfg
	if wl.previewLoadDelay() != 0 {
		t.Fatalf("configured preview load delay=%s", wl.previewLoadDelay())
	}
}

func TestAdjacentPageNeedSkipsExactEdges(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	items := make([]temporal.Workflow, 400)
	for i := range items {
		items[i] = temporal.Workflow{ID: fmt.Sprintf("wf-%d", i), RunID: "r"}
	}
	wl.workflows = items
	wl.allWorkflows = items
	wl.pager.reset("")
	for i := 0; i < workflowMaxPages; i++ {
		token := ""
		if i > 0 {
			token = fmt.Sprintf("p%d", i)
		}
		wl.pager.accept(i, token, fmt.Sprintf("p%d", i+1), items[i*100:(i+1)*100])
	}
	wl.populateTable()
	wl.table.SetRect(0, 0, 80, 12)

	wl.table.SelectRow(399)
	wl.rememberHighlightedWorkflow()
	if got := wl.adjacentPageNeed(); got != 0 {
		t.Fatalf("last row should not slide a full window, need=%d", got)
	}

	wl.table.SelectRow(0)
	wl.rememberHighlightedWorkflow()
	if got := wl.adjacentPageNeed(); got != 0 {
		t.Fatalf("first row should not slide a full window, need=%d", got)
	}

	wl.table.SelectRow(390)
	wl.rememberHighlightedWorkflow()
	if got := wl.adjacentPageNeed(); got != 1 {
		t.Fatalf("near the end should still prefetch, need=%d", got)
	}
}

func TestFetchAdjacentPageShowsStatusLoading(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	provider := &pageListProvider{
		list: func(context.Context, string, temporal.ListOptions) ([]temporal.Workflow, string, error) {
			close(started)
			<-release
			return []temporal.Workflow{{ID: "wf-next"}}, "", nil
		},
	}
	wl := NewWorkflowList(&App{provider: provider}, "default")
	wl.pager.reset("")
	wl.pager.accept(0, "", "next", []temporal.Workflow{{ID: "wf-0"}})

	wl.fetchAdjacentPage(false)
	<-started
	if wl.app.loadingText() == "" {
		t.Fatal("status bar should show loading while the next page is fetched")
	}
	if !wl.loading {
		t.Fatal("workflow list should be marked loading")
	}
	close(release)
}

type pageListProvider struct {
	temporal.Provider
	list func(context.Context, string, temporal.ListOptions) ([]temporal.Workflow, string, error)
}

func (p *pageListProvider) ListWorkflows(ctx context.Context, namespace string, opts temporal.ListOptions) ([]temporal.Workflow, string, error) {
	return p.list(ctx, namespace, opts)
}

func TestMaybeFetchPagesSkipsLocalFilter(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.filterText = "pay"
	wl.pager.reset("")
	wl.pager.accept(0, "", "next", []temporal.Workflow{{ID: "pay-1"}, {ID: "other"}})
	wl.allWorkflows = wl.pager.items()
	wl.workflows = []temporal.Workflow{{ID: "pay-1"}}
	wl.maybeFetchPages()
	if wl.pageBusy {
		t.Fatal("local / filter should not slide the loaded window")
	}
}

func TestDisplayedStatsUsesServerCounts(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflows = []temporal.Workflow{{Status: "Running"}, {Status: "Running"}, {Status: "TimedOut"}, {Status: "ContinuedAsNew"}}
	if got := wl.displayedStats(); got.Running != 2 || got.TimedOut != 1 || got.ContinuedAsNew != 1 {
		t.Fatalf("local stats=%+v", got)
	}
	wl.serverStats = WorkflowStats{Running: 40, Completed: 120, Failed: 7, Canceled: 3, Terminated: 2, TimedOut: 5, ContinuedAsNew: 9}
	wl.serverStatsOK = true
	if got := wl.displayedStats(); got != (WorkflowStats{Running: 40, Completed: 120, Failed: 7, Canceled: 3, Terminated: 2, TimedOut: 5, ContinuedAsNew: 9}) {
		t.Fatalf("server stats=%+v", got)
	}
}

func ids(workflows []temporal.Workflow) []string {
	out := make([]string, len(workflows))
	for i, w := range workflows {
		out[i] = w.ID
	}
	return out
}
