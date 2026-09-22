package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func workflowIOFromEvents(events []temporal.EnhancedHistoryEvent) (input, output string) {
	for _, event := range events {
		switch {
		case strings.Contains(event.Type, "WorkflowExecutionStarted"):
			if event.Input != "" {
				input = event.Input
			}
		case strings.Contains(event.Type, "WorkflowExecutionCompleted"):
			if event.Result != "" {
				output = event.Result
			}
		case strings.Contains(event.Type, "WorkflowExecutionFailed"),
			strings.Contains(event.Type, "WorkflowExecutionTerminated"),
			strings.Contains(event.Type, "WorkflowExecutionTimedOut"):
			if event.Failure != "" {
				output = event.Failure
			}
		case strings.Contains(event.Type, "WorkflowExecutionCanceled"):
			if event.Result != "" {
				output = event.Result
			}
		}
	}
	return input, output
}

// workflowRunning reports whether a workflow may still produce more history.
func workflowRunning(w temporal.Workflow) bool {
	return w.Status == "" || w.Status == "Running"
}

// workflowHistoryComplete reports whether these events carry the workflow's
// terminal event, i.e. whether its output can be read off them at all.
func workflowHistoryComplete(events []temporal.EnhancedHistoryEvent) bool {
	for _, event := range events {
		switch {
		case strings.Contains(event.Type, "ChildWorkflowExecution"):
			continue
		case strings.Contains(event.Type, "WorkflowExecutionCompleted"),
			strings.Contains(event.Type, "WorkflowExecutionFailed"),
			strings.Contains(event.Type, "WorkflowExecutionCanceled"),
			strings.Contains(event.Type, "WorkflowExecutionTerminated"),
			strings.Contains(event.Type, "WorkflowExecutionTimedOut"),
			strings.Contains(event.Type, "WorkflowExecutionContinuedAsNew"):
			return true
		}
	}
	return false
}

func ioTreeEnabled(app *App) bool {
	if app == nil {
		return false
	}
	return app.Config().ShouldShowIOTree()
}

func formatIOContent(label, content string, tree bool) string {
	if content == "" {
		return fmt.Sprintf("[%s]No %s[-]", theme.TagFgDim(), strings.ToLower(label))
	}
	if tree {
		return formatJSONTree(content)
	}
	return highlightFormattedJSONWorkflow(formatJSONPretty(content))
}

func workflowIOHints(maximized, tree bool) []components.KeyHint {
	maxHint := "Maximize"
	if maximized {
		maxHint = "Minimize"
	}
	hints := []components.KeyHint{
		{Key: "m", Description: maxHint},
		{Key: "w", Description: "Wrap"},
		{Key: "e", Description: "Editor"},
	}
	if tree {
		hints = append(hints,
			components.KeyHint{Key: "space", Description: "Fold"},
			components.KeyHint{Key: "y", Description: "Yank Row"},
			components.KeyHint{Key: "Y", Description: "Yank All"},
		)
	} else {
		hints = append(hints, components.KeyHint{Key: "y", Description: "Copy"})
	}
	return append(hints, components.KeyHint{Key: "esc", Description: "Close"})
}

