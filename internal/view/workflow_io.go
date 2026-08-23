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

func formatIOContent(label, content string) string {
	if content == "" {
		return fmt.Sprintf("[%s]No %s[-]", theme.TagFgDim(), strings.ToLower(label))
	}
	return highlightFormattedJSONWorkflow(formatJSONPretty(content))
}

func showWorkflowIO(app *App, workflowType, input, output string, onClose func()) {
	if app == nil || app.JigApp() == nil {
		return
	}

	closeModal := func() {
		app.JigApp().Pages().DismissModal()
		if onClose != nil {
			onClose()
		}
	}

	modal := newResizableModal(components.ModalConfig{
		Title:     fmt.Sprintf("%s Input/Output: %s", theme.IconWorkflow, truncateStr(workflowType, 30)),
		Width:     0,
		Height:    0,
		MinWidth:  120,
		MinHeight: 35,
	})

	inputView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true)
	inputView.SetBackgroundColor(theme.Bg())
	inputView.SetTextColor(theme.Fg())
	inputView.SetText(formatIOContent("Input", input))

	outputView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true)
	outputView.SetBackgroundColor(theme.Bg())
	outputView.SetTextColor(theme.Fg())
	outputView.SetText(formatIOContent("Output", output))

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
		maxHint := "Maximize"
		if modal.maximized {
			maxHint = "Minimize"
		}
		modal.SetHints([]components.KeyHint{
			{Key: "tab/h/l/←/→", Description: "Switch"},
			{Key: "j/k", Description: "Scroll"},
			{Key: "m", Description: maxHint},
			{Key: "e", Description: "Editor"},
			{Key: "y", Description: "Copy"},
			{Key: "esc", Description: "Close"},
		})
	}
	applyIOHints()
	modal.SetOnCancel(closeModal)

	focusedInput := true
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

	scrollView := func(delta int) {
		view := outputView
		if focusedInput {
			view = inputView
		}
		scrollTextView(view, delta)
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
		case tcell.KeyLeft:
			if !focusedInput {
				switchFocus()
			}
			return nil
		case tcell.KeyRight:
			if focusedInput {
				switchFocus()
			}
			return nil
		case tcell.KeyDown:
			scrollView(1)
			return nil
		case tcell.KeyUp:
			scrollView(-1)
			return nil
		case tcell.KeyPgDn:
			scrollView(10)
			return nil
		case tcell.KeyPgUp:
			scrollView(-10)
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'h':
				if !focusedInput {
					switchFocus()
				}
				return nil
			case 'l':
				if focusedInput {
					switchFocus()
				}
				return nil
			case 'j':
				scrollView(1)
				return nil
			case 'k':
				scrollView(-1)
				return nil
			case 'g':
				if focusedInput {
					inputView.ScrollTo(0, 0)
				} else {
					outputView.ScrollTo(0, 0)
				}
				return nil
			case 'G':
				if focusedInput {
					inputView.ScrollToEnd()
				} else {
					outputView.ScrollToEnd()
				}
				return nil
			case 'm':
				modal.toggleMaximize()
				applyIOHints()
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
			case 'q':
				closeModal()
				return nil
			}
		}
		return event
	}

	inputView.SetInputCapture(inputHandler)
	outputView.SetInputCapture(inputHandler)
	app.JigApp().Pages().Push(modal)
	app.JigApp().SetFocus(inputView)
}
