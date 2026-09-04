package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/validators"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type workflowActionTarget struct {
	Namespace string
	ID        string
	RunID     string
	Status    string
}

func workflowIsRunning(status string) bool {
	return status == "Running"
}

func workflowCanReset(status string) bool {
	switch status {
	case "Completed", "Failed", "Terminated", "Canceled":
		return true
	}
	return false
}

func dismissAppModal(app *App) {
	if app != nil && app.JigApp() != nil && app.JigApp().Pages() != nil {
		app.JigApp().Pages().DismissModal()
	}
}

func actionTargetNamespace(app *App, t workflowActionTarget) string {
	if t.Namespace != "" {
		return t.Namespace
	}
	if app != nil {
		return app.CurrentNamespace()
	}
	return ""
}

func showActionError(app *App, title string, err error) {
	if app == nil || app.JigApp() == nil || err == nil {
		return
	}
	ShowErrorModal(app.JigApp(), title, err.Error())
}

func eventMatchesSearch(ev temporal.EnhancedHistoryEvent, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(ev.Type), q) ||
		strings.Contains(strings.ToLower(ev.ActivityType), q) ||
		strings.Contains(strings.ToLower(ev.TimerID), q) ||
		strings.Contains(strings.ToLower(ev.ChildWorkflowType), q) ||
		strings.Contains(strings.ToLower(ev.Failure), q) ||
		strings.Contains(strings.ToLower(ev.FailureSource), q) ||
		strings.Contains(strings.ToLower(ev.FailureStackTrace), q) ||
		strings.Contains(strings.ToLower(ev.FailureCause), q) ||
		strings.Contains(strings.ToLower(ev.Details), q)
}

func filterHistoryEvents(events []temporal.EnhancedHistoryEvent, query string) []temporal.EnhancedHistoryEvent {
	if query == "" {
		return events
	}
	out := make([]temporal.EnhancedHistoryEvent, 0, len(events))
	for _, ev := range events {
		if eventMatchesSearch(ev, query) {
			out = append(out, ev)
		}
	}
	return out
}

func showCancelWorkflowModal(app *App, t workflowActionTarget, onDone func()) {
	form := components.NewFormBuilder().
		Text("reason", "Reason (optional)").
		Value("Cancelled via tempo").
		Done().
		OnSubmit(func(values map[string]any) {
			dismissAppModal(app)
			executeCancelWorkflow(app, t, stringValue(values, "reason"), onDone)
		}).
		OnCancel(func() {
			dismissAppModal(app)
		}).
		Build()

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Cancel Workflow", theme.IconWarning),
		Width:    60,
		Height:   12,
		Backdrop: true,
	})
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Ctrl+S", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
	})
	app.PushModal(modal)
	app.JigApp().SetFocus(form)
}

func executeCancelWorkflow(app *App, t workflowActionTarget, reason string, onDone func()) {
	provider := app.Provider()
	if provider == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := provider.CancelWorkflow(ctx, actionTargetNamespace(app, t), t.ID, t.RunID, reason)
		app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				showActionError(app, "Cancel Failed", err)
				return
			}
			if onDone != nil {
				onDone()
			}
		})
	}()
}

func showTerminateWorkflowModal(app *App, t workflowActionTarget, onDone func()) {
	form := components.NewFormBuilder().
		Text("reason", "Reason (required)").
		Value("Terminated via tempo").
		Validate(validators.Required()).
		Done().
		OnSubmit(func(values map[string]any) {
			dismissAppModal(app)
			executeTerminateWorkflow(app, t, stringValue(values, "reason"), onDone)
		}).
		OnCancel(func() {
			dismissAppModal(app)
		}).
		Build()

	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.SetBackgroundColor(theme.Bg())
	warningText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	warningText.SetBackgroundColor(theme.Bg())
	warningText.SetText(fmt.Sprintf("[%s]Warning: Termination is immediate and irreversible.\nNo cleanup code will run in the workflow.[-]", theme.TagError()))
	contentFlex.AddItem(warningText, 3, 0, false)
	contentFlex.AddItem(form, 0, 1, true)

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Terminate Workflow", theme.IconError),
		Width:    65,
		Height:   14,
		Backdrop: true,
	})
	modal.SetContent(contentFlex)
	modal.SetHints([]components.KeyHint{
		{Key: "Ctrl+S", Description: "Terminate"},
		{Key: "Esc", Description: "Cancel"},
	})
	app.PushModal(modal)
	app.JigApp().SetFocus(form)
}

