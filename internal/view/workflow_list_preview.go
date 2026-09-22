package view

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func formatTreeNodeDetail(node *temporal.EventTreeNode) string {
	if node == nil {
		return fmt.Sprintf("[%s]No events[-]", theme.TagFgDim())
	}

	status := temporal.GetWorkflowStatus(node.Status)
	statusTag := status.ColorTag()
	icon := status.Icon()

	durationStr := "running..."
	if node.Duration > 0 {
		durationStr = temporal.FormatDuration(node.Duration)
	}

	var attemptsStr string
	if node.Attempts > 1 {
		attemptsStr = fmt.Sprintf("\n\n[%s::b]Attempts[-:-:-]\n[%s]%d[-]", theme.TagAccent(), theme.TagFg(), node.Attempts)
	}

	var dataStr string
	for _, ev := range node.Events {
		if ev.Result != "" {
			formatted := formatSidePanelDetails(ev.Result)
			dataStr += fmt.Sprintf("\n\n[%s::b]Result[-:-:-]\n%s", theme.TagAccent(), formatted)
		}
		if ev.Failure != "" {
			dataStr += formatFailureSidePanel(ev)
		}
	}

	var eventsStr string
	if len(node.Events) > 0 {
		eventsStr = fmt.Sprintf("\n\n[%s::b]Events[-:-:-]", theme.TagAccent())
		for _, ev := range node.Events {
			evIcon := eventIcon(ev.Type)
			eventsStr += fmt.Sprintf("\n[%s]%s %s[-] [%s](%d)[-]",
				eventColorTag(ev.Type), evIcon, ev.Type, theme.TagFgDim(), ev.ID)
		}
	}

	return fmt.Sprintf(`
[%s::b]Name[-:-:-]
[%s]%s[-]

[%s::b]Status[-:-:-]
[%s]%s %s[-]

[%s::b]Duration[-:-:-]
[%s]%s[-]

[%s::b]Start Time[-:-:-]
[%s]%s[-]%s%s%s`,
		theme.TagAccent(),
		theme.TagFg(), node.Name,
		theme.TagAccent(),
		statusTag, icon, node.Status,
		theme.TagAccent(),
		theme.TagFg(), durationStr,
		theme.TagAccent(),
		theme.TagFg(), node.StartTime.Format("2006-01-02 15:04:05.000"),
		attemptsStr,
		dataStr,
		eventsStr,
	)
}

func formatSelectedEventDetail(ev temporal.EnhancedHistoryEvent) string {
	icon := eventIcon(ev.Type)
	colorTag := eventColorTag(ev.Type)
	formattedDetails := formatEventDetails(ev.Details)

	var nameLine string
	name := getEventNameDetail(&ev)
	if name != "" {
		nameLine = fmt.Sprintf("\n[%s::b]Name[-:-:-]         [%s]%s[-]", theme.TagFgDim(), theme.TagFg(), name)
	}

	return fmt.Sprintf(`
[%s::b]Event ID[-:-:-]     [%s]%d[-]
[%s::b]Type[-:-:-]         [%s]%s %s[-]%s
[%s::b]Time[-:-:-]         [%s]%s[-]

%s%s`,
		theme.TagFgDim(), theme.TagFg(), ev.ID,
		theme.TagFgDim(), colorTag, icon, ev.Type, nameLine,
		theme.TagFgDim(), theme.TagFg(), ev.Time.Format("2006-01-02 15:04:05.000"),
		formattedDetails,
		formatFailureSidePanel(&ev),
	)
}

func (wl *WorkflowList) selectedPreviewActivity() (previewActivity, bool) {
	activities := wl.visiblePreviewActivities()
	if wl.eventTable == nil || len(activities) == 0 {
		return previewActivity{}, false
	}
	row := wl.eventTable.SelectedRow()
	if row < 0 && len(activities) > 0 {
		row = 0
	}
	if row < 0 || row >= len(activities) {
		return previewActivity{}, false
	}
	return activities[row], true
}

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

func (wl *WorkflowList) activateSelectedWorkflow() {
	if !wl.workflowsActive() {
		return
	}
	if wl.previewModeEnabled() {
		wl.setPreviewVisible(false)
		return
	}
	row := wl.table.SelectedRow()
	if row < 0 || row >= len(wl.workflows) {
		return
	}
	wl.setPreviewVisible(true)
	wl.schedulePreview(wl.workflows[row], false)
}

func (wl *WorkflowList) revealWorkflow(id, runID string) {
	if wl == nil || id == "" {
		return
	}
	wl.setListKind(listWorkflows)
	found := wl.selectWorkflowByID(id)
	wl.setPreviewVisible(true)
	if found {
		if w, ok := wl.selectedWorkflow(); ok {
			wl.schedulePreview(w, false)
		}
		return
	}
	wl.schedulePreview(temporal.Workflow{ID: id, RunID: runID}, true)
}

