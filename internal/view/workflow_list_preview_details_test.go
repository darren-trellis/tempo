package view

import (
	"strings"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestPreviewDetailsSelectsRows(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	var child temporal.Workflow
	for _, w := range wl.workflows {
		if w.ID == "payment-xyz789" {
			child = w
			break
		}
	}
	if child.ID == "" {
		t.Fatal("expected child workflow in mock data")
	}
	wl.renderPreviewDetails(child)
	if len(wl.previewDetailRows) == 0 {
		t.Fatal("details should have rows")
	}
	parentIdx := workflowInfoRowIndex(wl.previewDetailRows, workflowInfoParent)
	if parentIdx < 0 {
		t.Fatal("child should have a parent row")
	}
	wl.workflowDetail.SelectRow(parentIdx)
	row, ok := wl.selectedPreviewDetailRow()
	if !ok || row.Value != "order-processing-abc123" {
		t.Fatalf("selected parent: %+v ok=%v", row, ok)
	}
}

func TestPreviewDetailsEnterHighlightsParent(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	var child temporal.Workflow
	childIdx := -1
	parentIdx := -1
	for i, w := range wl.workflows {
		if w.ID == "payment-xyz789" {
			child = w
			childIdx = i
		}
		if w.ID == "order-processing-abc123" {
			parentIdx = i
		}
	}
	if childIdx < 0 || parentIdx < 0 {
		t.Fatal("expected parent and child in mock data")
	}
	wl.table.SelectRow(childIdx)
	wl.renderPreviewDetails(child)
	wl.workflowDetail.SelectRow(workflowInfoRowIndex(wl.previewDetailRows, workflowInfoParent))
	wl.focusPane = focusEvents
	wl.activatePreviewDetailRow()
	if wl.table.SelectedRow() != parentIdx {
		t.Fatalf("parent row=%d want %d", wl.table.SelectedRow(), parentIdx)
	}
	if wl.focusPane != focusWorkflows {
		t.Fatalf("focus=%d", wl.focusPane)
	}
}

func TestPreviewDetailsEnterIgnoresNonParent(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	wf := wl.workflows[0]
	wl.table.SelectRow(0)
	wl.renderPreviewDetails(wf)
	wl.workflowDetail.SelectRow(0)
	wl.activatePreviewDetailRow()
	if wl.table.SelectedRow() != 0 {
		t.Fatalf("id row should not change selection, row=%d", wl.table.SelectedRow())
	}
}

func TestPreviewDetailsShowsWorkflowIOPane(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.setPreviewKind(previewDetails)
	if wl.rightFlex.GetItemCount() != 2 {
		t.Fatalf("details should show a sibling io pane, got %d items", wl.rightFlex.GetItemCount())
	}
	if wl.workflowIOTabs == nil {
		t.Fatal("expected workflow io tabs")
	}
	if wl.workflowIOTabs.GetActive() != int(workflowIOInput) {
		t.Fatalf("default io tab: %d", wl.workflowIOTabs.GetActive())
	}
	if wl.workflowIOKind != workflowIOInput {
		t.Fatalf("default io kind: %d", wl.workflowIOKind)
	}

	wl.previewEvents = startedAndCompleted()
	wl.renderWorkflowIO()
	if text := wl.workflowIOView.GetText(true); !strings.Contains(text, `"order"`) {
		t.Fatalf("input tab should show workflow input, got %q", text)
	}

	wl.focusPane = focusEventDetail
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ev != nil {
		t.Fatal("] should switch workflow io tabs")
	}
	if wl.previewKind != previewDetails {
		t.Fatal("] from workflow io should not change the preview tab")
	}
	if wl.workflowIOKind != workflowIOOutput {
		t.Fatalf("] should go to output, got %d", wl.workflowIOKind)
	}
	if text := wl.workflowIOView.GetText(true); !strings.Contains(text, `"ok"`) {
		t.Fatalf("output tab should show workflow output, got %q", text)
	}

	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '1', 0)); ev != nil {
		t.Fatal("1 should select input")
	}
	if wl.workflowIOKind != workflowIOInput {
		t.Fatalf("1 should select input, got %d", wl.workflowIOKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '2', 0)); ev != nil {
		t.Fatal("2 should select output")
	}
	if wl.workflowIOKind != workflowIOOutput {
		t.Fatalf("2 should select output, got %d", wl.workflowIOKind)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '[', 0)); ev != nil {
		t.Fatal("[ should cycle workflow io tabs")
	}
	if wl.workflowIOKind != workflowIOInput {
		t.Fatalf("[ should wrap to input, got %d", wl.workflowIOKind)
	}

	wl.focusPane = focusEvents
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ev != nil {
		t.Fatal("] from the details table should switch preview tabs")
	}
	if wl.previewKind != previewActivities {
		t.Fatalf("] from details table should go to activities, got %d", wl.previewKind)
	}
}

