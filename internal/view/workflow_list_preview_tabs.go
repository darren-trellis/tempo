package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (wl *WorkflowList) cyclePreviewKind(delta int) {
	if !wl.previewModeEnabled() {
		return
	}
	n := len(previewTabOrder)
	next := (int(wl.previewKind) + delta) % n
	if next < 0 {
		next += n
	}
	wl.setPreviewKind(previewKind(next))
}

func (wl *WorkflowList) handlePreviewTabKey(event *tcell.EventKey) bool {
	if event == nil || !wl.previewModeEnabled() || !wl.workflowsActive() {
		return false
	}
	if wl.focusPane == focusWorkflows || wl.focusPane == focusPollers {
		return false
	}
	if wl.focusPane == focusEventDetail && wl.previewKind == previewActivities {
		return wl.handleActivityDetailTabKey(event)
	}
	if wl.focusPane == focusEventDetail && wl.previewKind == previewDetails {
		return wl.handleWorkflowIOTabKey(event)
	}
	switch event.Rune() {
	case '[':
		wl.cyclePreviewKind(-1)
		return true
	case ']':
		wl.cyclePreviewKind(1)
		return true
	case '1':
		wl.setPreviewKind(previewDetails)
		return true
	case '2':
		wl.setPreviewKind(previewActivities)
		return true
	case '3':
		wl.setPreviewKind(previewEvents)
		return true
	case '4':
		wl.setPreviewKind(previewHierarchy)
		return true
	}
	return false
}

func (wl *WorkflowList) handleActivityDetailTabKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	switch event.Rune() {
	case '[':
		wl.cycleActivityDetailKind(-1)
		return true
	case ']':
		wl.cycleActivityDetailKind(1)
		return true
	case '1':
		wl.setActivityDetailKind(activityDetailDetails)
		return true
	case '2':
		wl.setActivityDetailKind(activityDetailInput)
		return true
	case '3':
		wl.setActivityDetailKind(activityDetailOutput)
		return true
	}
	return false
}

func (wl *WorkflowList) cycleActivityDetailKind(delta int) {
	n := len(activityDetailTabOrder)
	next := (int(wl.activityDetailKind) + delta) % n
	if next < 0 {
		next += n
	}
	wl.setActivityDetailKind(activityDetailKind(next))
}

func (wl *WorkflowList) setActivityDetailKind(kind activityDetailKind) {
	wl.activityDetailKind = kind
	if wl.activityDetailTabs != nil && wl.activityDetailTabs.GetActive() != int(kind) {
		wl.activityDetailTabs.SetActive(int(kind))
		return
	}
	wl.renderSelectedActivityDetail()
	if wl.focusPane == focusEventDetail {
		wl.setFocusPane(focusEventDetail)
	}
}

func (wl *WorkflowList) setPreviewKind(kind previewKind) {
	if !wl.previewModeEnabled() {
		return
	}
	changing := wl.previewKind != kind
	wl.previewKind = kind
	if wl.previewTabs != nil && wl.previewTabs.GetActive() != int(kind) {
		wl.previewTabs.SetActive(int(kind))
	}
	if !changing {
		return
	}
	wasPreview := wl.focusPane != focusWorkflows
	wl.applyPreviewPage()
	if wasPreview {
		wl.setFocusPane(focusEvents)
		return
	}
}

func (wl *WorkflowList) applyPreviewPage() {
	if wl.previewTabs != nil && wl.previewTabs.GetActive() != int(wl.previewKind) {
		wl.previewTabs.SetActive(int(wl.previewKind))
	}
	if wl.rightFlex != nil {
		wl.rightFlex.Clear()
		var tertiary tview.Primitive
		if wl.previewShowsSidePane() {
			tertiary = wl.eventDetailPanel
		} else if wl.previewKind == previewHierarchy {
			tertiary = wl.hierarchyGraphPanel
		}
		wl.addSecondaryTertiary(wl.rightFlex, wl.previewPanel, tertiary)
	}
	if wl.previewKind == previewHierarchy {
		if w, ok := wl.selectedWorkflow(); ok {
			wl.renderPreviewHierarchy(w)
		}
	}
	wl.syncPreviewChrome()
	wl.syncSearchTitles()
	if wl.previewWorkflowID != "" {
		if w, ok := wl.currentPreviewWorkflow(); ok {
			wl.renderPreview(w)
		}
	}
}

func previewTabWidth(kind previewKind) int {
	width := 2 + len(kind.title())
	if icon := kind.icon(); icon != "" {
		width += len(icon) + 1
	}
	return width
}

func previewTabAtX(startX, x int) (previewKind, bool) {
	col := startX
	for _, kind := range previewTabOrder {
		width := previewTabWidth(kind)
		if x >= col && x < col+width {
			return kind, true
		}
		col += width + 1
	}
	return 0, false
}

func (wl *WorkflowList) previewTabAt(x, y int) (previewKind, bool) {
	if wl == nil || wl.previewTabs == nil || !wl.previewModeEnabled() {
		return 0, false
	}
	tx, ty, tw, _ := wl.previewTabs.GetInnerRect()
	if tw <= 0 || y != ty || x < tx || x >= tx+tw {
		return 0, false
	}
	return previewTabAtX(tx, x)
}

// syncPreviewChrome swaps in the tertiary pane's content. The pane carries no
// title: its tabs already name what it holds.
func (wl *WorkflowList) syncPreviewChrome() {
	if wl.previewKind == previewHierarchy {
		return
	}
	if wl.eventDetailPanel == nil {
		return
	}
	switch wl.previewKind {
	case previewActivities:
		if wl.activityDetailTabs != nil {
			wl.eventDetailPanel.SetContent(wl.activityDetailTabs)
		}
	case previewEvents:
		if wl.activityDetailScroll != nil {
			wl.eventDetailPanel.SetContent(wl.activityDetailScroll)
		}
	case previewDetails:
		if wl.workflowIOTabs != nil {
			wl.eventDetailPanel.SetContent(wl.workflowIOTabs)
		}
	default:
		if wl.eventDetail != nil {
			wl.eventDetailPanel.SetContent(wl.eventDetail)
		}
	}
}