func (wl *WorkflowList) paneAt(x, y int) (workflowFocusPane, bool) {
	if wl.timelineVisible && wl.timelinePanel != nil && wl.timelinePanel.InRect(x, y) {
		return focusTimeline, true
	}
	if wl.filtersOnSide() && wl.workflowsActive() && wl.filterBar != nil && wl.filterBar.InRect(x, y) {
		return focusFilters, true
	}
	if !wl.workflowsActive() {
		if wl.taskQueuesActive() && wl.pollersVisible && wl.taskQueues != nil && wl.taskQueues.pollerPanel != nil && wl.taskQueues.pollerPanel.InRect(x, y) {
			return focusPollers, true
		}
		if wl.schedulesActive() && wl.scheduleDetailVisible && wl.schedules != nil {
			if wl.schedules.runsPanel != nil && wl.schedules.runsPanel.InRect(x, y) {
				return focusScheduleRuns, true
			}
			if wl.schedules.detailPanel != nil && wl.schedules.detailPanel.InRect(x, y) {
				return focusScheduleDetail, true
			}
		}
		if wl.workersActive() && wl.workerDetailVisible && wl.workers != nil && wl.workers.detailFlex != nil && wl.workers.detailFlex.InRect(x, y) {
			return focusWorkerDetail, true
		}
		if wl.workflowsPanel != nil && wl.workflowsPanel.InRect(x, y) {
			return focusWorkflows, true
		}
	}
	if wl.previewModeEnabled() {
		if _, ok := wl.previewTabAt(x, y); ok {
			return focusWorkflows, false
		}
		if wl.previewKind == previewDetails {
			if wl.eventDetailPanel != nil && wl.eventDetailPanel.InRect(x, y) {
				return focusEventDetail, true
			}
			if wl.workflowDetailScroll != nil && wl.workflowDetailScroll.InRect(x, y) {
				return focusEvents, true
			}
			if wl.workflowDetail != nil && wl.workflowDetail.InRect(x, y) {
				return focusEvents, true
			}
		} else if wl.previewKind == previewHierarchy {
			if wl.hierarchyGraphPanel != nil && wl.hierarchyGraphPanel.InRect(x, y) {
				return focusEventDetail, true
			}
			if wl.hierarchyView != nil && wl.hierarchyView.tree != nil && wl.hierarchyView.tree.InRect(x, y) {
				return focusEvents, true
			}
		} else {
			if wl.eventDetailPanel != nil && wl.eventDetailPanel.InRect(x, y) {
				return focusEventDetail, true
			}
			if wl.previewKind == previewEvents && wl.eventTreeMode && wl.eventTreeView != nil && wl.eventTreeView.InRect(x, y) {
				return focusEvents, true
			}
			if wl.eventTableScroll != nil && wl.eventTableScroll.InRect(x, y) {
				return focusEvents, true
			}
			if wl.eventTable != nil && wl.eventTable.InRect(x, y) {
				return focusEvents, true
			}
		}
	}
	if wl.workflowsPanel != nil && wl.workflowsPanel.InRect(x, y) {
		return focusWorkflows, true
	}
	return focusWorkflows, false
}

func (wl *WorkflowList) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return wl.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		if !wl.InRect(x, y) {
			return false, nil
		}

		if wl.timelineVisible && wl.timelinePanel != nil && timelineSizeButtonHit(wl.timelinePanel, x, y) {
			if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
				wl.toggleTimelineSize()
				return true, nil
			}
		}

		if kind, ok := wl.listTabAt(x, y); ok {
			if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
				wl.setListKind(kind)
				return true, nil
			}
		}

		if kind, ok := wl.previewTabAt(x, y); ok {
			if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
				wl.setPreviewKind(kind)
				return true, nil
			}
		}

		if pane, ok := wl.paneAt(x, y); ok {
			switch action {
			case tview.MouseLeftDown, tview.MouseLeftClick:
				if pane != wl.focusPane {
					wl.setFocusPane(pane)
				}
			}
		}

		consumed, capture := wl.Flex.MouseHandler()(action, event, setFocus)
		if action == tview.MouseLeftDoubleClick && consumed {
			if _, ok := wl.listTabAt(x, y); ok {
				return consumed, capture
			}
			if pane, ok := wl.paneAt(x, y); ok && pane == focusWorkflows {
				wl.activateSelectedWorkflow()
			}
		}
		return consumed, capture
	})
}

func (wl *WorkflowList) previewModeEnabled() bool {
	return wl != nil && wl.previewMode
}

func (wl *WorkflowList) applyPreviewLayout() {
	if !wl.workflowsActive() {
		wl.applyMainLayout()
		if wl.app != nil && wl.app.JigApp() != nil {
			wl.setFocusPane(focusWorkflows)
			return
		}
		wl.applyFocusStyles()
		return
	}

	on := wl.previewModeEnabled()
	if !on {
		if !wl.timelineVisible {
			if wl.previewTimer != nil {
				wl.previewTimer.Stop()
			}
			if wl.eventTable != nil {
				wl.clearPreview()
			}
		}
		wl.focusPane = focusWorkflows
	}

	wl.applyMainLayout()

	if on {
		wl.applyPreviewPage()
	}
	if wl.historyNeeded() {
		wl.syncHistoryForSelectedRow()
	}

	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
		return
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) togglePreviewMode() {
	wl.setPreviewVisible(!wl.previewMode)
}

func (wl *WorkflowList) setPreviewVisible(on bool) {
	if wl.previewMode == on {
		return
	}
	wl.previewMode = on
	wl.applyPreviewLayout()
}

// escapeFromPreview closes the preview when one of its panes is focused, the way
// the pollers, worker and schedule sidebars close on escape.
func (wl *WorkflowList) escapeFromPreview() bool {
	if !wl.workflowsActive() || !wl.previewModeEnabled() {
		return false
	}
	switch wl.focusPane {
	case focusEvents, focusEventDetail:
		wl.setPreviewVisible(false)
		return true
	}
	return false
}

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

func (wl *WorkflowList) activityDetailTableFocused() bool {
	if wl == nil {
		return false
	}
	if wl.previewKind == previewEvents {
		return true
	}
	return wl.previewKind == previewActivities && wl.activityDetailKind == activityDetailDetails
}

func (wl *WorkflowList) activityDetailFocusPrimitive() tview.Primitive {
	if wl.activityDetailTableFocused() && wl.activityDetail != nil {
		return wl.activityDetail
	}
	if wl.eventDetail != nil {
		return wl.eventDetail
	}
	return nil
}

