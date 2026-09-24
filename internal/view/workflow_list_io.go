package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (wl *WorkflowList) previewIOPayload() (title, input, output string, ok bool) {
	if wl.previewModeEnabled() && wl.previewKind == previewActivities {
		a, found := wl.selectedPreviewActivity()
		if !found {
			return "", "", "", false
		}
		out := a.Result
		if out == "" {
			out = a.Failure
		}
		name := a.Type
		if name == "" {
			name = "Activity"
		}
		return name, a.Input, out, true
	}
	w, found := wl.selectedWorkflow()
	if !found {
		return "", "", "", false
	}
	events, found := wl.workflowIOEvents(w)
	if !found {
		return "", "", "", false
	}
	input, output = workflowIOFromEvents(events)
	return w.Type, input, output, true
}

// workflowIOEvents returns history we already hold for a workflow, but only if it
// can actually answer the question. A snapshot taken while the workflow was
// running has no terminal event, so reading output off it would report none --
// which is what happened whenever the preview was closed and so never refreshed
// the cache. In that case say no and let the caller fetch.
func (wl *WorkflowList) workflowIOEvents(w temporal.Workflow) ([]temporal.EnhancedHistoryEvent, bool) {
	usable := func(events []temporal.EnhancedHistoryEvent) bool {
		if len(events) == 0 {
			return false
		}
		return workflowRunning(w) || workflowHistoryComplete(events)
	}
	if w.ID != "" && wl.previewWorkflowID == w.ID && wl.previewRunID == w.RunID && usable(wl.previewEvents) {
		return wl.previewEvents, true
	}
	if events, ok := wl.previewCache.get(w.ID, w.RunID); ok && usable(events) {
		return events, true
	}
	return nil, false
}

func (wl *WorkflowList) previewIOViewFocused() bool {
	if wl == nil || wl.focusPane != focusEventDetail {
		return false
	}
	if wl.previewKind == previewDetails {
		return true
	}
	return wl.previewKind == previewActivities && (wl.activityDetailKind == activityDetailInput || wl.activityDetailKind == activityDetailOutput)
}

func (wl *WorkflowList) previewIOEditorPayload() (label, content string, ok bool) {
	if !wl.previewIOViewFocused() {
		return "", "", false
	}
	if wl.previewKind == previewActivities {
		a, found := wl.selectedPreviewActivity()
		if !found {
			if visible := wl.visiblePreviewActivities(); len(visible) > 0 {
				a = visible[0]
			} else {
				return "", "", false
			}
		}
		if wl.activityDetailKind == activityDetailOutput {
			out := a.Result
			if out == "" {
				out = a.Failure
			}
			return "output", out, true
		}
		return "input", a.Input, true
	}
	input, output := workflowIOFromEvents(wl.previewEvents)
	if wl.workflowIOKind == workflowIOOutput {
		return "output", output, true
	}
	return "input", input, true
}

func (wl *WorkflowList) openPreviewIOInEditor() bool {
	label, content, ok := wl.previewIOEditorPayload()
	if !ok {
		return false
	}
	openInEditor(wl.app, label, content)
	return true
}

func (wl *WorkflowList) yankPreviewIO(all bool) bool {
	label, content, ok := wl.previewIOEditorPayload()
	if !ok {
		return false
	}
	if !all && ioTreeEnabled(wl.app) {
		var tree *jsonTreeSelection
		if wl.previewKind == previewActivities {
			tree = wl.eventDetailTree
		} else {
			tree = wl.workflowIOTree
		}
		if value, selected := tree.value(); selected {
			content = value
			label = "row"
		}
	}
	if wl.app == nil {
		return true
	}
	if strings.TrimSpace(content) == "" {
		wl.app.ToastError("No " + label + " to copy")
		return true
	}
	if err := copyToClipboard(content); err != nil {
		wl.app.ToastError("Failed to copy: " + err.Error())
		return true
	}
	wl.app.ToastSuccess("Copied " + label)
	return true
}

func (wl *WorkflowList) showPreviewIO() bool {
	if wl.previewModeEnabled() && wl.previewKind == previewActivities {
		if len(wl.previewActivities) == 0 {
			if wl.app != nil {
				if wl.previewWorkflowID == "" {
					wl.app.ToastError("Events still loading")
				} else {
					wl.app.ToastError("No activities")
				}
			}
			return true
		}
		title, input, output, ok := wl.previewIOPayload()
		if !ok {
			if wl.app != nil {
				wl.app.ToastError("Nothing to show")
			}
			return true
		}
		restore := wl.focusPane
		if restore == focusWorkflows {
			restore = focusEvents
		}
		wl.openWorkflowIO(title, input, output, restore)
		return true
	}
	w, ok := wl.selectedWorkflow()
	if !ok {
		return false
	}
	if events, found := wl.workflowIOEvents(w); found {
		input, output := workflowIOFromEvents(events)
		wl.openWorkflowIO(w.Type, input, output, wl.focusPane)
		return true
	}
	wl.loadWorkflowIO(w)
	return true
}

