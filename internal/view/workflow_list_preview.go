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

func (wl *WorkflowList) setupPreview() {
	wl.eventTable = components.NewTable()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wl.eventTable.SetBorder(false)
	wl.eventTable.SetBackgroundColor(theme.Bg())

	wl.eventDetail = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetWordWrap(true)
	wl.eventDetail.SetBackgroundColor(theme.Bg())
	wl.eventDetail.SetTextColor(theme.Fg())

	wl.eventsPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Events", theme.IconEvent))
	wl.eventsPanel.SetContent(wl.eventTable)

	wl.eventDetailPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Event Detail", theme.IconInfo))
	wl.eventDetailPanel.SetContent(wl.eventDetail)

	wl.preview = tview.NewFlex().SetDirection(tview.FlexRow)
	wl.preview.SetBackgroundColor(theme.Bg())
	wl.preview.AddItem(wl.eventsPanel, 0, 3, true)
	wl.preview.AddItem(wl.eventDetailPanel, 0, 2, false)

	wl.eventTable.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 && row-1 < len(wl.previewEvents) {
			wl.eventDetail.SetText(formatSelectedEventDetail(wl.previewEvents[row-1]))
		}
	})

	wl.eventTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyBacktab {
			wl.focusWorkflowTable()
			return nil
		}
		if event.Rune() == 'e' {
			if wl.previewWorkflowID != "" {
				wl.app.NavigateToEvents(wl.previewWorkflowID, wl.previewRunID)
				return nil
			}
		}
		return event
	})

	wl.clearPreview()
}

func (wl *WorkflowList) focusPreview() {
	if !wl.IsDetailVisible() {
		wl.ShowDetail()
	}
	wl.FocusDetail()
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.app.JigApp().SetFocus(wl.eventTable)
		wl.app.JigApp().Menu().SetHints(wl.Hints())
	}
}

func (wl *WorkflowList) focusWorkflowTable() {
	wl.FocusMaster()
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.app.JigApp().SetFocus(wl.table)
		wl.app.JigApp().Menu().SetHints(wl.Hints())
	}
}

func (wl *WorkflowList) clearPreview() {
	wl.previewEvents = nil
	wl.previewWorkflowID = ""
	wl.previewRunID = ""
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wl.eventDetail.SetText(fmt.Sprintf("[%s]Select a workflow to load events[-]", theme.TagFgDim()))
	wl.SetDetailTitle(fmt.Sprintf("%s Events", theme.IconEvent))
}

func (wl *WorkflowList) setPreviewStatus(title, message string) {
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wl.eventDetail.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), message))
	wl.SetDetailTitle(title)
}

func (wl *WorkflowList) schedulePreview(w temporal.Workflow, force bool) {
	if !force && wl.previewWorkflowID == w.ID && wl.previewRunID == w.RunID {
		return
	}

	gen := atomic.AddUint64(&wl.previewGen, 1)
	wl.previewWorkflowID = w.ID
	wl.previewRunID = w.RunID
	wl.setPreviewStatus(
		fmt.Sprintf("%s Events", theme.IconEvent),
		"Loading events...",
	)
	wl.SetDetailTitle(fmt.Sprintf("%s %s", theme.IconEvent, truncate(w.Type, 28)))

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
				fmt.Sprintf("%s Events", theme.IconEvent),
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
	wl.SetDetailTitle(fmt.Sprintf("%s %s", theme.IconEvent, truncate(w.Type, 28)))

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