func (wl *WorkflowList) renderSelectedActivityDetail() {
	defer wl.revealIOSearch()
	visible := wl.visiblePreviewActivities()
	if len(visible) == 0 {
		if wl.eventDetailTree != nil {
			wl.eventDetailTree.setContent("", false)
		}
		status := "No activities"
		if len(wl.previewActivities) > 0 && wl.previewActivitySearch != "" {
			status = "No matching activities"
		}
		wl.setActivityDetailStatus(status)
		if wl.eventDetail != nil {
			wl.eventDetail.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), status))
			wl.eventDetail.ScrollToBeginning()
		}
		return
	}
	a, ok := wl.selectedPreviewActivity()
	if !ok {
		a = visible[0]
	}
	switch wl.activityDetailKind {
	case activityDetailInput:
		if wl.eventDetail != nil {
			tree := ioTreeEnabled(wl.app)
			if wl.eventDetailTree != nil && wl.eventDetailTree.setContent(a.Input, tree) {
				return
			}
			wl.eventDetail.SetText(formatActivityInput(a, tree))
			wl.eventDetail.ScrollToBeginning()
		}
	case activityDetailOutput:
		if wl.eventDetail != nil {
			content := a.Result
			if content == "" {
				content = a.Failure
			}
			tree := ioTreeEnabled(wl.app)
			if wl.eventDetailTree != nil && wl.eventDetailTree.setContent(content, tree) {
				return
			}
			wl.eventDetail.SetText(formatActivityOutput(a, tree))
			wl.eventDetail.ScrollToBeginning()
		}
	default:
		if wl.eventDetailTree != nil {
			wl.eventDetailTree.setContent("", false)
		}
		wl.renderActivityDetailRows(a)
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

func (wl *WorkflowList) currentPreviewWorkflow() (temporal.Workflow, bool) {
	for _, w := range wl.workflows {
		if w.ID == wl.previewWorkflowID && w.RunID == wl.previewRunID {
			return w, true
		}
	}
	row := -1
	if wl.table != nil {
		row = wl.table.SelectedRow()
	}
	if row >= 0 && row < len(wl.workflows) {
		return wl.workflows[row], true
	}
	if len(wl.workflows) > 0 {
		return wl.workflows[0], true
	}
	return temporal.Workflow{}, false
}

func (wl *WorkflowList) setupPreview() {
	wl.eventTable = components.NewTable()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wl.eventTable.SetBorder(false)
	wl.eventTable.SetBackgroundColor(theme.Bg())
	wl.eventTable.SetEvaluateAllRows(true)
	wl.eventTableScroll = attachTableCharScroll(wl.eventTable, wl.app)

	wl.eventDetail = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetScrollable(true)
	setTextViewWrap(wl.eventDetail, ioWrapOn(wl.app))
	wl.eventDetail.SetBackgroundColor(theme.Bg())
	wl.eventDetail.SetTextColor(theme.Fg())
	attachTextViewScrollbar(wl.eventDetail, wl.app)
	wl.eventDetailTree = newJSONTreeSelection(wl.eventDetail)

	wl.eventTreeView = NewEventTreeView()
	wl.eventTreeView.SetBackgroundColor(theme.Bg())
	wl.eventTreeView.app = wl.app
	attachTreeScrollbar(wl.eventTreeView.TreeView, wl.app)
	wl.eventTreeView.SetOnSelectionChanged(func(node *temporal.EventTreeNode) {
		if wl.previewKind != previewEvents {
			return
		}
		wl.setActivityDetailRows(eventTreeInfoRows(node))
	})
	wl.eventTreeView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return wl.handlePreviewKeys(event)
	})

	wl.eventsPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Activities", theme.IconActivity))
	wl.eventsPanel.SetContent(wl.eventTableScroll)

	wl.activityDetail = components.NewTable()
	wl.activityDetail.SetBorder(false)
	wl.activityDetail.SetBackgroundColor(theme.Bg())
	wl.activityDetail.SetEvaluateAllRows(true)
	wl.activityDetailScroll = newCharScrollView(wl.activityDetail, func() int {
		return workflowInfoContentWidth(wl.activityDetailRows)
	}).withApp(wl.app)
	bindTableCharScroll(wl.activityDetail, wl.activityDetailScroll, func() int {
		return mouseScrollStepFromApp(wl.app)
	})
	wl.activityDetail.SetInputCapture(wl.handleActivityDetailKeys)

	wl.activityDetailTabs = components.NewTabs().
		SetShowIcons(true).
		SetShowBadges(false).
		AddTabWithIcon(activityDetailDetails.title(), activityDetailDetails.icon(), wl.activityDetailScroll).
		AddTabWithIcon(activityDetailInput.title(), activityDetailInput.icon(), wl.eventDetail).
		AddTabWithIcon(activityDetailOutput.title(), activityDetailOutput.icon(), wl.eventDetail).
		SetOnChange(func(index int, name string) {
			if index >= 0 && index < len(activityDetailTabOrder) {
				wl.setActivityDetailKind(activityDetailTabOrder[index])
			}
		}).
		SetActive(int(activityDetailDetails))
	wl.activityDetailTabs.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if handled := wl.handlePreviewKeys(event); handled == nil {
			return nil
		}
		if isJigTabsNavKey(event) {
			return nil
		}
		return event
	})

	wl.eventDetailPanel = components.NewPanel()
	wl.eventDetailPanel.SetContent(wl.activityDetailTabs)

	wl.workflowIOView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetScrollable(true)
	setTextViewWrap(wl.workflowIOView, ioWrapOn(wl.app))
	wl.workflowIOView.SetBackgroundColor(theme.Bg())
	wl.workflowIOView.SetTextColor(theme.Fg())
	attachTextViewScrollbar(wl.workflowIOView, wl.app)
	wl.workflowIOTree = newJSONTreeSelection(wl.workflowIOView)
	wl.workflowIOView.SetInputCapture(wl.capturePreviewTextView(wl.workflowIOView))

	wl.workflowIOTabs = components.NewTabs().
		SetShowIcons(true).
		SetShowBadges(false).
		AddTabWithIcon(workflowIOInput.title(), workflowIOInput.icon(), wl.workflowIOView).
		AddTabWithIcon(workflowIOOutput.title(), workflowIOOutput.icon(), wl.workflowIOView).
		SetOnChange(func(index int, name string) {
			if index >= 0 && index < len(workflowIOTabOrder) {
				wl.setWorkflowIOKind(workflowIOTabOrder[index])
			}
		}).
		SetActive(int(workflowIOInput))
	wl.workflowIOTabs.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if handled := wl.handlePreviewKeys(event); handled == nil {
			return nil
		}
		if isJigTabsNavKey(event) {
			return nil
		}
		return event
	})

	wl.workflowDetail = components.NewTable()
	wl.workflowDetail.SetBorder(false)
	wl.workflowDetail.SetBackgroundColor(theme.Bg())
	wl.workflowDetail.SetEvaluateAllRows(true)
	wl.workflowDetailScroll = newCharScrollView(wl.workflowDetail, func() int {
		return workflowInfoContentWidth(wl.previewDetailRows)
	}).withApp(wl.app)
	bindTableCharScroll(wl.workflowDetail, wl.workflowDetailScroll, func() int {
		return mouseScrollStepFromApp(wl.app)
	})
	wl.hierarchyView = NewWorkflowGraphView(wl.app, wl.namespace, nil)
	wl.hierarchyView.SetEmbedded(true)
	if wl.hierarchyView.tree != nil {
		wl.hierarchyView.tree.SetBackgroundColor(theme.Bg())
	}
	if wl.hierarchyView.graph != nil {
		wl.hierarchyView.graph.SetBackgroundColor(theme.Bg())
	}
	wl.hierarchyGraphPanel = components.NewPanel()
	wl.hierarchyGraphPanel.SetContent(wl.hierarchyView.graph)
	hierarchyInput := func(event *tcell.EventKey) *tcell.EventKey {
		if wl.hierarchyView.handleGraphKeys(event) {
			return nil
		}
		return wl.handlePreviewKeys(event)
	}
	if wl.hierarchyView.tree != nil {
		wl.hierarchyView.tree.SetInputCapture(hierarchyInput)
	}
	if wl.hierarchyView.graph != nil {
		wl.hierarchyView.graph.SetInputCapture(hierarchyInput)
	}

	wl.previewTabs = components.NewTabs().
		SetShowIcons(true).
		SetShowBadges(false).
		AddTabWithIcon(previewDetails.title(), previewDetails.icon(), wl.workflowDetailScroll).
		AddTabWithIcon(previewActivities.title(), previewActivities.icon(), wl.eventTableScroll).
		AddTabWithIcon(previewEvents.title(), previewEvents.icon(), wl.eventTableScroll).
		AddTabWithIcon(previewHierarchy.title(), previewHierarchy.icon(), wl.hierarchyView.tree)
	wl.previewTabs.SetActive(int(previewEvents))
	wl.eventTab = wl.previewTabs.GetActiveTab()
	wl.previewTabs.SetOnChange(func(index int, name string) {
		if index >= 0 && index < len(previewTabOrder) {
			wl.setPreviewKind(previewTabOrder[index])
		}
	}).SetActive(int(previewActivities))
	wl.applyEventsTabMode()

	wl.previewPanel = components.NewPanel()
	wl.previewPanel.SetContent(wl.previewTabs)

	wl.rightFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	wl.rightFlex.SetBackgroundColor(theme.Bg())

	wl.eventTable.SetSelectionChangedFunc(func(row, col int) {
		wl.updatePreviewSelection(row)
	})

	wl.eventTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if handleTableCharScroll(wl.eventTableScroll, wl.eventTable, event) {
			return nil
		}
		return wl.handlePreviewKeys(event)
	})
	wl.eventDetail.SetInputCapture(wl.capturePreviewTextView(wl.eventDetail))
	wl.workflowDetail.SetInputCapture(wl.handlePreviewDetailKeys)
	wl.previewTabs.SetInputCapture(wl.handlePreviewKeys)
	wl.setupTimeline()
}

