package view

import (
	"fmt"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	timelinePanelHeight = 12
	timelineSizeButton  = 3
)

type timelineFrame struct {
	*components.Panel
	list *WorkflowList
}

func (f *timelineFrame) Draw(screen tcell.Screen) {
	f.Panel.Draw(screen)
	drawTimelineSizeButton(screen, f, f.list != nil && !f.list.timelineNarrow)
}

func (f *timelineFrame) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return f.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event != nil && f.list != nil {
			x, y := event.Position()
			if timelineSizeButtonHit(f, x, y) && (action == tview.MouseLeftDown || action == tview.MouseLeftClick) {
				f.list.toggleTimelineSize()
				return true, nil
			}
		}
		if handler := f.Panel.MouseHandler(); handler != nil {
			return handler(action, event, setFocus)
		}
		return false, nil
	})
}

func timelineSizeButtonLabel(maximized bool) string {
	if maximized {
		return "[-]"
	}
	return "[+]"
}

func timelineSizeHint(narrow bool) string {
	if narrow {
		return "Maximize"
	}
	return "Minimize"
}

func timelineSizeButtonOrigin(p tview.Primitive) (x, y int) {
	px, py, pw, _ := p.GetRect()
	return px + pw - 2 - timelineSizeButton, py
}

func timelineSizeButtonHit(p tview.Primitive, x, y int) bool {
	bx, by := timelineSizeButtonOrigin(p)
	return y == by && x >= bx && x < bx+timelineSizeButton
}

func drawTimelineSizeButton(screen tcell.Screen, p tview.Primitive, maximized bool) {
	if screen == nil || p == nil {
		return
	}
	x, y := timelineSizeButtonOrigin(p)
	_, _, w, h := p.GetRect()
	if w < timelineSizeButton+4 || h < 1 {
		return
	}
	style := tcell.StyleDefault.Foreground(theme.PanelTitle()).Background(theme.Bg())
	for i, r := range timelineSizeButtonLabel(maximized) {
		screen.SetContent(x+i, y, r, nil, style)
	}
}

func (wl *WorkflowList) setupTimeline() {
	wl.timelineView = NewTimelineView()
	wl.timelineView.SetMouseScrollStep(func() int {
		return mouseScrollStepFromApp(wl.app)
	})
	wl.timelineView.SetShowScrollbars(func() bool {
		return appShowsScrollbars(wl.app)
	})
	wl.timelinePanel = &timelineFrame{
		Panel: components.NewPanel().SetTitle(fmt.Sprintf("%s Timeline", theme.IconEvent)),
		list:  wl,
	}
	wl.timelinePanel.SetContent(wl.timelineView)

	wl.timelineView.SetOnSelectionChange(func(lane *TimelineLane) {
		wl.onTimelineLaneChange(lane)
	})
	wl.timelineView.SetOnSelect(func(lane *TimelineLane) {
		wl.onTimelineLaneChange(lane)
	})
	wl.timelineView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if wl.handlePaneResizeKey(event) {
			return nil
		}
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
		case '?':
			if wl.app != nil {
				wl.app.showTimelineLegend()
			}
			return nil
		case 'm':
			wl.toggleTimelineSize()
			return nil
		case 'p':
			wl.togglePreviewMode()
			return nil
		case 'i':
			if wl.previewShowsIO() && wl.showPreviewIO() {
				return nil
			}
		case 'u':
			if wl.openSelectedWorkflowUI() {
				return nil
			}
		}
		return event
	})
}

func (wl *WorkflowList) historyNeeded() bool {
	return wl.previewModeEnabled() || wl.timelineVisible
}

func (wl *WorkflowList) timelineDocked() bool {
	return wl.timelineVisible && wl.timelinePanel != nil && wl.timelineNarrow && wl.previewModeEnabled()
}

func (wl *WorkflowList) applyMainLayout() {
	if !wl.workflowsActive() {
		if wl.mainFlex != nil {
			wl.mainFlex.Clear()
			var secondary tview.Primitive
			if wl.taskQueuesActive() && wl.pollersVisible && wl.taskQueues != nil {
				secondary = wl.taskQueues.pollerPanel
			}
			if wl.schedulesActive() && wl.scheduleDetailVisible && wl.schedules != nil {
				secondary = wl.schedules.detailFlex
			}
			if wl.workersActive() && wl.workerDetailVisible && wl.workers != nil {
				secondary = wl.workers.detailFlex
			}
			wl.addPrimarySecondary(wl.workflowsPanel, secondary, true)
		}
		wl.Clear()
		if wl.mainFlex != nil {
			wl.AddItem(wl.mainFlex, 0, 1, true)
		}
		wl.applyStoredPaneSizes()
		return
	}

	on := wl.previewModeEnabled()
	showTimeline := wl.timelineVisible && wl.timelinePanel != nil
	docked := showTimeline && wl.timelineDocked()

	if wl.mainFlex != nil {
		wl.mainFlex.Clear()
		if on {
			left := tview.Primitive(wl.workflowsPanel)
			if docked {
				stack := wl.ensurePrimaryStack()
				stack.Clear()
				stack.AddItem(wl.workflowsPanel, 0, 1, true)
				stack.AddItem(wl.timelinePanel, wl.timelineSize(), 0, false)
				left = stack
			}
			wl.addPrimarySecondary(left, wl.rightFlex, true)
		} else if wl.workflowsPanel != nil {
			wl.mainFlex.AddItem(wl.workflowsPanel, 0, 1, true)
		}
	}

	wl.Clear()
	if wl.mainFlex != nil {
		wl.AddItem(wl.mainFlex, 0, 1, true)
	}
	if showTimeline && !docked {
		wl.AddItem(wl.timelinePanel, wl.timelineSize(), 0, false)
	}
	wl.applyStoredPaneSizes()
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

func (wl *WorkflowList) toggleTimelineSize() {
	wl.timelineNarrow = !wl.timelineNarrow
	wl.applyMainLayout()
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
	} else {
		wl.applyFocusStyles()
	}
}

func (wl *WorkflowList) selectedWorkflow() (temporal.Workflow, bool) {
	if wl.table != nil {
		row := wl.table.SelectedRow()
		if row >= 0 && row < len(wl.workflows) {
			return wl.workflows[row], true
		}
	}
	if w, ok := wl.currentPreviewWorkflow(); ok {
		return w, true
	}
	if len(wl.workflows) > 0 {
		return wl.workflows[0], true
	}
	return temporal.Workflow{}, false
}

func (wl *WorkflowList) syncHistoryForSelectedRow() {
	wl.syncListEdgePin()
	wl.rememberHighlightedWorkflow()
	wl.maybeFetchPages()
	if !wl.historyNeeded() || wl.table == nil {
		return
	}
	row := wl.table.SelectedRow()
	if row < 0 || row >= len(wl.workflows) {
		return
	}
	wl.schedulePreview(wl.workflows[row], false)
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
	for i, a := range wl.visiblePreviewActivities() {
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
		wl.renderSelectedActivityDetail()
		return
	}
}
