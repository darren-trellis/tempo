package view

import (
	"fmt"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

const timelinePanelHeight = 12

func (wl *WorkflowList) setupTimeline() {
	wl.timelineView = NewTimelineView()
	wl.timelinePanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Timeline", theme.IconEvent))
	wl.timelinePanel.SetContent(wl.timelineView)

	wl.timelineView.SetOnSelectionChange(func(lane *TimelineLane) {
		wl.onTimelineLaneChange(lane)
	})
	wl.timelineView.SetOnSelect(func(lane *TimelineLane) {
		wl.onTimelineLaneChange(lane)
	})
	wl.timelineView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			wl.cycleFocus(1)
			return nil
		case tcell.KeyBacktab:
			wl.cycleFocus(-1)
			return nil
		case tcell.KeyEscape:
			wl.setFocusPane(focusWorkflows)
			return nil
		}
		switch event.Rune() {
		case 'z':
			wl.toggleTimeline()
			return nil
		case 'b':
			wl.toggleWorkflowTree()
			return nil
		case 'p':
			wl.togglePreviewMode()
			return nil
		case 'i':
			if wl.showPreviewIO() {
				return nil
			}
		}
		return event
	})
}

func (wl *WorkflowList) historyNeeded() bool {
	return wl.previewModeEnabled() || wl.timelineVisible
}

func (wl *WorkflowList) applyMainLayout() {
	wl.Clear()
	if wl.mainFlex != nil {
		wl.AddItem(wl.mainFlex, 0, 1, true)
	}
	if wl.timelineVisible && wl.timelinePanel != nil {
		wl.AddItem(wl.timelinePanel, timelinePanelHeight, 0, false)
	}
}

func (wl *WorkflowList) toggleTimeline() {
	wl.timelineVisible = !wl.timelineVisible
	if !wl.timelineVisible && wl.focusPane == focusTimeline {
		wl.setFocusPane(focusWorkflows)
	}
	wl.applyMainLayout()
	if wl.timelineVisible {
		if w, ok := wl.selectedWorkflow(); ok {
			wl.schedulePreview(w, false)
		}
		wl.refreshTimeline()
		wl.syncTimelineFromActivity()
	} else if !wl.previewModeEnabled() {
		wl.clearPreview()
	}
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
	} else {
		wl.applyFocusStyles()
	}
}

func (wl *WorkflowList) selectedWorkflow() (temporal.Workflow, bool) {
	if w, ok := wl.currentPreviewWorkflow(); ok {
		return w, true
	}
	if wl.table == nil {
		return temporal.Workflow{}, false
	}
	row := wl.table.SelectedRow()
	if row >= 0 && row < len(wl.workflows) {
		return wl.workflows[row], true
	}
	if len(wl.workflows) > 0 {
		return wl.workflows[0], true
	}
	return temporal.Workflow{}, false
}

func (wl *WorkflowList) refreshTimeline() {
	if wl.timelineView == nil {
		return
	}
	wl.timelineView.SetNodes(temporal.BuildEventTree(wl.previewEvents))
	wl.syncTimelineFromActivity()
}

func (wl *WorkflowList) syncTimelineFromActivity() {
	if wl.timelineView == nil || wl.highlightedActivityID == 0 {
		return
	}
	wl.timelineSyncing = true
	wl.timelineView.SelectByScheduledID(wl.highlightedActivityID)
	wl.timelineSyncing = false
}

func (wl *WorkflowList) onTimelineLaneChange(lane *TimelineLane) {
	if lane == nil || wl.timelineSyncing {
		return
	}
	id := timelineLaneScheduledID(*lane)
	if id == 0 {
		return
	}
	wl.highlightedActivityID = id
	for i, a := range wl.previewActivities {
		if a.ScheduledID != id {
			continue
		}
		if wl.previewModeEnabled() && wl.previewKind != previewActivities {
			wl.setPreviewKind(previewActivities)
		}
		if wl.eventTable != nil && wl.eventTable.SelectedRow() != i {
			wl.timelineSyncing = true
			wl.eventTable.SelectRow(i)
			wl.timelineSyncing = false
		}
		if wl.eventDetail != nil {
			wl.eventDetail.SetText(formatSelectedActivityDetail(a))
			wl.eventDetail.ScrollToBeginning()
		}
		return
	}
}
