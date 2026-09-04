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
)

const (
	startWorkflowFieldCount  = 6
	startWorkflowFieldHeight = 4
	startWorkflowModalHeight = 2 + startWorkflowFieldCount*startWorkflowFieldHeight + (startWorkflowFieldCount - 1)
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
		{Key: "Tab", Description: "Next field"},
		{Key: "Enter", Description: "Execute"},
		{Key: "Esc", Description: "Cancel"},
	}
}

func showStartWorkflowModal(app *App, prefill startWorkflowPrefill) {
	form := components.NewFormBuilder().
		Text("workflowId", "Workflow ID").
		Placeholder("Enter workflow ID").
		Value(prefill.WorkflowID).
		Validate(validators.Required()).
		Done().
		Text("workflowType", "Workflow Type").
		Placeholder("Enter workflow type").
		Value(prefill.WorkflowType).
		Validate(validators.Required()).
		Done().
		Text("taskQueue", "Task Queue").
		Placeholder("Enter task queue").
		Value(prefill.TaskQueue).
		Validate(validators.Required()).
		Done().
		Text("input", "Input (JSON, optional)").
		Placeholder("{}").
		Value(prefill.Input).
		Done().
		Text("signalName", "Signal Name (optional)").
		Placeholder("Leave empty to start without a signal").
		Value(prefill.SignalName).
		Done().
		Text("signalInput", "Signal Input (JSON, optional)").
		Placeholder("{}").
		Value(prefill.SignalInput).
		Done().
		OnSubmit(func(values map[string]any) {
			req := startWorkflowSubmit{
				Namespace:    prefill.Namespace,
				WorkflowID:   stringValue(values, "workflowId"),
				WorkflowType: stringValue(values, "workflowType"),
				TaskQueue:    stringValue(values, "taskQueue"),
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
			app.JigApp().Pages().DismissModal()
			executeStartWorkflow(app, req)
		}).
		OnCancel(func() {
			app.JigApp().Pages().DismissModal()
		}).
		Build()

	modal := newOverlayModal(components.ModalConfig{
		Title:  fmt.Sprintf("%s Start Workflow", theme.IconInfo),
		Width:  70,
		Height: startWorkflowModalHeight,
	}, nil)
	modal.SetContent(form)
	hints := startWorkflowHints()
	modal.SetHints(hints)
	modal.SetOnCancel(func() {
		app.JigApp().Pages().DismissModal()
	})

	app.PushModal(modal)
	if app.JigApp().Menu() != nil {
		app.JigApp().Menu().SetHints(hints)
	}
	app.JigApp().SetFocus(form)
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
			app.NavigateToWorkflowDetail(req.WorkflowID, runID)
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
