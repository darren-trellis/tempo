package view

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

type blockingHistoryProvider struct {
	temporal.Provider
	started chan struct{}
	release chan struct{}
}

func (p *blockingHistoryProvider) GetEnhancedWorkflowHistory(context.Context, string, string, string) ([]temporal.EnhancedHistoryEvent, error) {
	p.started <- struct{}{}
	<-p.release
	return []temporal.EnhancedHistoryEvent{{Type: "WorkflowExecutionStarted"}}, nil
}

func (p *blockingHistoryProvider) GetWorkflow(context.Context, string, string, string) (*temporal.Workflow, error) {
	return nil, nil
}

// An auto-refresh used to show the list's quiet spinner, drop it, wait out the
// highlight delay and then show a labelled one for the preview. The preview
// on screen should take the quiet spinner over in the same update instead.
func TestAutoRefreshReloadsShownPreviewQuietly(t *testing.T) {
	provider := &blockingHistoryProvider{started: make(chan struct{}, 1), release: make(chan struct{})}
	a := &App{provider: provider}
	wl := NewWorkflowList(a, "default")
	wl.previewMode = true
	w := temporal.Workflow{ID: "wf-1", RunID: "run-1", Status: "Running"}
	wl.workflows = []temporal.Workflow{w}
	wl.previewWorkflowID = w.ID
	wl.previewRunID = w.RunID
	wl.previewEvents = []temporal.EnhancedHistoryEvent{{Type: "WorkflowExecutionStarted"}}

	// What the end of an auto-refresh does: release the list's spinner, then
	// refresh the preview, all in one UI update.
	wl.setRefreshing(true)
	wl.setLoading(false)
	wl.refreshLivePreview()
	if got := a.loadingText(); got == "" || strings.Contains(got, "Loading") {
		t.Fatalf("the spinner should stay on and quiet across the handover, got %q", got)
	}

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("a live refresh should not wait out the highlight delay")
	}
	if got := a.loadingText(); got == "" || strings.Contains(got, "Loading") {
		t.Fatalf("the preview reload should stay quiet, got %q", got)
	}

	close(provider.release)
	deadline := time.After(time.Second)
	for a.loadingText() != "" {
		select {
		case <-deadline:
			t.Fatalf("the spinner should stop once the preview is back, got %q", a.loadingText())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Moving the cursor still waits for the highlight delay and says Loading.
func TestCursorPreviewLoadStillSaysLoading(t *testing.T) {
	provider := &blockingHistoryProvider{started: make(chan struct{}, 1), release: make(chan struct{})}
	a := &App{provider: provider}
	wl := NewWorkflowList(a, "default")
	wl.previewMode = true

	wl.schedulePreview(temporal.Workflow{ID: "wf-2", RunID: "run-2"}, false)
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the preview should load")
	}
	if got := a.loadingText(); !strings.Contains(got, "Loading") {
		t.Fatalf("a preview for a newly highlighted workflow should say Loading, got %q", got)
	}
	close(provider.release)
}