func (wl *WorkflowList) capturePreviewTextView(view *tview.TextView) func(*tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		var tree *jsonTreeSelection
		switch view {
		case wl.workflowIOView:
			tree = wl.workflowIOTree
		case wl.eventDetail:
			tree = wl.eventDetailTree
		}
		if tree.handleKey(event) {
			return nil
		}
		if event != nil && event.Key() == tcell.KeyRune && event.Rune() == 'w' && wl.togglePreviewIOWrap(view) {
			return nil
		}
		if handleTextViewScroll(view, event) {
			return nil
		}
		return wl.handlePreviewKeys(event)
	}
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
	wl.app.ShowFilterMode(wl.focusedIOQuery(), FilterModeCallbacks{
		OnChange: wl.applyFocusedIOSearch,
		OnSubmit: wl.applyFocusedIOSearch,
		OnCancel: func() { wl.applyFocusedIOSearch("") },
	})
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
		if view != nil {
			count = countSearchMatches(view.GetText(true), query)
		}
		wl.eventDetailPanel.SetTitle(searchLabel(query, count))
	}
}

func (wl *WorkflowList) handlePreviewKeys(event *tcell.EventKey) *tcell.EventKey {
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
		if wl.escapeFromPreview() {
			return nil
		}
		wl.setFocusPane(focusWorkflows)
		return nil
	}
	if wl.handlePreviewTabKey(event) {
		return nil
	}
	switch event.Rune() {
	case 'p':
		wl.togglePreviewMode()
		return nil
	case 'b':
		if wl.previewKind == previewEvents {
			wl.toggleEventTree()
			return nil
		}
	case ' ':
		if wl.previewKind == previewEvents && wl.eventTreeMode && wl.eventTreeView != nil {
			wl.eventTreeView.ToggleSelected()
			return nil
		}
	case 'e':
		if wl.openPreviewIOInEditor() {
			return nil
		}
	case 'y':
		if wl.yankPreviewIO(false) {
			return nil
		}
	case 'Y':
		if ioTreeEnabled(wl.app) && wl.yankPreviewIO(true) {
			return nil
		}
	case 'i':
		if !wl.previewShowsIO() {
			return event
		}
		if wl.showPreviewIO() {
			return nil
		}
	case 'u':
		if wl.openSelectedWorkflowUI() {
			return nil
		}
	case '/':
		if wl.showFocusedIOSearch() {
			return nil
		}
		if wl.previewKind == previewEvents {
			wl.showPreviewEventSearch()
			return nil
		}
		if wl.previewKind == previewActivities {
			wl.showPreviewActivitySearch()
			return nil
		}
	case 'g':
		if wl.previewKind == previewEvents {
			wl.jumpToPreviewChild()
			return nil
		}
	case 'r':
		wl.refreshSelectedPreview()
		return nil
	case '|':
		if wl.previewKind == previewActivities {
			wl.showActivityColumnEditor()
			return nil
		}
	}
	return event
}

func (wl *WorkflowList) updatePreviewSelection(row int) {
	if row <= 0 {
		return
	}
	idx := row - 1
	if wl.previewKind == previewActivities {
		visible := wl.visiblePreviewActivities()
		if idx < len(visible) {
			if !wl.timelineSyncing {
				wl.highlightedActivityID = visible[idx].ScheduledID
			}
			wl.renderSelectedActivityDetail()
			if !wl.timelineSyncing {
				wl.syncTimelineFromActivity()
			}
		}
		return
	}
	wl.renderSelectedEventDetail()
}

