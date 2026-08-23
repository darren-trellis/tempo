package view

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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
	if wl.eventTable == nil {
		return previewActivity{}, false
	}
	row := wl.eventTable.SelectedRow()
	if row < 0 && len(wl.previewActivities) > 0 {
		row = 0
	}
	if row < 0 || row >= len(wl.previewActivities) {
		return previewActivity{}, false
	}
	return wl.previewActivities[row], true
}

func (wl *WorkflowList) previewIOPayload() (title, input, output string, ok bool) {
	if !wl.previewModeEnabled() {
		return "", "", "", false
	}
	if wl.previewKind == previewActivities {
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
	row := wl.table.SelectedRow()
	if row < 0 || row >= len(wl.workflows) {
		return "", "", "", false
	}
	wf := wl.workflows[row]
	if wl.previewWorkflowID != wf.ID || wl.previewRunID != wf.RunID {
		return "", "", "", false
	}
	input, output = workflowIOFromEvents(wl.previewEvents)
	return wf.Type, input, output, true
}

func (wl *WorkflowList) showPreviewIO() bool {
	if !wl.previewModeEnabled() {
		return false
	}
	if wl.previewKind == previewActivities {
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
	} else {
		row := wl.table.SelectedRow()
		if row < 0 || row >= len(wl.workflows) {
			return false
		}
		wf := wl.workflows[row]
		if wl.previewWorkflowID != wf.ID || wl.previewRunID != wf.RunID {
			if wl.app != nil {
				wl.app.ToastError("Events still loading")
			}
			return true
		}
	}
	title, input, output, ok := wl.previewIOPayload()
	if !ok {
		if wl.app != nil {
			wl.app.ToastError("Nothing to show")
		}
		return true
	}
	restore := wl.focusPane
	if wl.previewKind == previewActivities && restore == focusWorkflows {
		restore = focusEvents
	}
	wl.keepDataOnStart = true
	showWorkflowIO(wl.app, wl, title, input, output, func() {
		wl.setFocusPane(restore)
	})
	return true
}

func (wl *WorkflowList) activateSelectedWorkflow() {
	row := wl.table.SelectedRow()
	if row < 0 || row >= len(wl.workflows) {
		return
	}
	wf := wl.workflows[row]
	if wl.previewModeEnabled() {
		wl.schedulePreview(wf, false)
		if wl.previewKind == previewDetails {
			wl.setFocusPane(focusEventDetail)
		} else {
			wl.setFocusPane(focusEvents)
		}
		return
	}
	if wl.app != nil {
		wl.app.NavigateToWorkflowDetail(wf.ID, wf.RunID)
	}
}

func (wl *WorkflowList) paneAt(x, y int) (workflowFocusPane, bool) {
	if wl.timelineVisible && wl.timelinePanel != nil && wl.timelinePanel.InRect(x, y) {
		return focusTimeline, true
	}
	if wl.previewModeEnabled() {
		if _, ok := wl.previewTabAt(x, y); ok {
			return focusWorkflows, false
		}
		if wl.previewKind == previewDetails {
			if wl.workflowDetailScroll != nil && wl.workflowDetailScroll.InRect(x, y) {
				return focusEventDetail, true
			}
			if wl.workflowDetail != nil && wl.workflowDetail.InRect(x, y) {
				return focusEventDetail, true
			}
		} else {
			if wl.eventDetailPanel != nil && wl.eventDetailPanel.InRect(x, y) {
				return focusEventDetail, true
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
	if wl.taskQueuesActive() {
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
		if wl.previewTimer != nil {
			wl.previewTimer.Stop()
		}
		if wl.eventTable != nil && !wl.timelineVisible {
			wl.clearPreview()
		}
		wl.focusPane = focusWorkflows
	}

	wl.applyMainLayout()

	if on {
		wl.applyPreviewPage()
		if len(wl.workflows) > 0 && wl.table != nil {
			row := wl.table.SelectedRow()
			if row < 0 || row >= len(wl.workflows) {
				row = 0
			}
			wl.schedulePreview(wl.workflows[row], false)
		}
	}

	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
		return
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) togglePreviewMode() {
	wl.previewMode = !wl.previewMode
	wl.applyPreviewLayout()
}

func (wl *WorkflowList) cyclePreviewKind(delta int) {
	if !wl.previewModeEnabled() {
		return
	}
	next := (int(wl.previewKind) + delta) % 3
	if next < 0 {
		next += 3
	}
	wl.setPreviewKind(previewKind(next))
}

func (wl *WorkflowList) handlePreviewTabKey(event *tcell.EventKey) bool {
	if event == nil || !wl.previewModeEnabled() {
		return false
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
	}
	return false
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
		if wl.previewKind == previewDetails {
			wl.setFocusPane(focusEventDetail)
		} else {
			wl.setFocusPane(focusEvents)
		}
		return
	}
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.app.JigApp().Menu().SetHints(wl.Hints())
	}
}

func (wl *WorkflowList) applyPreviewPage() {
	if wl.previewTabs != nil && wl.previewTabs.GetActive() != int(wl.previewKind) {
		wl.previewTabs.SetActive(int(wl.previewKind))
	}
	if wl.rightFlex != nil {
		wl.rightFlex.Clear()
		if wl.previewPanel != nil {
			wl.rightFlex.AddItem(wl.previewPanel, 0, 3, false)
		}
		if wl.previewKind != previewDetails && wl.eventDetailPanel != nil {
			wl.rightFlex.AddItem(wl.eventDetailPanel, 0, 2, false)
		}
	}
	wl.syncPreviewChrome()
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

	wl.eventDetail = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetWordWrap(true).
		SetScrollable(true)
	wl.eventDetail.SetBackgroundColor(theme.Bg())
	wl.eventDetail.SetTextColor(theme.Fg())

	wl.eventsPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Activities", theme.IconActivity))
	wl.eventsPanel.SetContent(wl.eventTable)

	wl.eventDetailPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Activity Details", theme.IconActivity))
	wl.eventDetailPanel.SetContent(wl.eventDetail)

	wl.workflowDetail = components.NewTable()
	wl.workflowDetail.SetBorder(false)
	wl.workflowDetail.SetBackgroundColor(theme.Bg())
	wl.workflowDetail.SetEvaluateAllRows(true)
	wl.workflowDetailScroll = newCharScrollView(wl.workflowDetail, func() int {
		return workflowInfoContentWidth(wl.previewDetailRows)
	})
	bindTableCharScroll(wl.workflowDetail, wl.workflowDetailScroll, func() int {
		return mouseScrollStepFromApp(wl.app)
	})
	wl.workflowDetail.SetSelectionChangedFunc(func(row, col int) {
		if wl.app != nil && wl.app.JigApp() != nil && wl.app.JigApp().Menu() != nil {
			wl.app.JigApp().Menu().SetHints(wl.Hints())
		}
	})

	wl.previewTabs = components.NewTabs().
		SetShowIcons(true).
		SetShowBadges(false).
		AddTabWithIcon(previewDetails.title(), previewDetails.icon(), wl.workflowDetailScroll).
		AddTabWithIcon(previewActivities.title(), previewActivities.icon(), wl.eventTable).
		AddTabWithIcon(previewEvents.title(), previewEvents.icon(), wl.eventTable).
		SetOnChange(func(index int, name string) {
			if index >= 0 && index < len(previewTabOrder) {
				wl.setPreviewKind(previewTabOrder[index])
			}
		}).
		SetActive(int(previewActivities))

	wl.previewPanel = components.NewPanel()
	wl.previewPanel.SetContent(wl.previewTabs)

	wl.rightFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	wl.rightFlex.SetBackgroundColor(theme.Bg())

	wl.eventTable.SetSelectionChangedFunc(func(row, col int) {
		wl.updatePreviewSelection(row)
	})

	bindTableHorizontalScroll(wl.eventTable, func() int {
		return mouseScrollStepFromApp(wl.app)
	})
	wl.eventTable.SetInputCapture(wl.handlePreviewKeys)
	wl.eventDetail.SetInputCapture(wl.capturePreviewTextView(wl.eventDetail))
	wl.workflowDetail.SetInputCapture(wl.handlePreviewDetailKeys)
	wl.previewTabs.SetInputCapture(wl.handlePreviewKeys)
	wl.setupTimeline()
}

func (wl *WorkflowList) capturePreviewTextView(view *tview.TextView) func(*tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		if handleTextViewScroll(view, event) {
			return nil
		}
		return wl.handlePreviewKeys(event)
	}
}

func (wl *WorkflowList) handlePreviewKeys(event *tcell.EventKey) *tcell.EventKey {
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
	if wl.handlePreviewTabKey(event) {
		return nil
	}
	switch event.Rune() {
	case 'p':
		wl.togglePreviewMode()
		return nil
	case 'z':
		wl.toggleTimeline()
		return nil
	case 'b':
		wl.toggleWorkflowTree()
		return nil
	case 'i':
		if wl.showPreviewIO() {
			return nil
		}
	case 'e':
		if wl.app != nil && wl.previewWorkflowID != "" {
			wl.app.NavigateToEvents(wl.previewWorkflowID, wl.previewRunID)
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
		if idx < len(wl.previewActivities) {
			if !wl.timelineSyncing {
				wl.highlightedActivityID = wl.previewActivities[idx].ScheduledID
			}
			wl.eventDetail.SetText(formatSelectedActivityDetail(wl.previewActivities[idx]))
			wl.eventDetail.ScrollToBeginning()
			if !wl.timelineSyncing {
				wl.syncTimelineFromActivity()
			}
		}
		return
	}
	if idx < len(wl.previewEvents) {
		wl.eventDetail.SetText(formatSelectedEventDetail(wl.previewEvents[idx]))
		wl.eventDetail.ScrollToBeginning()
	}
}

func (wl *WorkflowList) previewFocusOrder() []workflowFocusPane {
	order := []workflowFocusPane{focusWorkflows}
	if wl.previewModeEnabled() {
		if wl.previewKind == previewDetails {
			order = append(order, focusEventDetail)
		} else {
			order = append(order, focusEvents, focusEventDetail)
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
	case focusEvents:
		wl.app.JigApp().SetFocus(wl.eventTable)
	case focusEventDetail:
		if wl.previewKind == previewDetails {
			wl.app.JigApp().SetFocus(wl.workflowDetail)
		} else {
			wl.app.JigApp().SetFocus(wl.eventDetail)
		}
	case focusTimeline:
		wl.app.JigApp().SetFocus(wl.timelineView)
	default:
		if wl.taskQueuesActive() && wl.taskQueues != nil {
			wl.app.JigApp().SetFocus(wl.taskQueues)
		} else {
			wl.app.JigApp().SetFocus(wl.table)
		}
	}
	wl.applyFocusStyles()
	wl.app.JigApp().Menu().SetHints(wl.Hints())
}

func (wl *WorkflowList) applyFocusStyles() {
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetFocused(wl.focusPane == focusWorkflows)
	}
	if wl.previewPanel != nil {
		wl.previewPanel.SetFocused(wl.focusPane == focusEvents || (wl.previewKind == previewDetails && wl.focusPane == focusEventDetail))
	}
	if wl.eventDetailPanel != nil {
		wl.eventDetailPanel.SetFocused(wl.previewKind != previewDetails && wl.focusPane == focusEventDetail)
	}
	if wl.eventsPanel != nil {
		wl.eventsPanel.SetFocused(wl.focusPane == focusEvents)
	}
	if wl.timelinePanel != nil {
		wl.timelinePanel.SetFocused(wl.focusPane == focusTimeline)
	}
	if wl.table != nil {
		wl.table.SetSelectable(wl.focusPane == focusWorkflows, false)
	}
	if wl.eventTable != nil {
		wl.eventTable.SetSelectable(wl.focusPane == focusEvents, false)
	}
	if wl.workflowDetail != nil {
		wl.workflowDetail.SetSelectable(wl.previewKind == previewDetails && wl.focusPane == focusEventDetail, false)
	}
}

func (wl *WorkflowList) syncFocusFromPrimitives() {
	var pane workflowFocusPane
	switch {
	case wl.workflowDetail != nil && wl.workflowDetail.HasFocus():
		pane = focusEventDetail
	case wl.eventDetail != nil && wl.eventDetail.HasFocus():
		pane = focusEventDetail
	case wl.eventTable != nil && wl.eventTable.HasFocus():
		pane = focusEvents
	case wl.timelineView != nil && wl.timelineView.HasFocus():
		pane = focusTimeline
	case wl.taskQueues != nil && wl.taskQueues.HasFocus():
		pane = focusWorkflows
	case wl.table != nil && wl.table.HasFocus():
		pane = focusWorkflows
	default:
		wl.applyFocusStyles()
		return
	}
	if pane != wl.focusPane {
		wl.focusPane = pane
		if wl.app != nil && wl.app.JigApp() != nil {
			wl.app.JigApp().Menu().SetHints(wl.Hints())
		}
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

func (wl *WorkflowList) syncPreviewChrome() {
	if wl.eventDetailPanel == nil {
		return
	}
	if wl.previewKind == previewActivities {
		wl.eventDetailPanel.SetTitle(fmt.Sprintf("%s Activity Details", theme.IconActivity))
		return
	}
	wl.eventDetailPanel.SetTitle(fmt.Sprintf("%s Event Details", theme.IconEvent))
}

func (wl *WorkflowList) clearPreview() {
	wl.previewEvents = nil
	wl.previewActivities = nil
	wl.previewWorkflowID = ""
	wl.previewRunID = ""
	wl.highlightedActivityID = 0
	if wl.timelineView != nil {
		wl.timelineView.SetNodes(nil)
	}
	if wl.eventTable != nil {
		wl.eventTable.ClearRows()
		wl.eventTable.SetHeaders("STATUS", "NAME", "STARTED", "DURATION")
	}
	if wl.eventDetail != nil {
		wl.eventDetail.SetText(fmt.Sprintf("[%s]Select a workflow to load preview[-]", theme.TagFgDim()))
	}
	wl.setPreviewDetailStatus("Select a workflow to load preview")
	wl.syncPreviewChrome()
}

func (wl *WorkflowList) setPreviewStatus(message string) {
	wl.syncPreviewChrome()
	if wl.previewKind == previewDetails {
		wl.setPreviewDetailStatus(message)
		return
	}
	wl.eventTable.ClearRows()
	if wl.previewKind == previewActivities {
		wl.eventTable.SetHeaders("STATUS", "NAME", "STARTED", "DURATION")
	} else {
		wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	}
	wl.eventDetail.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), message))
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

	if !force {
		if events, ok := wl.previewCache.get(w.ID, w.RunID); ok {
			atomic.AddUint64(&wl.previewGen, 1)
			wl.previewWorkflowID = w.ID
			wl.previewRunID = w.RunID
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
	if wl.previewKind == previewDetails {
		wl.renderPreviewDetails(w)
	} else {
		wl.setPreviewStatus("Loading...")
	}

	if wl.previewTimer != nil {
		wl.previewTimer.Stop()
	}
	wl.previewTimer = time.AfterFunc(200*time.Millisecond, func() {
		wl.loadPreview(gen, w)
	})
}

func (wl *WorkflowList) loadPreview(gen uint64, w temporal.Workflow) {
	if atomic.LoadUint64(&wl.previewGen) != gen {
		return
	}

	provider := wl.app.Provider()
	if provider == nil {
		events := mockPreviewEvents(w)
		wl.app.JigApp().QueueUpdateDraw(func() {
			if atomic.LoadUint64(&wl.previewGen) != gen {
				return
			}
			wl.showPreviewEvents(w, events)
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	events, err := provider.GetEnhancedWorkflowHistory(ctx, wl.namespace, w.ID, w.RunID)
	if atomic.LoadUint64(&wl.previewGen) != gen {
		return
	}

	wl.app.JigApp().QueueUpdateDraw(func() {
		if atomic.LoadUint64(&wl.previewGen) != gen {
			return
		}
		if err != nil {
			wl.setPreviewStatus("Failed to load preview: " + err.Error())
			return
		}
		wl.showPreviewEvents(w, events)
	})
}

func (wl *WorkflowList) showPreviewEvents(w temporal.Workflow, events []temporal.EnhancedHistoryEvent) {
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
	switch wl.previewKind {
	case previewDetails:
		wl.renderPreviewDetails(w)
	case previewEvents:
		wl.renderPreviewEvents(w)
	default:
		wl.renderPreviewActivities(w)
	}
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
}

func (wl *WorkflowList) renderPreviewEvents(w temporal.Workflow) {
	wl.syncPreviewChrome()
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	if len(wl.previewEvents) == 0 {
		wl.eventDetail.SetText(fmt.Sprintf("[%s]No events[-]", theme.TagFgDim()))
		return
	}
	for _, ev := range wl.previewEvents {
		name := getEventNameDetail(&ev)
		wl.eventTable.AddRowWithColor(eventColor(ev.Type),
			fmt.Sprintf("%d", ev.ID),
			ev.Time.Format("15:04:05"),
			eventIcon(ev.Type)+" "+truncateStr(ev.Type, 28),
			name,
		)
	}
	wl.eventTable.SelectRow(0)
	wl.eventDetail.SetText(formatSelectedEventDetail(wl.previewEvents[0]))
}

func (wl *WorkflowList) renderPreviewActivities(w temporal.Workflow) {
	wl.syncPreviewChrome()
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("STATUS", "NAME", "STARTED", "DURATION")
	if len(wl.previewActivities) == 0 {
		wl.eventDetail.SetText(fmt.Sprintf("[%s]No activities[-]", theme.TagFgDim()))
		return
	}
	for _, a := range wl.previewActivities {
		name := a.Type
		if name == "" {
			name = "Activity"
		}
		status := temporal.GetActivityStatus(a.Status)
		statusText := a.Status
		if icon := status.Icon(); icon != "" {
			statusText = icon + " " + a.Status
		}
		wl.eventTable.AddRowWithColor(status.Color(),
			statusText,
			truncateStr(name, 28),
			a.StartTime.Format("15:04:05"),
			a.duration(),
		)
	}
	idx := 0
	if wl.highlightedActivityID != 0 {
		for i, a := range wl.previewActivities {
			if a.ScheduledID == wl.highlightedActivityID {
				idx = i
				break
			}
		}
	} else {
		wl.highlightedActivityID = wl.previewActivities[0].ScheduledID
	}
	wl.eventTable.SelectRow(idx)
	wl.eventDetail.SetText(formatSelectedActivityDetail(wl.previewActivities[idx]))
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
		events = append(events, temporal.EnhancedHistoryEvent{
			ID:      8,
			Type:    endType,
			Time:    *w.EndTime,
			Details: "status: " + w.Status,
		})
	}
	return events
}
