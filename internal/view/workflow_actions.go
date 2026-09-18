package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/validators"
	"github.com/galaxy-io/tempo/internal/config"
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

func terminateUnavailableMessage(status string) string {
	if status == "" {
		return "Cannot terminate this workflow"
	}
	return "Cannot terminate a " + strings.ToLower(status) + " workflow"
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

func activityMatchesSearch(a previewActivity, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(a.Type), q) ||
		strings.Contains(strings.ToLower(a.Status), q) ||
		strings.Contains(strings.ToLower(a.ActivityID), q) ||
		strings.Contains(strings.ToLower(a.TaskQueue), q) ||
		strings.Contains(strings.ToLower(a.Identity), q) ||
		strings.Contains(strings.ToLower(a.Input), q) ||
		strings.Contains(strings.ToLower(a.Result), q) ||
		strings.Contains(strings.ToLower(a.Failure), q)
}

func filterPreviewActivities(activities []previewActivity, query string) []previewActivity {
	if query == "" {
		return activities
	}
	out := make([]previewActivity, 0, len(activities))
	for _, a := range activities {
		if activityMatchesSearch(a, query) {
			out = append(out, a)
		}
	}
	return out
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
		{Key: "Enter", Description: "Confirm"},
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
	if !workflowIsRunning(t.Status) {
		if app != nil {
			app.ToastError(terminateUnavailableMessage(t.Status))
		}
		return
	}
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
		{Key: "Enter", Description: "Terminate"},
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
		{Key: "Enter", Description: "Delete"},
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
		{Key: "Enter", Description: "Send signal"},
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
	queryTypes := []string{"__stack_trace", "custom"}
	queryField := newOrderedDropdownField("queryType", "Query Type", queryTypes).
		SetValue(queryTypes[0])
	form := components.NewFormBuilder().
		AddField(queryField).
		Text("customQuery", "Custom Query Name").
		Placeholder("Enter custom query name").
		Done().
		Text("args", "Arguments (JSON, optional)").
		Placeholder("{}").
		Done().
		OnSubmit(func(values map[string]any) {
			queryType := queryField.GetValue()
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
			if collapseOpenDropdowns(queryField) {
				return
			}
			dismissAppModal(app)
		}).
		Build()

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Query Workflow", theme.IconInfo),
		Width:    70,
		Height:   22,
		Backdrop: true,
	})
	modal.bindDropdowns(form, queryField)
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Tab", Description: "Next field"},
		{Key: "Enter", Description: "Execute query"},
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
			showResetFormModal(app, t, resetPoints, onDone)
		})
	}()
}

func formatResetPointLabel(rp temporal.ResetPoint) string {
	desc := rp.Description
	if desc == "" {
		desc = rp.EventType
	}
	return fmt.Sprintf("#%d  %s  %s", rp.EventID, rp.Timestamp.Format("15:04:05"), desc)
}

func resetPointLabels(points []temporal.ResetPoint) []string {
	labels := make([]string, len(points))
	for i, rp := range points {
		labels[i] = formatResetPointLabel(rp)
	}
	return labels
}

func resetPointIndex(labels []string, value string) int {
	for i, label := range labels {
		if label == value {
			return i
		}
	}
	return -1
}

func resetFormDefaults(app *App, n int) (selected int, reason string) {
	reason = config.DefaultResetReason
	if n < 1 {
		return 0, reason
	}
	selected = 0
	if app == nil || app.Config() == nil {
		return selected, reason
	}
	cfg := app.Config()
	if cfg.ResetPointDefault() == config.ResetPointLast {
		selected = n - 1
	}
	return selected, cfg.ResetReasonDefault()
}

func showResetFormModal(app *App, t workflowActionTarget, resetPoints []temporal.ResetPoint, onDone func(string)) {
	labels := resetPointLabels(resetPoints)
	selected, reason := resetFormDefaults(app, len(labels))
	pointField := newOrderedDropdownField("point", "Reset Point", labels).
		SetPlaceholder("Select reset point")
	if selected >= 0 && selected < len(labels) {
		pointField.SetValue(labels[selected])
	}
	form := components.NewFormBuilder().
		AddField(pointField).
		Text("reason", "Reason").
		Value(reason).
		Done().
		OnSubmit(func(values map[string]any) {
			idx := resetPointIndex(labels, pointField.GetValue())
			if idx < 0 || idx >= len(resetPoints) {
				return
			}
			dismissAppModal(app)
			executeResetWorkflow(app, t, resetPoints[idx].EventID, stringValue(values, "reason"), onDone)
		}).
		OnCancel(func() {
			if collapseOpenDropdowns(pointField) {
				return
			}
			dismissAppModal(app)
		}).
		Build()

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Reset Workflow", theme.IconWarning),
		Width:    80,
		Height:   22,
		Backdrop: true,
	})
	modal.bindDropdowns(form, pointField)
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Tab", Description: "Next field"},
		{Key: "Enter", Description: "Reset"},
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
