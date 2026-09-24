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

const (
	startWorkflowFieldCount  = 6
	startWorkflowFieldHeight = 4
	startWorkflowInputHeight = 11
	startWorkflowModalHeight = 2 + (startWorkflowFieldCount-1)*startWorkflowFieldHeight + startWorkflowInputHeight + (startWorkflowFieldCount - 1)
)

// startWorkflowPrefill holds the pre-fill values for the start workflow modal.
type startWorkflowPrefill struct {
	Namespace    string
	WorkflowID   string
	WorkflowType string
	TaskQueue    string
	Input        string
	SignalName   string
	SignalInput  string
}

type startWorkflowSubmit struct {
	Namespace    string
	WorkflowID   string
	WorkflowType string
	TaskQueue    string
	Input        string
	SignalName   string
	SignalInput  string
}

func (r startWorkflowSubmit) signalName() string {
	return strings.TrimSpace(r.SignalName)
}

func (r startWorkflowSubmit) withSignal() bool {
	return r.signalName() != ""
}

func (r startWorkflowSubmit) validate() error {
	if !r.withSignal() && strings.TrimSpace(r.SignalInput) != "" {
		return fmt.Errorf("signal name is required when signal input is set")
	}
	return nil
}

func (r startWorkflowSubmit) namespace(app *App) string {
	if r.Namespace != "" {
		return r.Namespace
	}
	if app != nil {
		return app.CurrentNamespace()
	}
	return ""
}

func startWorkflowHints() []components.KeyHint {
	return []components.KeyHint{
		{Key: "Tab", Description: "Complete / Next"},
		{Key: "Enter", Description: "Execute"},
		{Key: "Esc", Description: "Cancel"},
	}
}

func showStartWorkflowModal(app *App, prefill startWorkflowPrefill) {
	namespace := prefill.Namespace
	if namespace == "" && app != nil {
		namespace = app.CurrentNamespace()
	}
	types, queues := startWorkflowSuggestions(app, namespace)
	typeField := newDropdownField("workflowType", "Workflow Type", types).
		SetPlaceholder("Enter workflow type").
		SetValue(prefill.WorkflowType).
		SetValidator(validators.Required())
	queueField := newDropdownField("taskQueue", "Task Queue", queues).
		SetPlaceholder("Enter task queue").
		SetValue(prefill.TaskQueue).
		SetValidator(validators.Required())
	inputField := components.NewTextArea("input").
		SetLabel("Input (JSON, optional)").
		SetPlaceholder("{}").
		SetValue(prefill.Input)

	form := components.NewFormBuilder().
		Text("workflowId", "Workflow ID").
		Placeholder("Enter workflow ID").
		Value(prefill.WorkflowID).
		Validate(validators.Required()).
		Done().
		AddField(typeField).
		AddField(queueField).
		AddField(inputField).
		Text("signalName", "Signal Name (optional)").
		Placeholder("Leave empty to start without a signal").
		Value(prefill.SignalName).
		Done().
		Text("signalInput", "Signal Input (JSON, optional)").
		Placeholder("{}").
		Value(prefill.SignalInput).
		Done().
		OnSubmit(func(values map[string]any) {
			if err := typeField.Validate(); err != nil {
				if app != nil {
					app.ToastError("Workflow Type: " + err.Error())
				}
				return
			}
			if err := queueField.Validate(); err != nil {
				if app != nil {
					app.ToastError("Task Queue: " + err.Error())
				}
				return
			}
			req := startWorkflowSubmit{
				Namespace:    prefill.Namespace,
				WorkflowID:   stringValue(values, "workflowId"),
				WorkflowType: typeField.GetValue(),
				TaskQueue:    queueField.GetValue(),
				Input:        stringValue(values, "input"),
				SignalName:   stringValue(values, "signalName"),
				SignalInput:  stringValue(values, "signalInput"),
			}
			if err := req.validate(); err != nil {
				if app != nil {
					app.ToastError(err.Error())
				}
				return
			}
			stopWatchingStartCatalog(app, namespace)
			app.JigApp().Pages().DismissModal()
			executeStartWorkflow(app, req)
		}).
		OnCancel(func() {
			if collapseOpenDropdowns(typeField, queueField) {
				return
			}
			stopWatchingStartCatalog(app, namespace)
			app.JigApp().Pages().DismissModal()
		}).
		Build()

	modal := newOverlayModal(components.ModalConfig{
		Title:  fmt.Sprintf("%s Start Workflow", theme.IconInfo),
		Width:  70,
		Height: startWorkflowModalHeight,
	}, nil)
	modal.bindDropdowns(form, typeField, queueField)
	form.SetInputCapture(startWorkflowFormCapture(inputField, typeField, queueField))
	modal.SetContent(form)
	hints := startWorkflowHints()
	modal.SetHints(hints)
	modal.SetOnDismiss(func() bool {
		if collapseOpenDropdowns(typeField, queueField) {
			return false
		}
		stopWatchingStartCatalog(app, namespace)
		return true
	})
	modal.SetOnCancel(func() {
		if collapseOpenDropdowns(typeField, queueField) {
			return
		}
		stopWatchingStartCatalog(app, namespace)
		app.JigApp().Pages().DismissModal()
	})

	app.PushModal(modal)
	app.JigApp().SetFocus(form)
	if app != nil {
		if !app.catalog.has(namespace) {
			app.refreshNamespaceCatalogFor(namespace)
		}
		app.watchStartCatalog(namespace, typeField, queueField)
	}
}

