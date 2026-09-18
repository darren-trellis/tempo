package view

import (
	"context"
	"fmt"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/validators"
	"github.com/rivo/tview"
)

// Selection mode methods

func (wl *WorkflowList) toggleSelectionMode() {
	wl.selectionMode = !wl.selectionMode
	if wl.selectionMode {
		wl.table.SetMultiSelect(true)
		wl.SetMasterTitle(fmt.Sprintf("%s Workflows (Select Mode)", theme.IconWorkflow))
	} else {
		indices := wl.selectedWorkflowIndices()
		wl.table.SetMultiSelect(false)
		wl.table.ClearSelection()
		wl.refreshWorkflowRowStyles(indices...)
		wl.updatePanelTitle()
	}
	wl.refreshSelectHints()
}

func (wl *WorkflowList) toggleRowSelection() {
	if wl == nil || wl.table == nil {
		return
	}
	tableRow, _ := wl.table.GetSelection()
	dataIdx := wl.table.SelectedRow()
	wl.table.ToggleSelection()
	if dataIdx >= 0 && !wl.table.IsRowSelected(tableRow) {
		wl.refreshWorkflowRowStyles(dataIdx)
	}
}

func (wl *WorkflowList) refreshWorkflowRowStyles(indices ...int) {
	if wl == nil || wl.table == nil || len(indices) == 0 {
		return
	}
	now := time.Now()
	for _, i := range indices {
		if i < 0 || i >= len(wl.workflows) {
			continue
		}
		_ = wl.table.UpdateStyledRow(i, wl.styledWorkflowCells(now, wl.workflows[i], i))
	}
}

func (wl *WorkflowList) updateSelectionPreview() {
	count := len(wl.table.GetSelectedRows())
	if count == 0 {
		wl.SetMasterTitle(fmt.Sprintf("%s Workflows (Select Mode)", theme.IconWorkflow))
	} else {
		wl.SetMasterTitle(fmt.Sprintf("%s Workflows (%d selected)", theme.IconWorkflow, count))
	}
	wl.refreshSelectHints()
}

func (wl *WorkflowList) refreshSelectHints() {}

func (wl *WorkflowList) selectedWorkflowIndices() []int {
	selected := wl.table.GetSelectedRows()
	indices := make([]int, 0, len(selected))
	for _, row := range selected {
		idx := row - 1
		if idx < 0 || idx >= len(wl.workflows) {
			continue
		}
		indices = append(indices, idx)
	}
	return indices
}

// Batch operation methods

func (wl *WorkflowList) showBatchCancelConfirm() {
	selected := wl.selectedWorkflowIndices()
	if len(selected) == 0 {
		return
	}

	// Count running workflows
	var runningCount int
	for _, idx := range selected {
		if idx < len(wl.workflows) && wl.workflows[idx].Status == "Running" {
			runningCount++
		}
	}

	form := components.NewFormBuilder().
		Text("reason", "Reason (optional)").
		Value("Batch cancelled via tempo").
		Done().
		OnSubmit(func(values map[string]any) {
			reason := values["reason"].(string)
			wl.closeModal()
			wl.executeBatchCancel(selected, reason)
		}).
		OnCancel(func() {
			wl.closeModal()
		}).
		Build()

	infoText := tview.NewTextView().SetDynamicColors(true)
	infoText.SetBackgroundColor(theme.Bg())
	infoText.SetText(fmt.Sprintf(`[%s]Selected:[-] %d workflow(s)
[%s]Running:[-] %d (will be cancelled)
[%s]Other:[-] %d (will be skipped)`,
		theme.TagFgDim(), len(selected),
		theme.TagAccent(), runningCount,
		theme.TagFgDim(), len(selected)-runningCount))

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(infoText, 4, 0, false).
		AddItem(form, 0, 1, true)
	content.SetBackgroundColor(theme.Bg())

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Cancel %d Workflow(s)", theme.IconWarning, len(selected)),
		Width:    60,
		Height:   14,
		Backdrop: true,
	})
	modal.SetContent(content)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
	})

	wl.app.PushModal(modal)
	wl.app.JigApp().SetFocus(form)
}

func (wl *WorkflowList) executeBatchCancel(indices []int, reason string) {
	provider := wl.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var succeeded, failed int
		for _, idx := range indices {
			if idx >= len(wl.workflows) {
				continue
			}
			wf := wl.workflows[idx]
			if wf.Status != "Running" {
				continue
			}

			err := provider.CancelWorkflow(ctx, wl.namespace, wf.ID, wf.RunID, reason)
			if err != nil {
				failed++
			} else {
				succeeded++
			}
		}

		wl.app.JigApp().QueueUpdateDraw(func() {
			wl.toggleSelectionMode()
			wl.loadData()
			msg := fmt.Sprintf("Cancelled %d workflow(s)", succeeded)
			if failed > 0 {
				msg += fmt.Sprintf(", %d failed", failed)
				wl.app.ToastError(msg)
			} else {
				wl.app.ToastSuccess(msg)
			}
		})
	}()
}

