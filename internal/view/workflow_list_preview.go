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
	wl.setupActivitiesPane()
	wl.setupActivityDetailPane()
	wl.setupWorkflowIOPane()
	wl.setupHierarchyPane()
	wl.workflowDetail, wl.workflowDetailScroll = newInfoRowsTable(wl.app, func() []workflowInfoRow { return wl.previewDetailRows })
	wl.workflowDetail.SetInputCapture(wl.handlePreviewDetailKeys)

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
	wl.previewTabs.SetInputCapture(wl.handlePreviewKeys)
	wl.applyEventsTabMode()

	wl.previewPanel = components.NewPanel()
	wl.previewPanel.SetContent(wl.previewTabs)

	wl.rightFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	wl.rightFlex.SetBackgroundColor(theme.Bg())
	wl.setupTimeline()
}

// setupActivitiesPane builds the activity table and the event tree that
// shares its tab slot.
func (wl *WorkflowList) setupActivitiesPane() {
	wl.eventTable = components.NewTable()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wl.eventTable.SetBorder(false)
	wl.eventTable.SetBackgroundColor(theme.Bg())
	wl.eventTable.SetEvaluateAllRows(true)
	wl.eventTableScroll = attachTableCharScroll(wl.eventTable, wl.app)
	wl.eventTable.SetSelectionChangedFunc(func(row, col int) {
		wl.updatePreviewSelection(row)
	})
	wl.eventTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if handleTableCharScroll(wl.eventTableScroll, wl.eventTable, event) {
			return nil
		}
		return wl.handlePreviewKeys(event)
	})

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
}

// setupActivityDetailPane builds the Details/Input/Output tabs for the
// selected activity or event.
func (wl *WorkflowList) setupActivityDetailPane() {
	wl.eventDetail = newPreviewTextView(wl.app)
	wl.eventDetailTree = newJSONTreeSelection(wl.eventDetail)
	wl.eventDetail.SetInputCapture(wl.capturePreviewTextView(wl.eventDetail))

	wl.activityDetail, wl.activityDetailScroll = newInfoRowsTable(wl.app, func() []workflowInfoRow { return wl.activityDetailRows })
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
	wl.activityDetailTabs.SetInputCapture(wl.capturePreviewTabs)

	wl.eventDetailPanel = components.NewPanel()
	wl.eventDetailPanel.SetContent(wl.activityDetailTabs)
}

// setupWorkflowIOPane builds the workflow Input/Output tabs.
func (wl *WorkflowList) setupWorkflowIOPane() {
	wl.workflowIOView = newPreviewTextView(wl.app)
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
	wl.workflowIOTabs.SetInputCapture(wl.capturePreviewTabs)
}

func (wl *WorkflowList) setupHierarchyPane() {
	wl.hierarchyView = NewWorkflowGraphView(wl.app, wl.namespace, nil)
	wl.hierarchyView.SetEmbedded(true)
	hierarchyInput := func(event *tcell.EventKey) *tcell.EventKey {
		if wl.hierarchyView.handleGraphKeys(event) {
			return nil
		}
		return wl.handlePreviewKeys(event)
	}
	if wl.hierarchyView.tree != nil {
		wl.hierarchyView.tree.SetBackgroundColor(theme.Bg())
		wl.hierarchyView.tree.SetInputCapture(hierarchyInput)
	}
	if wl.hierarchyView.graph != nil {
		wl.hierarchyView.graph.SetBackgroundColor(theme.Bg())
		wl.hierarchyView.graph.SetInputCapture(hierarchyInput)
	}
	wl.hierarchyGraphPanel = components.NewPanel()
	wl.hierarchyGraphPanel.SetContent(wl.hierarchyView.graph)
}

func newPreviewTextView(app *App) *tview.TextView {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetScrollable(true)
	setTextViewWrap(view, ioWrapOn(app))
	view.SetBackgroundColor(theme.Bg())
	view.SetTextColor(theme.Fg())
	attachTextViewScrollbar(view, app)
	return view
}

// newInfoRowsTable builds a key/value table that scrolls sideways across its
// widest row.
func newInfoRowsTable(app *App, rows func() []workflowInfoRow) (*components.Table, *charScrollView) {
	table := components.NewTable()
	table.SetBorder(false)
	table.SetBackgroundColor(theme.Bg())
	table.SetEvaluateAllRows(true)
	scroll := newCharScrollView(table, func() int {
		return workflowInfoContentWidth(rows())
	}).withApp(app)
	bindTableCharScroll(table, scroll, func() int {
		return mouseScrollStepFromApp(app)
	})
	return table, scroll
}

// capturePreviewTabs lets preview keys win over a tab strip, and keeps jig's
// own tab navigation keys from leaking to the parent.
func (wl *WorkflowList) capturePreviewTabs(event *tcell.EventKey) *tcell.EventKey {
	if handled := wl.handlePreviewKeys(event); handled == nil {
		return nil
	}
	if isJigTabsNavKey(event) {
		return nil
	}
	return event
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
