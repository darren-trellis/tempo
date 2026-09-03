package view

import (
	"fmt"
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
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
	wl.workflows = []temporal.Workflow{{Status: "Running"}, {Status: "Running"}}
	if got := wl.displayedStats(); got.Running != 2 {
		t.Fatalf("local stats=%+v", got)
	}
	wl.serverStats = WorkflowStats{Running: 40, Completed: 120, Failed: 7, Canceled: 3, Terminated: 2}
	wl.serverStatsOK = true
	if got := wl.displayedStats(); got != (WorkflowStats{Running: 40, Completed: 120, Failed: 7, Canceled: 3, Terminated: 2}) {
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