func (wl *WorkflowList) showBatchTerminateConfirm() {
	selected := wl.selectedWorkflowIndices()
	if len(selected) == 0 {
		return
	}

	var runningCount int
	for _, idx := range selected {
		if idx < len(wl.workflows) && wl.workflows[idx].Status == "Running" {
			runningCount++
		}
	}
	if runningCount == 0 {
		if wl.app != nil {
			status := ""
			if idx := selected[0]; idx < len(wl.workflows) {
				status = wl.workflows[idx].Status
			}
			wl.app.ToastError(terminateUnavailableMessage(status))
		}
		return
	}

	form := components.NewFormBuilder().
		Text("reason", "Reason (required)").
		Placeholder("Enter reason for termination").
		Validate(validators.Required()).
		Done().
		OnSubmit(func(values map[string]any) {
			reason := values["reason"].(string)
			wl.closeModal()
			wl.executeBatchTerminate(selected, reason)
		}).
		OnCancel(func() {
			wl.closeModal()
		}).
		Build()

	warningText := tview.NewTextView().SetDynamicColors(true)
	warningText.SetBackgroundColor(theme.Bg())
	warningText.SetText(fmt.Sprintf(`[%s]⚠ WARNING: This action cannot be undone![-]

[%s]Selected:[-] %d workflow(s)
[%s]Running:[-] %d (will be terminated)
[%s]Other:[-] %d (will be skipped)`,
		theme.TagError(),
		theme.TagFgDim(), len(selected),
		theme.TagAccent(), runningCount,
		theme.TagFgDim(), len(selected)-runningCount))

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(warningText, 5, 0, false).
		AddItem(form, 0, 1, true)
	content.SetBackgroundColor(theme.Bg())

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Terminate %d Workflow(s)", theme.IconError, len(selected)),
		Width:    65,
		Height:   16,
		Backdrop: true,
	})
	modal.SetContent(content)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Terminate"},
		{Key: "Esc", Description: "Cancel"},
	})

	wl.app.PushModal(modal)
	wl.app.JigApp().SetFocus(form)
}

func (wl *WorkflowList) executeBatchTerminate(indices []int, reason string) {
	provider := wl.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var succeeded, failed int
		for _, idx := range indices {
			if idx >= len(wl.workflows) {
				continue
			}
			wf := wl.workflows[idx]
			if wf.Status != "Running" {
				continue
			}

			err := provider.TerminateWorkflow(ctx, wl.namespace, wf.ID, wf.RunID, reason)
			if err != nil {
				failed++
			} else {
				succeeded++
			}
		}

		wl.app.JigApp().QueueUpdateDraw(func() {
			wl.toggleSelectionMode()
			wl.loadData()
			msg := fmt.Sprintf("Terminated %d workflow(s)", succeeded)
			if failed > 0 {
				msg += fmt.Sprintf(", %d failed", failed)
				wl.app.ToastError(msg)
			} else {
				wl.app.ToastSuccess(msg)
			}
		})
	}()
}

func (wl *WorkflowList) showBatchDeleteConfirm() {
	selected := wl.selectedWorkflowIndices()
	if len(selected) == 0 {
		return
	}

	var form *components.Form
	submit := func() {
		if form == nil {
			return
		}
		confirm, _ := form.GetValues()["confirm"].(string)
		if confirm != "delete" {
			return
		}
		wl.closeModal()
		wl.executeBatchDelete(selected)
	}

	form = components.NewFormBuilder().
		Text("confirm", "Type delete to confirm").
		Placeholder("delete").
		Validate(validators.Custom(func(value any) error {
			if s, ok := value.(string); ok && s == "delete" {
				return nil
			}
			return fmt.Errorf("must type delete")
		})).
		Done().
		OnSubmit(func(values map[string]any) {
			submit()
		}).
		OnCancel(func() {
			wl.closeModal()
		}).
		Build()

	warningText := tview.NewTextView().SetDynamicColors(true)
	warningText.SetBackgroundColor(theme.Bg())
	warningText.SetText(fmt.Sprintf(`[%s]⚠ WARNING: This permanently deletes the workflow and its history.
This action cannot be undone.[-]

[%s]Selected:[-] %d workflow(s)`,
		theme.TagError(),
		theme.TagFgDim(), len(selected)))

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(warningText, 5, 0, false).
		AddItem(form, 0, 1, true)
	content.SetBackgroundColor(theme.Bg())

	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Delete %d Workflow(s)", theme.IconError, len(selected)),
		Width:    65,
		Height:   16,
		Backdrop: true,
	})
	modal.SetContent(content)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Delete"},
		{Key: "Esc", Description: "Cancel"},
	})
	modal.SetOnSubmit(submit)
	modal.SetOnCancel(func() {
		wl.closeModal()
	})

	wl.app.PushModal(modal)
	if jig := wl.app.JigApp(); jig != nil {
		jig.SetFocus(form)
	}
}

func (wl *WorkflowList) executeBatchDelete(indices []int) {
	provider := wl.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var succeeded, failed int
		for _, idx := range indices {
			if idx >= len(wl.workflows) {
				continue
			}
			wf := wl.workflows[idx]
			err := provider.DeleteWorkflow(ctx, wl.namespace, wf.ID, wf.RunID)
			if err != nil {
				failed++
			} else {
				succeeded++
			}
		}

		wl.app.JigApp().QueueUpdateDraw(func() {
			wl.toggleSelectionMode()
			wl.loadData()
			msg := fmt.Sprintf("Deleted %d workflow(s)", succeeded)
			if failed > 0 {
				msg += fmt.Sprintf(", %d failed", failed)
				wl.app.ToastError(msg)
			} else {
				wl.app.ToastSuccess(msg)
			}
		})
	}()
}

func (wl *WorkflowList) closeModal() {
	wl.app.JigApp().Pages().DismissModal()
}
