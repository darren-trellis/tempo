package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
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
	wl.focusPane = focusEventDetail
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