func executeTerminateWorkflow(app *App, t workflowActionTarget, reason string, onDone func()) {
	provider := app.Provider()
	if provider == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := provider.TerminateWorkflow(ctx, actionTargetNamespace(app, t), t.ID, t.RunID, reason)
		app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				showActionError(app, "Terminate Failed", err)
				return
			}
			if onDone != nil {
				onDone()
			}
		})
	}()
}

func showDeleteWorkflowModal(app *App, t workflowActionTarget, onDeleted func()) {
	workflowID := t.ID
	form := components.NewFormBuilder().
		Text("confirm", "Type workflow ID to confirm").
		Placeholder(workflowID).
		Validate(validators.Custom(func(value any) error {
			if s, ok := value.(string); ok && s != workflowID {
				return fmt.Errorf("must match workflow ID")
			}
			return nil
		})).
		Done().
		OnSubmit(func(values map[string]any) {
			if stringValue(values, "confirm") != workflowID {
				return
			}
			dismissAppModal(app)
			executeDeleteWorkflow(app, t, onDeleted)
		}).
		OnCancel(func() {
			dismissAppModal(app)
		}).
		Build()

	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.SetBackgroundColor(theme.Bg())
	warningText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	warningText.SetBackgroundColor(theme.Bg())
	warningText.SetText(fmt.Sprintf(`[%s]Warning: This will permanently delete the workflow and its history.
This action cannot be undone.[-]

[%s]Workflow ID:[-] [%s]%s[-]`,
		theme.TagError(),
		theme.TagFgDim(), theme.TagFg(), workflowID))
	contentFlex.AddItem(warningText, 5, 0, false)
	contentFlex.AddItem(form, 0, 1, true)

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Delete Workflow", theme.IconError),
		Width:    70,
		Height:   16,
		Backdrop: true,
	})
	modal.SetContent(contentFlex)
	modal.SetHints([]components.KeyHint{
		{Key: "Ctrl+S", Description: "Delete"},
		{Key: "Esc", Description: "Cancel"},
	})
	app.PushModal(modal)
	app.JigApp().SetFocus(form)
}

func executeDeleteWorkflow(app *App, t workflowActionTarget, onDeleted func()) {
	provider := app.Provider()
	if provider == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := provider.DeleteWorkflow(ctx, actionTargetNamespace(app, t), t.ID, t.RunID)
		app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				showActionError(app, "Delete Failed", err)
				return
			}
			if onDeleted != nil {
				onDeleted()
			}
		})
	}()
}

func showSignalWorkflowModal(app *App, t workflowActionTarget, onDone func()) {
	form := components.NewFormBuilder().
		Text("signalName", "Signal Name").
		Placeholder("Enter signal name").
		Validate(validators.Required()).
		Done().
		Text("input", "Input (JSON, optional)").
		Placeholder("{}").
		Done().
		OnSubmit(func(values map[string]any) {
			dismissAppModal(app)
			executeSignalWorkflow(app, t, stringValue(values, "signalName"), stringValue(values, "input"), onDone)
		}).
		OnCancel(func() {
			dismissAppModal(app)
		}).
		Build()

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Signal Workflow", theme.IconSignal),
		Width:    70,
		Height:   16,
		Backdrop: true,
	})
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Tab", Description: "Next field"},
		{Key: "Ctrl+S", Description: "Send signal"},
		{Key: "Esc", Description: "Cancel"},
	})
	app.PushModal(modal)
	app.JigApp().SetFocus(form)
}

func executeSignalWorkflow(app *App, t workflowActionTarget, signalName, input string, onDone func()) {
	provider := app.Provider()
	if provider == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var inputBytes []byte
		if input != "" {
			inputBytes = []byte(input)
		}
		err := provider.SignalWorkflow(ctx, actionTargetNamespace(app, t), t.ID, t.RunID, signalName, inputBytes)
		app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				showActionError(app, "Signal Failed", err)
				return
			}
			if onDone != nil {
				onDone()
			}
		})
	}()
}

