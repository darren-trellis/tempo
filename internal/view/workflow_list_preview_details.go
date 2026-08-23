package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

func workflowInfoRowIndex(rows []workflowInfoRow, key string) int {
	if key == "" {
		return -1
	}
	for i, row := range rows {
		if row.Key == key {
			return i
		}
	}
	return -1
}

func (wl *WorkflowList) selectedPreviewDetailRow() (workflowInfoRow, bool) {
	if wl == nil || wl.workflowDetail == nil {
		return workflowInfoRow{}, false
	}
	row := wl.workflowDetail.SelectedRow()
	if row < 0 || row >= len(wl.previewDetailRows) {
		return workflowInfoRow{}, false
	}
	return wl.previewDetailRows[row], true
}

func (wl *WorkflowList) selectedPreviewDetailRowIs(key string) bool {
	row, ok := wl.selectedPreviewDetailRow()
	return ok && row.Key == key
}

func (wl *WorkflowList) setPreviewDetailStatus(message string) {
	wl.previewDetailRows = nil
	if wl.workflowDetail == nil {
		return
	}
	wl.workflowDetail.ClearRows()
	if message == "" {
		return
	}
	wl.workflowDetail.AddStyledRow([]components.TableCell{
		{Text: message, Color: theme.FgDim(), Selectable: true},
	})
	wl.workflowDetail.SelectRow(0)
}

func (wl *WorkflowList) handlePreviewDetailKeys(event *tcell.EventKey) *tcell.EventKey {
	if wl.handlePreviewDetailScroll(event) {
		return nil
	}
	switch event.Key() {
	case tcell.KeyEnter:
		wl.activatePreviewDetailRow()
		return nil
	}
	if event.Rune() == 'y' {
		wl.yankPreviewDetailRow()
		return nil
	}
	return wl.handlePreviewKeys(event)
}

func (wl *WorkflowList) handlePreviewDetailScroll(event *tcell.EventKey) bool {
	if wl.workflowDetailScroll == nil || event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyLeft:
		wl.workflowDetailScroll.scrollChars(-1)
		return true
	case tcell.KeyRight:
		wl.workflowDetailScroll.scrollChars(1)
		return true
	}
	switch event.Rune() {
	case 'h':
		wl.workflowDetailScroll.scrollChars(-1)
		return true
	case 'l':
		wl.workflowDetailScroll.scrollChars(1)
		return true
	}
	return false
}

func (wl *WorkflowList) yankPreviewDetailRow() {
	row, ok := wl.selectedPreviewDetailRow()
	if !ok || row.Value == "" {
		return
	}
	if wl.app == nil {
		return
	}
	if err := copyToClipboard(row.Value); err != nil {
		wl.app.ToastError("Failed to copy: " + err.Error())
		return
	}
	wl.app.ToastSuccess("Copied " + row.Label)
}

func (wl *WorkflowList) activatePreviewDetailRow() {
	row, ok := wl.selectedPreviewDetailRow()
	if !ok || row.Key != workflowInfoParent || row.Value == "" {
		return
	}
	if wl.selectWorkflowByID(row.Value) {
		wl.setFocusPane(focusWorkflows)
		return
	}
	if wl.app != nil {
		wl.app.ToastWarning("Parent workflow is not in the list")
	}
}

func (wl *WorkflowList) selectWorkflowByID(id string) bool {
	if id == "" || wl.table == nil {
		return false
	}
	for i, w := range wl.workflows {
		if w.ID == id {
			wl.table.SelectRow(i)
			return true
		}
	}
	return false
}