func (wl *WorkflowList) previewFocusOrder() []workflowFocusPane {
	if wl.taskQueuesActive() {
		if wl.pollersVisible {
			return []workflowFocusPane{focusWorkflows, focusPollers}
		}
		return []workflowFocusPane{focusWorkflows}
	}
	if wl.schedulesActive() {
		if wl.scheduleDetailVisible {
			return []workflowFocusPane{focusWorkflows, focusScheduleDetail, focusScheduleRuns}
		}
		return []workflowFocusPane{focusWorkflows}
	}
	if wl.workersActive() {
		if wl.workerDetailVisible {
			return []workflowFocusPane{focusWorkflows, focusWorkerDetail}
		}
		return []workflowFocusPane{focusWorkflows}
	}
	order := []workflowFocusPane{}
	if wl.filtersOnSide() {
		order = append(order, focusFilters)
	}
	order = append(order, focusWorkflows)
	if wl.previewModeEnabled() {
		if wl.previewShowsSidePane() || wl.previewKind == previewHierarchy {
			order = append(order, focusEvents, focusEventDetail)
		} else {
			order = append(order, focusEventDetail)
		}
	}
	if wl.timelineVisible {
		order = append(order, focusTimeline)
	}
	return order
}

func (wl *WorkflowList) cycleFocus(delta int) {
	order := wl.previewFocusOrder()
	if len(order) == 0 {
		return
	}
	idx := 0
	for i, pane := range order {
		if pane == wl.focusPane {
			idx = i
			break
		}
	}
	next := (idx + delta) % len(order)
	if next < 0 {
		next += len(order)
	}
	wl.setFocusPane(order[next])
}

func (wl *WorkflowList) setFocusPane(pane workflowFocusPane) {
	if pane == focusTimeline {
		wl.syncTimelineFromActivity()
	}
	wl.focusPane = pane
	if wl.app == nil || wl.app.JigApp() == nil {
		wl.applyFocusStyles()
		return
	}
	switch pane {
	case focusPollers:
		if wl.taskQueues != nil {
			wl.app.JigApp().SetFocus(wl.taskQueues.pollerTable)
		}
	case focusScheduleDetail:
		if wl.schedules != nil {
			wl.app.JigApp().SetFocus(wl.schedules.detail)
		}
	case focusScheduleRuns:
		if wl.schedules != nil {
			wl.app.JigApp().SetFocus(wl.schedules.runsTable)
		}
	case focusWorkerDetail:
		if wl.workers != nil {
			wl.app.JigApp().SetFocus(wl.workers.detail)
		}
	case focusEvents:
		if p := wl.eventsPreviewPrimitive(); p != nil {
			wl.app.JigApp().SetFocus(p)
		} else {
			wl.app.JigApp().SetFocus(wl.eventTable)
		}
	case focusEventDetail:
		if wl.previewKind == previewDetails && wl.workflowIOView != nil {
			wl.app.JigApp().SetFocus(wl.workflowIOView)
		} else if wl.previewKind == previewHierarchy && wl.hierarchyView != nil && wl.hierarchyView.graph != nil {
			wl.app.JigApp().SetFocus(wl.hierarchyView.graph)
		} else if p := wl.activityDetailFocusPrimitive(); p != nil {
			wl.app.JigApp().SetFocus(p)
		} else {
			wl.app.JigApp().SetFocus(wl.eventDetail)
		}
	case focusTimeline:
		wl.app.JigApp().SetFocus(wl.timelineView)
	case focusFilters:
		if wl.filterBar != nil {
			wl.app.JigApp().SetFocus(wl.filterBar)
		} else {
			wl.app.JigApp().SetFocus(wl.table)
		}
	default:
		if wl.taskQueuesActive() && wl.taskQueues != nil {
			wl.app.JigApp().SetFocus(wl.taskQueues.queueTable)
		} else if wl.schedulesActive() && wl.schedules != nil {
			wl.app.JigApp().SetFocus(wl.schedules.table)
		} else if wl.workersActive() && wl.workers != nil {
			wl.app.JigApp().SetFocus(wl.workers.table)
		} else {
			wl.app.JigApp().SetFocus(wl.table)
		}
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) applyFocusStyles() {
	active := wl == nil || wl.app == nil || !wl.app.modalHasFocus()
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetFocused(active && (wl.focusPane == focusWorkflows || wl.focusPane == focusFilters))
	}
	if wl.previewPanel != nil {
		wl.previewPanel.SetFocused(active && wl.focusPane == focusEvents)
	}
	if wl.eventDetailPanel != nil {
		wl.eventDetailPanel.SetFocused(active && wl.previewShowsSidePane() && wl.focusPane == focusEventDetail)
	}
	if wl.hierarchyGraphPanel != nil {
		wl.hierarchyGraphPanel.SetFocused(active && wl.previewKind == previewHierarchy && wl.focusPane == focusEventDetail)
	}
	if wl.eventsPanel != nil {
		wl.eventsPanel.SetFocused(active && wl.focusPane == focusEvents)
	}
	if wl.timelinePanel != nil {
		wl.timelinePanel.SetFocused(active && wl.focusPane == focusTimeline)
	}
	if wl.taskQueues != nil && wl.taskQueues.pollerPanel != nil {
		wl.taskQueues.pollerPanel.SetFocused(active && wl.focusPane == focusPollers)
	}
	if wl.schedules != nil && wl.schedules.detailPanel != nil {
		wl.schedules.detailPanel.SetFocused(active && wl.focusPane == focusScheduleDetail)
	}
	if wl.schedules != nil && wl.schedules.runsPanel != nil {
		wl.schedules.runsPanel.SetFocused(active && wl.focusPane == focusScheduleRuns)
	}
	if wl.workers != nil && wl.workers.previewPanel != nil {
		wl.workers.previewPanel.SetFocused(active && wl.focusPane == focusWorkerDetail)
	}
	if wl.taskQueues != nil && wl.taskQueues.queueTable != nil {
		wl.taskQueues.queueTable.SetSelectable(active && wl.taskQueuesActive() && wl.focusPane == focusWorkflows, false)
	}
	if wl.taskQueues != nil && wl.taskQueues.pollerTable != nil {
		wl.taskQueues.pollerTable.SetSelectable(active && wl.taskQueuesActive() && wl.focusPane == focusPollers, false)
	}
	if wl.schedules != nil && wl.schedules.table != nil {
		wl.schedules.table.SetSelectable(active && wl.schedulesActive() && wl.focusPane == focusWorkflows, false)
	}
	if wl.schedules != nil && wl.schedules.detail != nil {
		wl.schedules.detail.SetSelectable(active && wl.schedulesActive() && wl.focusPane == focusScheduleDetail, false)
	}
	if wl.schedules != nil && wl.schedules.runsTable != nil {
		wl.schedules.runsTable.SetSelectable(active && wl.schedulesActive() && wl.focusPane == focusScheduleRuns, false)
	}
	if wl.workers != nil && wl.workers.table != nil {
		wl.workers.table.SetSelectable(active && wl.workersActive() && wl.focusPane == focusWorkflows, false)
	}
	if wl.table != nil {
		wl.table.SetSelectable(wl.workflowsActive(), false)
	}
	if wl.eventTable != nil {
		wl.eventTable.SetSelectable(wl.previewKind != previewHierarchy, false)
	}
	if wl.workflowDetail != nil {
		wl.workflowDetail.SetSelectable(active && wl.previewKind == previewDetails && wl.focusPane == focusEvents, false)
	}
	if wl.activityDetail != nil {
		wl.activityDetail.SetSelectable(active && wl.activityDetailTableFocused() && wl.focusPane == focusEventDetail, false)
	}
}