func TestPreviewIOEditorPayload(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.setPreviewKind(previewDetails)
	wl.previewEvents = startedAndCompleted()
	wl.focusPane = focusEvents
	if _, _, ok := wl.previewIOEditorPayload(); ok {
		t.Fatal("details table should not open the editor")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'e', 0)); ev == nil {
		t.Fatal("e on the details table should not be consumed")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'i', 0)); ev == nil {
		t.Fatal("i on the details table should not open the io modal")
	}

	wl.focusPane = focusEventDetail
	label, content, ok := wl.previewIOEditorPayload()
	if !ok || label != "input" || !strings.Contains(content, `"order"`) {
		t.Fatalf("details input: label=%q content=%q ok=%v", label, content, ok)
	}
	wl.setWorkflowIOKind(workflowIOOutput)
	label, content, ok = wl.previewIOEditorPayload()
	if !ok || label != "output" || !strings.Contains(content, `"ok"`) {
		t.Fatalf("details output: label=%q content=%q ok=%v", label, content, ok)
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'e', 0)); ev != nil {
		t.Fatal("e on the workflow io pane should open the editor")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'i', 0)); ev == nil {
		t.Fatal("i on the workflow io pane should not open the io modal")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'y', 0)); ev != nil {
		t.Fatal("y on the workflow io pane should yank")
	}
	wl.workflowIOView.SetText(strings.Repeat("x", 200))
	wl.workflowIOView.SetRect(0, 0, 20, 4)
	if ev := wl.workflowIOView.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'w', 0)); ev != nil {
		t.Fatal("w on workflow input/output should toggle wrapping")
	}
	if !ioWrapOn(wl.app) || !textViewWraps(wl.workflowIOView) || !textViewWraps(wl.eventDetail) {
		t.Fatal("wrap should turn on for every input/output pane")
	}
	if got := wl.app.hintBarMessage(); got != "Wrap on" {
		t.Fatalf("toggling wrap should report it, got %q", got)
	}

	wl.setPreviewKind(previewActivities)
	wl.activityDetailKind = activityDetailInput
	wl.previewActivities = []previewActivity{{
		Input:   `{"id":1}`,
		Result:  `{"ok":true}`,
		Failure: "unused",
	}}
	wl.focusPane = focusEventDetail
	label, content, ok = wl.previewIOEditorPayload()
	if !ok || label != "input" || content != `{"id":1}` {
		t.Fatalf("activity input: label=%q content=%q ok=%v", label, content, ok)
	}
	if ev := wl.eventDetail.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'w', 0)); ev != nil {
		t.Fatal("w on activity input/output should toggle wrapping")
	}
	if ioWrapOn(wl.app) || textViewWraps(wl.workflowIOView) || textViewWraps(wl.eventDetail) {
		t.Fatal("wrap is one setting, so toggling it again should turn every pane off")
	}
	if got := wl.app.hintBarMessage(); got != "Wrap off" {
		t.Fatalf("toggling activity wrap should report it, got %q", got)
	}
	wl.activityDetailKind = activityDetailOutput
	label, content, ok = wl.previewIOEditorPayload()
	if !ok || label != "output" || content != `{"ok":true}` {
		t.Fatalf("activity output: label=%q content=%q ok=%v", label, content, ok)
	}
	wl.previewActivities[0].Result = ""
	label, content, ok = wl.previewIOEditorPayload()
	if !ok || label != "output" || content != "unused" {
		t.Fatalf("activity failure: label=%q content=%q ok=%v", label, content, ok)
	}
	wl.activityDetailKind = activityDetailDetails
	if _, _, ok := wl.previewIOEditorPayload(); ok {
		t.Fatal("activity details table should not open the editor")
	}
}

