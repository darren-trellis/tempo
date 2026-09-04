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

func (wl *WorkflowList) handleActivityDetailKeys(event *tcell.EventKey) *tcell.EventKey {
	if handleTableCharScroll(wl.activityDetailScroll, wl.activityDetail, event) {
		return nil
	}
	if event.Rune() == 'y' {
		wl.yankActivityDetailRow()
		return nil
	}
	return wl.handlePreviewKeys(event)
}

func (wl *WorkflowList) selectedActivityDetailRow() (workflowInfoRow, bool) {
	if wl == nil || wl.activityDetail == nil {
		return workflowInfoRow{}, false
	}
	row := wl.activityDetail.SelectedRow()
	if row < 0 || row >= len(wl.activityDetailRows) {
		return workflowInfoRow{}, false
	}
	return wl.activityDetailRows[row], true
}

func (wl *WorkflowList) yankActivityDetailRow() {
	row, ok := wl.selectedActivityDetailRow()
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

func (wl *WorkflowList) setActivityDetailStatus(message string) {
	wl.activityDetailRows = nil
	if wl.activityDetail == nil {
		return
	}
	wl.activityDetail.ClearRows()
	if message == "" {
		return
	}
	wl.activityDetail.AddStyledRow([]components.TableCell{
		{Text: message, Color: theme.FgDim(), Selectable: true},
	})
	wl.activityDetail.SelectRow(0)
}

func (wl *WorkflowList) setActivityDetailRows(rows []workflowInfoRow) {
	selectedKey := ""
	if row, ok := wl.selectedActivityDetailRow(); ok {
		selectedKey = row.Key
	}
	wl.activityDetailRows = rows
	if wl.activityDetail == nil {
		return
	}
	wl.activityDetail.ClearRows()
	for _, row := range rows {
		wl.activityDetail.AddStyledRow([]components.TableCell{
			{Text: row.Label, Color: theme.FgDim(), Selectable: true},
			{Text: row.displayText(), Color: row.Color, Selectable: true},
		})
	}
	if idx := workflowInfoRowIndex(wl.activityDetailRows, selectedKey); idx >= 0 {
		wl.activityDetail.SelectRow(idx)
	} else if len(wl.activityDetailRows) > 0 {
		wl.activityDetail.SelectRow(0)
	}
	if wl.activityDetailScroll != nil {
		wl.activityDetailScroll.clamp()
	}
}

func (wl *WorkflowList) renderActivityDetailRows(a previewActivity) {
	wl.setActivityDetailRows(activityInfoRows(a))
}

func (wl *WorkflowList) renderSelectedEventDetail() {
	if wl.eventTreeMode && wl.eventTreeView != nil {
		if node := wl.eventTreeView.SelectedNode(); node != nil {
			wl.setActivityDetailRows(eventTreeInfoRows(node))
			return
		}
	}
	if len(wl.previewEvents) == 0 {
		wl.setActivityDetailStatus("No events")
		return
	}
	idx := 0
	if wl.eventTable != nil {
		row := wl.eventTable.SelectedRow()
		if row >= 0 && row < len(wl.previewEvents) {
			idx = row
		}
	}
	wl.setActivityDetailRows(eventInfoRows(wl.previewEvents[idx]))
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
