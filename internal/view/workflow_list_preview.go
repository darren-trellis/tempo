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

func (wl *WorkflowList) showPreviewIO() bool {
	if !wl.previewModeEnabled() {
		return false
	}
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
	input, output := workflowIOFromEvents(wl.previewEvents)
	showWorkflowIO(wl.app, wf.Type, input, output, func() {
		wl.setFocusPane(wl.focusPane)
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
		wl.setFocusPane(focusEvents)
		return
	}
	if wl.app != nil {
		wl.app.NavigateToWorkflowDetail(wf.ID, wf.RunID)
	}
}

func (wl *WorkflowList) paneAt(x, y int) (workflowFocusPane, bool) {
	if wl.previewModeEnabled() {
		if wl.eventDetailPanel != nil && wl.eventDetailPanel.InRect(x, y) {
			return focusEventDetail, true
		}
		if wl.eventsPanel != nil && wl.eventsPanel.InRect(x, y) {
			return focusEvents, true
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
			if pane, ok := wl.paneAt(x, y); ok && pane == focusWorkflows {
				wl.activateSelectedWorkflow()
			}
		}
		return consumed, capture
	})
}

func (wl *WorkflowList) previewModeEnabled() bool {
	if wl.app == nil {
		return false
	}
	return wl.app.Config().ShouldPreviewMode()
}

func (wl *WorkflowList) applyPreviewLayout() {
	on := wl.previewModeEnabled()
	if !on {
		if wl.previewTimer != nil {
			wl.previewTimer.Stop()
		}
		if wl.eventTable != nil {
			wl.clearPreview()
		}
		wl.focusPane = focusWorkflows
	}

	wl.Clear()
	if on {
		wl.AddItem(wl.workflowsPanel, 0, 11, true)
		wl.AddItem(wl.rightFlex, 0, 9, false)
	} else if wl.workflowsPanel != nil {
		wl.AddItem(wl.workflowsPanel, 0, 1, true)
	}

	if on && len(wl.workflows) > 0 && wl.table != nil {
		row := wl.table.SelectedRow()
		if row < 0 || row >= len(wl.workflows) {
			row = 0
		}
		wl.schedulePreview(wl.workflows[row], false)
	}

	if wl.app != nil && wl.app.JigApp() != nil {
		wl.setFocusPane(wl.focusPane)
		return
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) togglePreviewMode() {
	if wl.app == nil {
		return
	}
	cfg := wl.app.Config()
	if cfg == nil {
		return
	}
	on := !cfg.ShouldPreviewMode()
	cfg.SetPreviewMode(on)
	if err := cfg.Save(); err != nil {
		wl.app.ToastError("Failed to save preview mode: " + err.Error())
		return
	}
	wl.applyPreviewLayout()
	if on {
		wl.app.ToastSuccess("Preview mode on")
		return
	}
	wl.app.ToastSuccess("Preview mode off")
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

	wl.eventsPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Events", theme.IconEvent))
	wl.eventsPanel.SetContent(wl.eventTable)

	wl.eventDetailPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Event Details", theme.IconInfo))
	wl.eventDetailPanel.SetContent(wl.eventDetail)

	wl.eventTable.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 && row-1 < len(wl.previewEvents) {
			wl.eventDetail.SetText(formatSelectedEventDetail(wl.previewEvents[row-1]))
			wl.eventDetail.ScrollToBeginning()
		}
	})

	wl.eventTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
		if event.Rune() == 'p' {
			wl.togglePreviewMode()
			return nil
		}
		if event.Rune() == 'i' && wl.showPreviewIO() {
			return nil
		}
		if event.Rune() == 'e' && wl.previewWorkflowID != "" {
			wl.app.NavigateToEvents(wl.previewWorkflowID, wl.previewRunID)
			return nil
		}
		return event
	})

	wl.eventDetail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
		if event.Rune() == 'p' {
			wl.togglePreviewMode()
			return nil
		}
		if event.Rune() == 'i' && wl.showPreviewIO() {
			return nil
		}
		return event
	})
}

func (wl *WorkflowList) cycleFocus(delta int) {
	next := (int(wl.focusPane) + delta) % 3
	if next < 0 {
		next += 3
	}
	wl.setFocusPane(workflowFocusPane(next))
}