func showWorkflowIO(app *App, background tview.Primitive, workflowType, input, output string, onClose func()) {
	if app == nil || app.JigApp() == nil {
		return
	}

	closeModal := func() {
		app.JigApp().Pages().DismissModal()
		if onClose != nil {
			onClose()
		}
	}

	modal := newOverlayModal(components.ModalConfig{
		Title:     fmt.Sprintf("%s Input/Output: %s", theme.IconWorkflow, truncateStr(workflowType, 30)),
		Width:     0,
		Height:    0,
		MinWidth:  120,
		MinHeight: 35,
	}, background)
	modal.frameless = true

	inputView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	inputView.SetBackgroundColor(theme.Bg())
	inputView.SetTextColor(theme.Fg())
	attachTextViewScrollbar(inputView, app)

	outputView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	outputView.SetBackgroundColor(theme.Bg())
	outputView.SetTextColor(theme.Fg())
	attachTextViewScrollbar(outputView, app)
	treeEnabled := ioTreeEnabled(app)
	inputTree := newJSONTreeSelection(inputView)
	outputTree := newJSONTreeSelection(outputView)
	if !inputTree.setContent(input, treeEnabled) {
		inputView.SetText(formatIOContent("Input", input, treeEnabled))
	}
	if !outputTree.setContent(output, treeEnabled) {
		outputView.SetText(formatIOContent("Output", output, treeEnabled))
	}

	inputPanel := components.NewPanel().SetTitle(fmt.Sprintf("%s Input", theme.IconArrowRight))
	inputPanel.SetContent(inputView)
	outputPanel := components.NewPanel().SetTitle(fmt.Sprintf("%s Output", theme.IconArrowLeft))
	outputPanel.SetContent(outputView)

	flex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(inputPanel, 0, 1, true).
		AddItem(outputPanel, 0, 1, false)
	flex.SetBackgroundColor(theme.Bg())

	modal.SetContent(flex)
	applyIOHints := func() {
		hints := workflowIOHints(modal.maximized, treeEnabled)
		modal.SetHints(hints)
		app.syncModalHints(modal)
	}
	applyIOHints()
	modal.SetOnCancel(closeModal)

	focusedInput := true
	wrapped := false
	applyIOFocus := func() {
		inputPanel.SetTitle(fmt.Sprintf("%s Input", theme.IconArrowRight))
		outputPanel.SetTitle(fmt.Sprintf("%s Output", theme.IconArrowLeft))
		inputPanel.SetTitleColor(0)
		outputPanel.SetTitleColor(0)
		inputPanel.SetFocused(focusedInput)
		outputPanel.SetFocused(!focusedInput)
	}
	applyIOFocus()

	switchFocus := func() {
		focusedInput = !focusedInput
		applyIOFocus()
		if focusedInput {
			app.JigApp().SetFocus(inputView)
		} else {
			app.JigApp().SetFocus(outputView)
		}
	}

	inputHandler := func(event *tcell.EventKey) *tcell.EventKey {
		if outputView.HasFocus() {
			focusedInput = false
		} else if inputView.HasFocus() {
			focusedInput = true
		}
		applyIOFocus()

		switch event.Key() {
		case tcell.KeyEscape:
			closeModal()
			return nil
		case tcell.KeyTab, tcell.KeyBacktab:
			switchFocus()
			return nil
		}
		view := outputView
		tree := outputTree
		if focusedInput {
			view = inputView
			tree = inputTree
		}
		if tree.handleKey(event) {
			return nil
		}
		if handleTextViewScroll(view, event) {
			return nil
		}
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'm':
				modal.toggleMaximize()
				applyIOHints()
				return nil
			case 'w':
				wrapped = !wrapped
				setTextViewWrap(inputView, wrapped)
				setTextViewWrap(outputView, wrapped)
				app.ToastInfo(wrapToggleMessage(wrapped))
				return nil
			case 'e':
				if focusedInput {
					openInEditor(app, "input", input)
				} else {
					openInEditor(app, "output", output)
				}
				return nil
			case 'y':
				content := output
				panel := outputPanel
				if focusedInput {
					content = input
					panel = inputPanel
				}
				if treeEnabled {
					if selected, ok := tree.value(); ok {
						content = selected
					}
				}
				if content != "" {
					copyToClipboard(content)
					panel.SetTitle(fmt.Sprintf("%s Copied!", theme.IconCompleted))
					go func() {
						time.Sleep(1 * time.Second)
						app.JigApp().QueueUpdateDraw(func() {
							applyIOFocus()
						})
					}()
				}
				return nil
			case 'Y':
				if !treeEnabled {
					return event
				}
				content := output
				panel := outputPanel
				if focusedInput {
					content = input
					panel = inputPanel
				}
				if content != "" {
					copyToClipboard(content)
					panel.SetTitle(fmt.Sprintf("%s Copied!", theme.IconCompleted))
					go func() {
						time.Sleep(1 * time.Second)
						app.JigApp().QueueUpdateDraw(applyIOFocus)
					}()
				}
				return nil
			case 'q':
				closeModal()
				return nil
			}
		}
		return event
	}

	inputView.SetInputCapture(inputHandler)
	outputView.SetInputCapture(inputHandler)
	app.PushModal(modal)
	app.JigApp().SetFocus(inputView)
}