func stopWatchingStartCatalog(app *App, namespace string) {
	if app == nil {
		return
	}
	app.catalog.unlisten(namespace)
}

func startWorkflowFormCapture(input *components.TextArea, fields ...*dropdownField) func(*tcell.EventKey) *tcell.EventKey {
	dropdowns := dropdownFormCapture(fields...)
	return func(event *tcell.EventKey) *tcell.EventKey {
		if event == nil {
			return event
		}
		if event.Key() == tcell.KeyEnter && input != nil && input.HasFocus() {
			if handler := input.InputHandler(); handler != nil {
				handler(event, func(tview.Primitive) {})
			}
			return nil
		}
		return dropdowns(event)
	}
}

func startWorkflowSuggestions(app *App, namespace string) (types, queues []string) {
	types, queues, _ = app.catalogSuggestions(namespace)
	return types, queues
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if v, ok := values[key].(string); ok {
		return v
	}
	return ""
}

func executeStartWorkflow(app *App, req startWorkflowSubmit) {
	provider := app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		namespace := req.namespace(app)
		var runID string
		var err error
		if req.withSignal() {
			signalReq := temporal.SignalWithStartRequest{
				WorkflowID:   req.WorkflowID,
				WorkflowType: req.WorkflowType,
				TaskQueue:    req.TaskQueue,
				SignalName:   req.signalName(),
			}
			if req.SignalInput != "" {
				signalReq.SignalInput = []byte(req.SignalInput)
			}
			if req.Input != "" {
				signalReq.WorkflowInput = []byte(req.Input)
			}
			runID, err = provider.SignalWithStartWorkflow(ctx, namespace, signalReq)
		} else {
			startReq := temporal.StartWorkflowRequest{
				WorkflowID:   req.WorkflowID,
				WorkflowType: req.WorkflowType,
				TaskQueue:    req.TaskQueue,
			}
			if req.Input != "" {
				startReq.Input = []byte(req.Input)
			}
			runID, err = provider.StartWorkflow(ctx, namespace, startReq)
		}

		app.JigApp().QueueUpdateDraw(func() {
			if err != nil {
				title := "Start Workflow Failed"
				if req.withSignal() {
					title = "Signal With Start Failed"
				}
				ShowErrorModal(app.JigApp(), title, err.Error())
				return
			}

			app.ToastSuccess(fmt.Sprintf("Workflow %s started", req.WorkflowID))
			app.OpenWorkflowPreview(req.WorkflowID, runID)
		})
	}()
}

func (wl *WorkflowList) showStartWorkflow() {
	row := wl.table.SelectedRow()

	var prefill startWorkflowPrefill
	if row >= 0 && row < len(wl.workflows) {
		wf := wl.workflows[row]
		prefill = startWorkflowPrefill{
			WorkflowID:   wf.ID,
			WorkflowType: wf.Type,
			TaskQueue:    wf.TaskQueue,
			Input:        wf.Input,
		}
	}

	showStartWorkflowModal(wl.app, prefill)
}