func (wl *WorkflowList) openWorkflowIO(title, input, output string, restore workflowFocusPane) {
	wl.keepDataOnStart = true
	showWorkflowIO(wl.app, wl, title, input, output, func() {
		wl.setFocusPane(restore)
	})
}

func (wl *WorkflowList) loadWorkflowIO(w temporal.Workflow) {
	if wl.app == nil {
		return
	}
	wl.app.ToastWarning("Loading input/output...")
	go func() {
		events, err := wl.fetchWorkflowEvents(w)
		if wl.app.JigApp() == nil {
			return
		}
		wl.app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				wl.app.ToastError("Failed to load input/output: " + err.Error())
				return
			}
			wl.previewCache.put(w.ID, w.RunID, events)
			if selected, ok := wl.selectedWorkflow(); ok && selected.ID == w.ID && selected.RunID == w.RunID {
				if wl.previewWorkflowID == "" {
					wl.previewWorkflowID = w.ID
					wl.previewRunID = w.RunID
					wl.previewEvents = events
					wl.previewActivities = previewActivitiesFromEvents(events)
				}
			}
			input, output := workflowIOFromEvents(events)
			wl.openWorkflowIO(w.Type, input, output, wl.focusPane)
		})
	}()
}

func (wl *WorkflowList) fetchWorkflowEvents(w temporal.Workflow) ([]temporal.EnhancedHistoryEvent, error) {
	if wl.app == nil {
		return mockPreviewEvents(w), nil
	}
	provider := wl.app.Provider()
	if provider == nil {
		return mockPreviewEvents(w), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return provider.GetEnhancedWorkflowHistory(ctx, wl.namespace, w.ID, w.RunID)
}

func (wl *WorkflowList) handleWorkflowIOTabKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	switch event.Rune() {
	case '[':
		wl.cycleWorkflowIOKind(-1)
		return true
	case ']':
		wl.cycleWorkflowIOKind(1)
		return true
	case '1':
		wl.setWorkflowIOKind(workflowIOInput)
		return true
	case '2':
		wl.setWorkflowIOKind(workflowIOOutput)
		return true
	}
	return false
}

func (wl *WorkflowList) cycleWorkflowIOKind(delta int) {
	n := len(workflowIOTabOrder)
	next := (int(wl.workflowIOKind) + delta) % n
	if next < 0 {
		next += n
	}
	wl.setWorkflowIOKind(workflowIOKind(next))
}

func (wl *WorkflowList) setWorkflowIOKind(kind workflowIOKind) {
	wl.workflowIOKind = kind
	if wl.workflowIOTabs != nil && wl.workflowIOTabs.GetActive() != int(kind) {
		wl.workflowIOTabs.SetActive(int(kind))
		return
	}
	wl.renderWorkflowIO()
	if wl.focusPane == focusEventDetail {
		wl.setFocusPane(focusEventDetail)
	}
}

func (wl *WorkflowList) renderWorkflowIO() {
	if wl.workflowIOView == nil {
		return
	}
	defer wl.revealIOSearch()
	if len(wl.previewEvents) == 0 {
		if wl.workflowIOTree != nil {
			wl.workflowIOTree.setContent("", false)
		}
		message := "Select a workflow to load preview"
		if wl.previewWorkflowID != "" {
			message = "Loading..."
		}
		wl.workflowIOView.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), message))
		wl.workflowIOView.ScrollToBeginning()
		return
	}
	input, output := workflowIOFromEvents(wl.previewEvents)
	content := input
	label := "Input"
	if wl.workflowIOKind == workflowIOOutput {
		content = output
		label = "Output"
	}
	tree := ioTreeEnabled(wl.app)
	if wl.workflowIOTree != nil && wl.workflowIOTree.setContent(content, tree) {
		return
	}
	wl.workflowIOView.SetText(formatIOContent(label, content, tree))
	wl.workflowIOView.ScrollToBeginning()
}

func (wl *WorkflowList) reportWrap(wrap bool) {
	if wl != nil && wl.app != nil {
		wl.app.ToastInfo(wrapToggleMessage(wrap))
	}
}