func (wl *WorkflowList) setFocusPane(pane workflowFocusPane) {
	wl.focusPane = pane
	if wl.app == nil || wl.app.JigApp() == nil {
		wl.applyFocusStyles()
		return
	}
	switch pane {
	case focusEvents:
		wl.app.JigApp().SetFocus(wl.eventTable)
	case focusEventDetail:
		wl.app.JigApp().SetFocus(wl.eventDetail)
	default:
		wl.app.JigApp().SetFocus(wl.table)
	}
	wl.applyFocusStyles()
	wl.app.JigApp().Menu().SetHints(wl.Hints())
}

func (wl *WorkflowList) applyFocusStyles() {
	if wl.workflowsPanel != nil {
		wl.workflowsPanel.SetFocused(wl.focusPane == focusWorkflows)
	}
	if wl.eventsPanel != nil {
		wl.eventsPanel.SetFocused(wl.focusPane == focusEvents)
	}
	if wl.eventDetailPanel != nil {
		wl.eventDetailPanel.SetFocused(wl.focusPane == focusEventDetail)
	}
	if wl.table != nil {
		wl.table.SetSelectable(wl.focusPane == focusWorkflows, false)
	}
	if wl.eventTable != nil {
		wl.eventTable.SetSelectable(wl.focusPane == focusEvents, false)
	}
}

func (wl *WorkflowList) syncFocusFromPrimitives() {
	pane := focusWorkflows
	switch {
	case wl.eventDetail != nil && wl.eventDetail.HasFocus():
		pane = focusEventDetail
	case wl.eventTable != nil && wl.eventTable.HasFocus():
		pane = focusEvents
	}
	if pane != wl.focusPane {
		wl.focusPane = pane
		if wl.app != nil && wl.app.JigApp() != nil {
			wl.app.JigApp().Menu().SetHints(wl.Hints())
		}
	}
	wl.applyFocusStyles()
}

func (wl *WorkflowList) clearPreview() {
	wl.previewEvents = nil
	wl.previewWorkflowID = ""
	wl.previewRunID = ""
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wl.eventDetail.SetText(fmt.Sprintf("[%s]Select a workflow to load events[-]", theme.TagFgDim()))
	if wl.eventsPanel != nil {
		wl.eventsPanel.SetTitle(fmt.Sprintf("%s Events", theme.IconEvent))
	}
}

func (wl *WorkflowList) setPreviewStatus(title, message string) {
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wl.eventDetail.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), message))
	if wl.eventsPanel != nil {
		wl.eventsPanel.SetTitle(title)
	}
}

func (wl *WorkflowList) schedulePreview(w temporal.Workflow, force bool) {
	if !wl.previewModeEnabled() {
		return
	}
	if !force && wl.previewWorkflowID == w.ID && wl.previewRunID == w.RunID {
		return
	}

	gen := atomic.AddUint64(&wl.previewGen, 1)
	wl.previewWorkflowID = w.ID
	wl.previewRunID = w.RunID
	wl.setPreviewStatus(
		fmt.Sprintf("%s %s", theme.IconEvent, truncate(w.Type, 28)),
		"Loading events...",
	)

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
			wl.setPreviewStatus(
				fmt.Sprintf("%s %s", theme.IconEvent, truncate(w.Type, 28)),
				"Failed to load events: "+err.Error(),
			)
			return
		}
		wl.showPreviewEvents(w, events)
	})
}

func (wl *WorkflowList) showPreviewEvents(w temporal.Workflow, events []temporal.EnhancedHistoryEvent) {
	wl.previewEvents = events
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	if wl.eventsPanel != nil {
		wl.eventsPanel.SetTitle(fmt.Sprintf("%s %s", theme.IconEvent, truncate(w.Type, 28)))
	}

	if len(events) == 0 {
		wl.eventDetail.SetText(fmt.Sprintf("[%s]No events[-]", theme.TagFgDim()))
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
	wl.eventTable.SelectRow(0)
	wl.eventDetail.SetText(formatSelectedEventDetail(events[0]))
}

func mockPreviewEvents(w temporal.Workflow) []temporal.EnhancedHistoryEvent {
	events := []temporal.EnhancedHistoryEvent{
		{
			ID:      1,
			Type:    "WorkflowExecutionStarted",
			Time:    w.StartTime,
			Details: "taskQueue: " + w.TaskQueue,
		},
	}
	if w.EndTime != nil {
		endType := "WorkflowExecutionCompleted"
		if w.Status == "Failed" {
			endType = "WorkflowExecutionFailed"
		}
		events = append(events, temporal.EnhancedHistoryEvent{
			ID:      2,
			Type:    endType,
			Time:    *w.EndTime,
			Details: "status: " + w.Status,
		})
	}
	return events
}