func childEvents() []temporal.EnhancedHistoryEvent {
	return []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted"},
		{ID: 5, Type: "StartChildWorkflowExecutionInitiated", ChildWorkflowID: "a"},
		{ID: 6, Type: "StartChildWorkflowExecutionInitiated", ChildWorkflowID: "b"},
		{ID: 7, Type: "StartChildWorkflowExecutionInitiated", ChildWorkflowID: "c"},
		{ID: 8, Type: "StartChildWorkflowExecutionInitiated", ChildWorkflowID: "d"},
		{ID: 9, Type: "ChildWorkflowExecutionStarted", InitiatedEventID: 5},
		{ID: 10, Type: "ChildWorkflowExecutionStarted", InitiatedEventID: 6},
		{ID: 11, Type: "ChildWorkflowExecutionCompleted", InitiatedEventID: 5},
		{ID: 12, Type: "StartChildWorkflowExecutionFailed", InitiatedEventID: 7},
	}
}

func TestWorkflowChildStatuses(t *testing.T) {
	got := strings.Join(workflowChildStatuses(childEvents()), ",")
	if got != "Completed,Running,Failed,Scheduled" {
		t.Fatalf("statuses=%s", got)
	}
	if n := len(workflowChildStatuses([]temporal.EnhancedHistoryEvent{{Type: "WorkflowExecutionStarted"}})); n != 0 {
		t.Fatalf("a workflow without children has no statuses, got %d", n)
	}
}

func TestPreviewDetailsShowsChildrenAfterParent(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	parent := "root-1"
	w := temporal.Workflow{ID: "mid-1", RunID: "r", ParentID: &parent, Status: "Running"}
	wl.previewEvents = childEvents()
	wl.renderPreviewDetails(w)

	idx := workflowInfoRowIndex(wl.previewDetailRows, workflowInfoChildren)
	if idx != workflowInfoRowIndex(wl.previewDetailRows, workflowInfoParent)+1 {
		t.Fatalf("children should follow the parent, rows=%+v", wl.previewDetailRows)
	}
	row := wl.previewDetailRows[idx]
	if row.Value != "4" || row.Display != "4 · 1 Completed, 1 Running, 1 Failed, 1 Scheduled" {
		t.Fatalf("children row=%+v", row)
	}
	if row.Color != temporal.StatusFailed.Color() {
		t.Fatal("a failed child should color the row as failed")
	}

	wl.previewEvents = []temporal.EnhancedHistoryEvent{{ID: 1, Type: "WorkflowExecutionStarted"}}
	wl.renderPreviewDetails(temporal.Workflow{ID: "leaf", RunID: "r"})
	if workflowInfoRowIndex(wl.previewDetailRows, workflowInfoChildren) >= 0 {
		t.Fatal("a workflow without children should have no children row")
	}
}

func TestPreviewDetailsEnterOnChildrenListsThem(t *testing.T) {
	_, wl := adHocWorkflowList(t, config.DefaultConfig())
	wl.previewWorkflowID = "it's-1"
	wl.previewRunID = "r"
	wl.previewEvents = childEvents()
	wl.renderPreviewDetails(temporal.Workflow{ID: "it's-1", RunID: "r"})
	wl.workflowDetail.SelectRow(workflowInfoRowIndex(wl.previewDetailRows, workflowInfoChildren))
	wl.focusPane = focusEvents

	wl.activatePreviewDetailRow()
	if !strings.Contains(wl.visibilityQuery, "ParentWorkflowId = 'it''s-1'") {
		t.Fatalf("Enter on children should list them, query=%q", wl.visibilityQuery)
	}
	if wl.focusPane != focusWorkflows {
		t.Fatalf("focus should move to the list, focus=%d", wl.focusPane)
	}
}