func showQueryWorkflowModal(app *App, t workflowActionTarget) {
	form := components.NewFormBuilder().
		Select("queryType", "Query Type", []string{"__stack_trace", "custom"}).
		Done().
		Text("customQuery", "Custom Query Name").
		Placeholder("Enter custom query name").
		Done().
		Text("args", "Arguments (JSON, optional)").
		Placeholder("{}").
		Done().
		OnSubmit(func(values map[string]any) {
			queryType := stringValue(values, "queryType")
			if queryType == "custom" {
				queryType = stringValue(values, "customQuery")
			}
			if queryType == "" {
				return
			}
			dismissAppModal(app)
			executeQueryWorkflow(app, t, queryType, stringValue(values, "args"))
		}).
		OnCancel(func() {
			dismissAppModal(app)
		}).
		Build()

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Query Workflow", theme.IconInfo),
		Width:    70,
		Height:   18,
		Backdrop: true,
	})
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Tab", Description: "Next field"},
		{Key: "Ctrl+S", Description: "Execute query"},
		{Key: "Esc", Description: "Cancel"},
	})
	app.PushModal(modal)
	app.JigApp().SetFocus(form)
}

func executeQueryWorkflow(app *App, t workflowActionTarget, queryType, args string) {
	provider := app.Provider()
	if provider == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var argsBytes []byte
		if args != "" {
			argsBytes = []byte(args)
		}
		result, err := provider.QueryWorkflow(ctx, actionTargetNamespace(app, t), t.ID, t.RunID, queryType, argsBytes)
		app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				showQueryErrorModal(app, queryType, err.Error())
				return
			}
			showQueryResultModal(app, queryType, result.Result)
		})
	}()
}

func showQueryResultModal(app *App, queryType, result string) {
	modal := newModal(components.ModalConfig{
		Title:     fmt.Sprintf("%s Query Result: %s", theme.IconInfo, queryType),
		Width:     0,
		Height:    0,
		MinWidth:  80,
		MinHeight: 20,
		Backdrop:  true,
	})
	resultView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true)
	resultView.SetBackgroundColor(theme.Bg())
	resultView.SetTextColor(theme.Fg())
	resultView.SetText(highlightFormattedJSONWorkflow(formatJSONPretty(result)))

	panel := components.NewPanel().SetTitle("Result")
	panel.SetContent(resultView)
	resultView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			dismissAppModal(app)
			return nil
		}
		if handleTextViewScroll(resultView, event) {
			return nil
		}
		if event.Rune() == 'y' {
			if err := copyToClipboard(result); err != nil {
				app.ToastError("Failed to copy: " + err.Error())
				return nil
			}
			app.ToastSuccess("Copied query result")
			return nil
		}
		return event
	})
	modal.SetContent(panel)
	modal.SetHints([]components.KeyHint{
		{Key: "y", Description: "Copy"},
		{Key: "Esc", Description: "Close"},
	})
	modal.SetOnCancel(func() {
		dismissAppModal(app)
	})
	app.PushModal(modal)
	app.JigApp().SetFocus(resultView)
}

func showQueryErrorModal(app *App, queryType, errMsg string) {
	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Query Failed: %s", theme.IconError, queryType),
		Width:    60,
		Height:   10,
		Backdrop: true,
	})
	errorText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	errorText.SetBackgroundColor(theme.Bg())
	errorText.SetText(fmt.Sprintf("[%s]Error executing query:[-]\n\n[%s]%s[-]",
		theme.TagError(), theme.TagFg(), errMsg))
	modal.SetContent(errorText)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter/Esc", Description: "Close"},
	})
	modal.SetOnSubmit(func() { dismissAppModal(app) })
	modal.SetOnCancel(func() { dismissAppModal(app) })
	app.PushModal(modal)
}

func showResetWorkflowModal(app *App, t workflowActionTarget, onDone func(newRunID string)) {
	provider := app.Provider()
	if provider == nil {
		return
	}
	loadingModal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Loading Reset Points...", theme.IconInfo),
		Width:    40,
		Height:   5,
		Backdrop: true,
	})
	loadingText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	loadingText.SetBackgroundColor(theme.Bg())
	loadingText.SetText(fmt.Sprintf("[%s]Fetching reset points...[-]", theme.TagFgDim()))
	loadingModal.SetContent(loadingText)
	app.PushModal(loadingModal)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resetPoints, err := provider.GetResetPoints(ctx, actionTargetNamespace(app, t), t.ID, t.RunID)
		app.JigApp().QueueUpdateDraw(func() {
			dismissAppModal(app)
			if err != nil {
				showActionError(app, "Reset Failed", err)
				return
			}
			if len(resetPoints) == 0 {
				showResetErrorModal(app, "No valid reset points found for this workflow.")
				return
			}
			showResetPickerModal(app, t, resetPoints, onDone)
		})
	}()
}