func (wl *WorkflowList) togglePreviewIOWrap(view *tview.TextView) bool {
	if wl == nil || (view != wl.workflowIOView && view != wl.eventDetail) || !wl.previewIOViewFocused() {
		return false
	}
	on := !ioWrapOn(wl.app)
	if wl.app != nil {
		wl.app.setIOWrap(on)
	}
	wl.applyIOWrap(on)
	wl.reportWrap(on)
	return true
}

func ioWrapOn(app *App) bool {
	if app == nil || app.config == nil {
		return false
	}
	return app.config.ShouldWrapIO()
}

func (wl *WorkflowList) applyIOWrap(on bool) {
	if wl == nil {
		return
	}
	setTextViewWrap(wl.workflowIOView, on)
	setTextViewWrap(wl.eventDetail, on)
	if wl.workflowIOTree != nil {
		wl.workflowIOTree.relayout()
	}
	if wl.eventDetailTree != nil {
		wl.eventDetailTree.relayout()
	}
}

func searchLabel(query string, count int) string {
	if query == "" {
		return ""
	}
	return fmt.Sprintf("/%s (%d)", query, count)
}

func (wl *WorkflowList) showFocusedIOSearch() bool {
	if wl == nil || wl.app == nil || !wl.previewIOViewFocused() {
		return false
	}
	wl.app.ShowSearchPrompt(wl.focusedIOQuery(), wl.applyFocusedIOSearch, nil)
	return true
}

func (wl *WorkflowList) focusedIOQuery() string {
	if wl == nil {
		return ""
	}
	switch {
	case wl.previewKind == previewActivities && wl.activityDetailKind == activityDetailOutput:
		return wl.activityOutputSearch
	case wl.previewKind == previewActivities && wl.activityDetailKind == activityDetailInput:
		return wl.activityInputSearch
	case wl.previewKind == previewDetails && wl.workflowIOKind == workflowIOOutput:
		return wl.workflowOutputSearch
	case wl.previewKind == previewDetails:
		return wl.workflowInputSearch
	default:
		return ""
	}
}

func (wl *WorkflowList) applyFocusedIOSearch(query string) {
	if wl == nil {
		return
	}
	switch {
	case wl.previewKind == previewActivities && wl.activityDetailKind == activityDetailOutput:
		wl.activityOutputSearch = query
	case wl.previewKind == previewActivities && wl.activityDetailKind == activityDetailInput:
		wl.activityInputSearch = query
	case wl.previewKind == previewDetails && wl.workflowIOKind == workflowIOOutput:
		wl.workflowOutputSearch = query
	case wl.previewKind == previewDetails:
		wl.workflowInputSearch = query
	}
	wl.revealIOSearch()
}

func (wl *WorkflowList) revealIOSearch() {
	if wl == nil {
		return
	}
	view, query := wl.focusedIOView()
	if query != "" && view != nil {
		scrollTextViewToMatch(view, query)
	}
	wl.syncSearchTitles()
}

func (wl *WorkflowList) focusedIOView() (*tview.TextView, string) {
	if wl == nil {
		return nil, ""
	}
	switch {
	case wl.previewKind == previewActivities && wl.activityDetailKind == activityDetailInput:
		return wl.eventDetail, wl.activityInputSearch
	case wl.previewKind == previewActivities && wl.activityDetailKind == activityDetailOutput:
		return wl.eventDetail, wl.activityOutputSearch
	case wl.previewKind == previewDetails && wl.workflowIOKind == workflowIOOutput:
		return wl.workflowIOView, wl.workflowOutputSearch
	case wl.previewKind == previewDetails:
		return wl.workflowIOView, wl.workflowInputSearch
	default:
		return nil, ""
	}
}

func (wl *WorkflowList) syncSearchTitles() {
	if wl == nil {
		return
	}
	if wl.previewPanel != nil {
		title := ""
		switch wl.previewKind {
		case previewActivities:
			title = searchLabel(wl.previewActivitySearch, len(wl.visiblePreviewActivities()))
		case previewEvents:
			title = searchLabel(wl.previewEventSearch, len(wl.visiblePreviewEvents()))
		}
		wl.previewPanel.SetTitle(title)
	}
	if wl.eventDetailPanel != nil {
		view, query := wl.focusedIOView()
		count := 0
		if view != nil && query != "" {
			count = countSearchMatches(view.GetText(true), query)
		}
		wl.eventDetailPanel.SetTitle(searchLabel(query, count))
	}
}
