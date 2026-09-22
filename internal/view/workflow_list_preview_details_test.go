package view

import (
	"strings"
	"testing"

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
	scrollTextViewHoriz(wl.workflowIOView, 10)
	if _, col := wl.workflowIOView.GetScrollOffset(); col == 0 {
		t.Fatal("test setup should horizontally scroll the workflow input")
	}
	if ev := wl.workflowIOView.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'w', 0)); ev != nil {
		t.Fatal("w on workflow input/output should toggle wrapping")
	}
	if wl.workflowIOWrap {
		t.Fatal("workflow input/output should start wrapped and toggle off")
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
	if wl.eventDetailWrap {
		t.Fatal("activity input/output should start wrapped and toggle off")
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