func (wl *WorkflowList) syncFocusFromPrimitives() {
	var pane workflowFocusPane
	switch {
	case wl.workflowDetail != nil && wl.workflowDetail.HasFocus():
		pane = focusEvents
	case wl.workflowIOView != nil && wl.workflowIOView.HasFocus():
		pane = focusEventDetail
	case wl.workflowIOTabs != nil && wl.workflowIOTabs.HasFocus():
		pane = focusEventDetail
	case wl.activityDetail != nil && wl.activityDetail.HasFocus():
		pane = focusEventDetail
	case wl.activityDetailTabs != nil && wl.activityDetailTabs.HasFocus():
		pane = focusEventDetail
	case wl.eventDetail != nil && wl.eventDetail.HasFocus():
		pane = focusEventDetail
	case wl.hierarchyView != nil && wl.hierarchyView.graph != nil && wl.hierarchyView.graph.HasFocus():
		pane = focusEventDetail
	case wl.hierarchyView != nil && wl.hierarchyView.tree != nil && wl.hierarchyView.tree.HasFocus():
		pane = focusEvents
	case wl.eventTreeView != nil && wl.eventTreeView.HasFocus():
		pane = focusEvents
	case wl.eventTable != nil && wl.eventTable.HasFocus():
		pane = focusEvents
	case wl.timelineView != nil && wl.timelineView.HasFocus():
		pane = focusTimeline
	case wl.taskQueues != nil && wl.taskQueues.pollerTable != nil && wl.taskQueues.pollerTable.HasFocus():
		pane = focusPollers
	case wl.schedules != nil && wl.schedules.detail != nil && wl.schedules.detail.HasFocus():
		pane = focusScheduleDetail
	case wl.schedules != nil && wl.schedules.runsTable != nil && wl.schedules.runsTable.HasFocus():
		pane = focusScheduleRuns
	case wl.workers != nil && wl.workers.detail != nil && wl.workers.detail.HasFocus():
		pane = focusWorkerDetail
	case wl.taskQueues != nil && wl.taskQueues.queueTable != nil && wl.taskQueues.queueTable.HasFocus():
		pane = focusWorkflows
	case wl.taskQueues != nil && wl.taskQueues.HasFocus():
		pane = focusWorkflows
	case wl.schedules != nil && wl.schedules.table != nil && wl.schedules.table.HasFocus():
		pane = focusWorkflows
	case wl.workers != nil && wl.workers.table != nil && wl.workers.table.HasFocus():
		pane = focusWorkflows
	case wl.filterBar != nil && wl.filterBar.HasFocus():
		pane = focusFilters
	case wl.table != nil && wl.table.HasFocus():
		pane = focusWorkflows
	default:
		wl.applyFocusStyles()
		return
	}
	if pane != wl.focusPane {
		wl.focusPane = pane
	}
	wl.applyFocusStyles()
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

func (wl *WorkflowList) clearPreviewContent() {
	wl.previewEvents = nil
	wl.previewActivities = nil
	if wl.timelineView != nil {
		wl.timelineView.SetNodes(nil)
	}
	if wl.eventTreeView != nil {
		wl.eventTreeView.SetNodes(nil)
	}
}

func (wl *WorkflowList) clearPreview() {
	wl.previewPending = false
	wl.clearPreviewContent()
	wl.previewWorkflowID = ""
	wl.previewRunID = ""
	wl.highlightedActivityID = 0
	if wl.eventTable != nil {
		wl.eventTable.ClearRows()
		wl.applyActivityTableHeaders()
	}
	if wl.eventDetail != nil {
		wl.eventDetail.SetText(fmt.Sprintf("[%s]Select a workflow to load preview[-]", theme.TagFgDim()))
	}
	wl.setActivityDetailStatus("Select a workflow to load preview")
	wl.setPreviewDetailStatus("Select a workflow to load preview")
	wl.renderWorkflowIO()
	wl.syncPreviewChrome()
}

func (wl *WorkflowList) setPreviewStatus(message string) {
	wl.syncPreviewChrome()
	if wl.previewKind == previewDetails {
		wl.setPreviewDetailStatus(message)
		if wl.workflowIOView != nil {
			wl.workflowIOView.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), message))
		}
		return
	}
	if wl.previewKind == previewHierarchy {
		return
	}
	if wl.eventTable != nil {
		wl.eventTable.ClearRows()
		if wl.previewKind == previewActivities {
			wl.applyActivityTableHeaders()
		} else {
			setTableHeaders(wl.eventTable, "ID", "TIME", "TYPE", "NAME")
		}
		if message != "" {
			wl.eventTable.AddStyledRow([]components.TableCell{
				{Text: message, Color: theme.FgDim(), Selectable: true},
			})
		}
	}
	if wl.previewKind == previewEvents && wl.eventTreeView != nil {
		wl.eventTreeView.SetNodes(nil)
	}
	if wl.eventDetail != nil {
		wl.eventDetail.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), message))
	}
	wl.setActivityDetailStatus(message)
}