func showResetPickerModal(app *App, t workflowActionTarget, resetPoints []temporal.ResetPoint, onDone func(string)) {
	modal := newModal(components.ModalConfig{
		Title:     fmt.Sprintf("%s Select Reset Point", theme.IconInfo),
		Width:     90,
		Height:    20,
		MinHeight: 15,
		Backdrop:  true,
	})
	table := components.NewTable()
	table.SetHeaders("EVENT ID", "TYPE", "TIME", "DESCRIPTION")
	table.SetBackgroundColor(theme.Bg())
	for _, rp := range resetPoints {
		table.AddRow(
			fmt.Sprintf("%d", rp.EventID),
			truncateStr(rp.EventType, 25),
			rp.Timestamp.Format("15:04:05"),
			truncateStr(rp.Description, 35),
		)
	}
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			row := table.SelectedRow()
			if row >= 0 && row < len(resetPoints) {
				dismissAppModal(app)
				showResetConfirmModal(app, t, resetPoints[row], onDone)
			}
			return nil
		case tcell.KeyEscape:
			dismissAppModal(app)
			return nil
		}
		return event
	})
	modal.SetContent(table)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Select"},
		{Key: "Esc", Description: "Cancel"},
	})
	modal.SetOnCancel(func() { dismissAppModal(app) })
	app.PushModal(modal)
	app.JigApp().SetFocus(table)
}

func showResetConfirmModal(app *App, t workflowActionTarget, resetPoint temporal.ResetPoint, onDone func(string)) {
	eventID := resetPoint.EventID
	form := components.NewFormBuilder().
		Text("reason", "Reason").
		Value("Reset via tempo").
		Done().
		OnSubmit(func(values map[string]any) {
			dismissAppModal(app)
			executeResetWorkflow(app, t, eventID, stringValue(values, "reason"), onDone)
		}).
		OnCancel(func() {
			dismissAppModal(app)
		}).
		Build()

	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.SetBackgroundColor(theme.Bg())
	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	infoText.SetBackgroundColor(theme.Bg())
	infoText.SetText(fmt.Sprintf(`[%s]Reset workflow to event:[-]

[%s]Event ID:[-]    [%s]%d[-]
[%s]Type:[-]        [%s]%s[-]
[%s]Time:[-]        [%s]%s[-]
[%s]Description:[-] [%s]%s[-]`,
		theme.TagAccent(),
		theme.TagFgDim(), theme.TagFg(), resetPoint.EventID,
		theme.TagFgDim(), theme.TagFg(), resetPoint.EventType,
		theme.TagFgDim(), theme.TagFg(), resetPoint.Timestamp.Format("2006-01-02 15:04:05"),
		theme.TagFgDim(), theme.TagFg(), resetPoint.Description))
	contentFlex.AddItem(infoText, 7, 0, false)
	contentFlex.AddItem(form, 0, 1, true)

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Confirm Reset", theme.IconWarning),
		Width:    70,
		Height:   16,
		Backdrop: true,
	})
	modal.SetContent(contentFlex)
	modal.SetHints([]components.KeyHint{
		{Key: "Ctrl+S", Description: "Reset"},
		{Key: "Esc", Description: "Cancel"},
	})
	app.PushModal(modal)
	app.JigApp().SetFocus(form)
}

func executeResetWorkflow(app *App, t workflowActionTarget, eventID int64, reason string, onDone func(string)) {
	provider := app.Provider()
	if provider == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		newRunID, err := provider.ResetWorkflow(ctx, actionTargetNamespace(app, t), t.ID, t.RunID, eventID, reason)
		app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				showActionError(app, "Reset Failed", err)
				return
			}
			if onDone != nil {
				onDone(newRunID)
			}
		})
	}()
}

func showResetErrorModal(app *App, message string) {
	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Reset Error", theme.IconError),
		Width:    50,
		Height:   8,
		Backdrop: true,
	})
	errorText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	errorText.SetBackgroundColor(theme.Bg())
	errorText.SetText(fmt.Sprintf("[%s]%s[-]", theme.TagError(), message))
	modal.SetContent(errorText)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter/Esc", Description: "Close"},
	})
	modal.SetOnSubmit(func() { dismissAppModal(app) })
	modal.SetOnCancel(func() { dismissAppModal(app) })
	app.PushModal(modal)
}