func (wl *WorkflowList) schedulePreview(w temporal.Workflow, force bool) {
	if !wl.historyNeeded() {
		return
	}
	if !force && wl.previewWorkflowID == w.ID && wl.previewRunID == w.RunID {
		return
	}
	if wl.previewWorkflowID != w.ID || wl.previewRunID != w.RunID {
		wl.highlightedActivityID = 0
	}

	alreadyShowing := wl.previewWorkflowID == w.ID && wl.previewRunID == w.RunID && len(wl.previewEvents) > 0

	if !force {
		if events, ok := wl.previewCache.get(w.ID, w.RunID); ok {
			atomic.AddUint64(&wl.previewGen, 1)
			wl.previewWorkflowID = w.ID
			wl.previewRunID = w.RunID
			wl.previewPending = false
			if wl.previewTimer != nil {
				wl.previewTimer.Stop()
			}
			wl.showPreviewEvents(w, events)
			return
		}
	}

	gen := atomic.AddUint64(&wl.previewGen, 1)
	wl.previewWorkflowID = w.ID
	wl.previewRunID = w.RunID
	if !alreadyShowing {
		wl.previewPending = true
		wl.clearPreviewContent()
		wl.setPreviewStatus("Loading...")
		if wl.previewKind == previewHierarchy {
			wl.renderPreviewHierarchy(w)
		}
	}

	if wl.previewTimer != nil {
		wl.previewTimer.Stop()
	}
	wl.previewTimer = time.AfterFunc(wl.previewLoadDelay(), func() {
		wl.loadPreview(gen, w)
	})
}

func (wl *WorkflowList) loadPreview(gen uint64, w temporal.Workflow) {
	if atomic.LoadUint64(&wl.previewGen) != gen {
		return
	}

	var events []temporal.EnhancedHistoryEvent
	var fresh *temporal.Workflow
	var err error
	if wl.app != nil {
		if provider := wl.app.Provider(); provider != nil {
			wl.app.SetViewLoading("workflow-preview", true)
			defer wl.app.SetViewLoading("workflow-preview", false)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			events, err = provider.GetEnhancedWorkflowHistory(ctx, wl.namespace, w.ID, w.RunID)
			if err == nil {
				if got, gerr := provider.GetWorkflow(ctx, wl.namespace, w.ID, w.RunID); gerr == nil && got != nil {
					fresh = got
				}
			}
		} else {
			events = mockPreviewEvents(w)
		}
	} else {
		events = mockPreviewEvents(w)
	}
	if atomic.LoadUint64(&wl.previewGen) != gen {
		return
	}

	apply := func() {
		if atomic.LoadUint64(&wl.previewGen) != gen {
			return
		}
		if err != nil {
			wl.previewPending = false
			wl.setPreviewStatus("Failed to load preview: " + err.Error())
			return
		}
		if fresh != nil {
			w = *fresh
		}
		if w.Status == "Running" && temporal.HistoryHasTaskFailure(events) {
			w.TaskFailure = true
		}
		if fresh != nil || w.TaskFailure {
			wl.mergeWorkflow(w)
		}
		wl.showPreviewEvents(w, events)
	}
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.app.JigApp().QueueUpdateDraw(apply)
		return
	}
	apply()
}

func (wl *WorkflowList) showPreviewEvents(w temporal.Workflow, events []temporal.EnhancedHistoryEvent) {
	wl.previewPending = false
	wl.previewEvents = events
	wl.previewActivities = previewActivitiesFromEvents(events)
	wl.previewCache.put(w.ID, w.RunID, events)
	if wl.highlightedActivityID != 0 {
		found := false
		for _, a := range wl.previewActivities {
			if a.ScheduledID == wl.highlightedActivityID {
				found = true
				break
			}
		}
		if !found {
			wl.highlightedActivityID = 0
		}
	}
	wl.renderPreview(w)
	wl.refreshTimeline()
}

func (wl *WorkflowList) renderPreview(w temporal.Workflow) {
	if wl.previewPending {
		wl.setPreviewStatus("Loading...")
		if wl.previewKind == previewHierarchy {
			wl.renderPreviewHierarchy(w)
		}
		return
	}
	switch wl.previewKind {
	case previewDetails:
		wl.renderPreviewDetails(w)
	case previewEvents:
		wl.renderPreviewEvents(w)
	case previewHierarchy:
		wl.renderPreviewHierarchy(w)
	default:
		wl.renderPreviewActivities(w)
	}
}

func (wl *WorkflowList) renderPreviewHierarchy(w temporal.Workflow) {
	if wl.hierarchyView == nil {
		return
	}
	wl.hierarchyView.ShowWorkflow(wl.namespace, w)
}

func (wl *WorkflowList) renderPreviewDetails(w temporal.Workflow) {
	wl.syncPreviewChrome()
	if wl.workflowDetail == nil {
		return
	}
	selectedKey := ""
	if row, ok := wl.selectedPreviewDetailRow(); ok {
		selectedKey = row.Key
	}
	wl.previewDetailRows = workflowInfoRows(time.Now(), w)
	wl.workflowDetail.ClearRows()
	for _, row := range wl.previewDetailRows {
		wl.workflowDetail.AddStyledRow([]components.TableCell{
			{Text: row.Label, Color: theme.FgDim(), Selectable: true},
			{Text: row.displayText(), Color: row.Color, Selectable: true},
		})
	}
	if idx := workflowInfoRowIndex(wl.previewDetailRows, selectedKey); idx >= 0 {
		wl.workflowDetail.SelectRow(idx)
	} else if len(wl.previewDetailRows) > 0 {
		wl.workflowDetail.SelectRow(0)
	}
	if wl.workflowDetailScroll != nil {
		wl.workflowDetailScroll.clamp()
	}
	wl.renderWorkflowIO()
}

func (wl *WorkflowList) eventsPreviewPrimitive() tview.Primitive {
	if wl.previewKind == previewDetails && wl.workflowDetail != nil {
		return wl.workflowDetail
	}
	if wl.previewKind == previewHierarchy && wl.hierarchyView != nil && wl.hierarchyView.tree != nil {
		return wl.hierarchyView.tree
	}
	if wl.previewKind == previewEvents && wl.eventTreeMode && wl.eventTreeView != nil {
		return wl.eventTreeView
	}
	return wl.eventTable
}

func (wl *WorkflowList) applyEventsTabMode() {
	if wl.eventTab != nil {
		if wl.eventTreeMode && wl.eventTreeView != nil {
			wl.eventTab.Content = wl.eventTreeView
		} else {
			wl.eventTab.Content = wl.eventTableScroll
		}
	}
	if wl.focusPane == focusEvents && wl.previewKind == previewEvents {
		if wl.app != nil && wl.app.JigApp() != nil {
			wl.setFocusPane(focusEvents)
			return
		}
		wl.applyFocusStyles()
	}
}

func (wl *WorkflowList) toggleEventTree() {
	if wl.previewKind != previewEvents {
		return
	}
	wl.eventTreeMode = !wl.eventTreeMode
	wl.applyEventsTabMode()
	if w, ok := wl.currentPreviewWorkflow(); ok {
		wl.renderPreviewEvents(w)
		return
	}
	wl.renderPreviewEvents(temporal.Workflow{})
}

func (wl *WorkflowList) renderPreviewEvents(w temporal.Workflow) {
	wl.syncPreviewChrome()
	events := wl.visiblePreviewEvents()
	selectedID := int64(0)
	if ev, ok := wl.selectedPreviewEvent(); ok {
		selectedID = ev.ID
	}
	if wl.eventTreeView != nil {
		wl.eventTreeView.SetNodes(temporal.BuildEventTree(events))
	}
	wl.eventTable.ClearRows()
	setTableHeaders(wl.eventTable, "ID", "TIME", "TYPE", "NAME")
	if len(events) == 0 {
		if wl.previewEventSearch != "" {
			wl.setActivityDetailStatus("No matching events")
		} else {
			wl.setActivityDetailStatus("No events")
		}
		return
	}
	for _, ev := range events {
		name := getEventNameDetail(&ev)
		wl.eventTable.AddRowWithColor(eventColor(ev.Type),
			fmt.Sprintf("%d", ev.ID),
			ev.Time.Format("15:04:05"),
			eventIcon(ev.Type)+" "+truncateStr(ev.Type, 28),
			name,
		)
	}
	idx := 0
	if selectedID != 0 {
		for i, ev := range events {
			if ev.ID == selectedID {
				idx = i
				break
			}
		}
	}
	wl.eventTable.SelectRow(idx)
	wl.renderSelectedEventDetail()
}

func (wl *WorkflowList) renderPreviewActivities(w temporal.Workflow) {
	wl.syncPreviewChrome()
	activities := wl.visiblePreviewActivities()
	wl.eventTable.ClearRows()
	wl.applyActivityTableHeaders()
	if len(activities) == 0 {
		if wl.previewActivitySearch != "" {
			wl.setActivityDetailStatus("No matching activities")
		} else {
			wl.setActivityDetailStatus("No activities")
		}
		if wl.eventDetail != nil {
			if wl.previewActivitySearch != "" {
				wl.eventDetail.SetText(fmt.Sprintf("[%s]No matching activities[-]", theme.TagFgDim()))
			} else {
				wl.eventDetail.SetText(fmt.Sprintf("[%s]No activities[-]", theme.TagFgDim()))
			}
		}
		return
	}
	now := time.Now()
	for _, a := range activities {
		wl.eventTable.AddStyledRow(wl.styledActivityCells(now, a))
	}
	idx := 0
	if wl.highlightedActivityID != 0 {
		found := false
		for i, a := range activities {
			if a.ScheduledID == wl.highlightedActivityID {
				idx = i
				found = true
				break
			}
		}
		if !found {
			wl.highlightedActivityID = activities[0].ScheduledID
		}
	} else {
		wl.highlightedActivityID = activities[0].ScheduledID
	}
	wl.eventTable.SelectRow(idx)
	wl.renderSelectedActivityDetail()
}

func mockPreviewEvents(w temporal.Workflow) []temporal.EnhancedHistoryEvent {
	started := w.StartTime
	if started.IsZero() {
		started = time.Now().Add(-2 * time.Minute)
	}
	events := []temporal.EnhancedHistoryEvent{
		{
			ID:      1,
			Type:    "WorkflowExecutionStarted",
			Time:    started,
			Details: "taskQueue: " + w.TaskQueue,
			Input:   `{"orderId":"` + w.ID + `","items":2}`,
		},
		{
			ID:           5,
			Type:         "ActivityTaskScheduled",
			Time:         started.Add(10 * time.Second),
			ActivityType: "MockActivity",
			ActivityID:   "1",
			TaskQueue:    w.TaskQueue,
		},
		{
			ID:               6,
			Type:             "ActivityTaskStarted",
			Time:             started.Add(15 * time.Second),
			ActivityType:     "MockActivity",
			ScheduledEventID: 5,
			Attempt:          1,
		},
		{
			ID:               7,
			Type:             "ActivityTaskCompleted",
			Time:             started.Add(30 * time.Second),
			ActivityType:     "MockActivity",
			ScheduledEventID: 5,
			Result:           `{"ok":true}`,
		},
	}
	if w.EndTime != nil {
		endType := "WorkflowExecutionCompleted"
		if w.Status == "Failed" {
			endType = "WorkflowExecutionFailed"
		}
		end := temporal.EnhancedHistoryEvent{
			ID:      8,
			Type:    endType,
			Time:    *w.EndTime,
			Details: "status: " + w.Status,
		}
		if endType == "WorkflowExecutionFailed" {
			end.Failure = "mock failure: activity exhausted its retries"
		} else {
			end.Result = `{"status":"ok","processed":2}`
		}
		events = append(events, end)
	}
	return events
}
