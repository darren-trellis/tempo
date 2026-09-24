package view

import (
	"context"
	"fmt"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/command"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/rivo/tview"
)

// executeUserCommand expands the command template, optionally confirms, then runs it.
func (a *App) executeUserCommand(name string, cfg config.CommandConfig, args []string) {
	cmdCtx := a.buildCommandContext(args)

	expandedCmd, err := command.ExpandCmd(cfg.Cmd, cmdCtx)
	if err != nil {
		a.ToastError(fmt.Sprintf("Command %q: %s", name, err))
		a.refocusCurrent()
		return
	}

	// Inject connection flags for temporal CLI commands
	expandedCmd = command.InjectConnectionFlags(expandedCmd, cmdCtx)

	if cfg.Confirm {
		a.showCommandConfirm(name, cfg, expandedCmd)
		return
	}

	a.runCommand(name, expandedCmd, cfg)
}

// showCommandConfirm shows a confirmation modal before executing a command.
func (a *App) showCommandConfirm(name string, cfg config.CommandConfig, expandedCmd string) {
	form := components.NewFormBuilder().
		OnSubmit(func(values map[string]any) {
			a.app.Pages().DismissModal()
			a.runCommand(name, expandedCmd, cfg)
		}).
		OnCancel(func() {
			a.app.Pages().DismissModal()
			if current := a.app.Pages().Current(); current != nil {
				a.app.SetFocus(current)
			}
		}).
		Build()

	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.SetBackgroundColor(theme.Bg())

	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	infoText.SetBackgroundColor(theme.Bg())
	infoText.SetText(fmt.Sprintf("[%s]Command:[-] [%s]%s[-]\n\n[%s]%s[-]",
		theme.TagFgDim(), theme.TagAccent(), name,
		theme.TagFg(), expandedCmd))

	contentFlex.AddItem(infoText, 4, 0, false)
	contentFlex.AddItem(form, 0, 1, true)

	title := fmt.Sprintf("Confirm: %s", name)
	if cfg.Description != "" {
		title = fmt.Sprintf("Confirm: %s", cfg.Description)
	}

	modal := newModal(components.ModalConfig{
		Title:    title,
		Width:    70,
		Height:   12,
		Backdrop: true,
	})
	modal.SetContent(contentFlex)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
	})

	a.PushModal(modal)
	a.app.SetFocus(form)
}

// runCommand dispatches to the appropriate output handler based on config.
func (a *App) runCommand(name, expandedCmd string, cfg config.CommandConfig) {
	outputType := cfg.Output
	if outputType == "" {
		outputType = config.OutputLog
	}

	switch outputType {
	case config.OutputLog:
		a.runCommandLog(name, expandedCmd, cfg)
	case config.OutputJSON:
		a.runCommandJSON(name, expandedCmd, cfg)
	case config.OutputWorkflows:
		a.runCommandWorkflows(name, expandedCmd, cfg)
	case config.OutputWorkflow:
		a.runCommandWorkflow(name, expandedCmd, cfg)
	default:
		a.runCommandLog(name, expandedCmd, cfg)
	}
}

// runCommandLog creates a CommandOutputView with LogViewer, pushes it, and streams output.
func (a *App) runCommandLog(name, expandedCmd string, cfg config.CommandConfig) {
	ctx, cancel := context.WithCancel(context.Background())

	lv := components.NewLogViewer()
	description := cfg.Description
	if description == "" {
		description = name
	}
	view := NewCommandOutputView(a, name, description, lv, cancel)

	a.app.Pages().Push(view)
	a.app.SetFocus(lv)

	go func() {
		lv.AddEntry(components.LogEntry{
			Level:   components.LogLevelInfo,
			Message: "$ " + expandedCmd,
		})

		err := command.RunStreaming(ctx, expandedCmd, func(line string) {
			a.app.QueueUpdateDraw(func() {
				lv.AddEntry(components.LogEntry{
					Level:   components.LogLevelInfo,
					Message: line,
				})
			})
		})

		a.app.QueueUpdateDraw(func() {
			if err != nil && ctx.Err() == nil {
				lv.AddEntry(components.LogEntry{
					Level:   components.LogLevelError,
					Message: fmt.Sprintf("Error: %s", err),
				})
			} else {
				lv.AddEntry(components.LogEntry{
					Level:   components.LogLevelInfo,
					Message: "--- Done ---",
				})
			}
		})
	}()
}

// runCommandJSON runs a command and displays the result in a CodeView with JSON highlighting.
func (a *App) runCommandJSON(name, expandedCmd string, cfg config.CommandConfig) {
	ctx, cancel := context.WithCancel(context.Background())

	cv := components.NewCodeView().SetLanguage(components.LangJSON)
	cv.SetCode("Running...")

	description := cfg.Description
	if description == "" {
		description = name
	}
	view := NewCommandOutputView(a, name, description, cv, cancel)

	a.app.Pages().Push(view)
	a.app.SetFocus(cv)

	go func() {
		output, err := command.Run(ctx, expandedCmd)
		a.app.QueueUpdateDraw(func() {
			if err != nil && ctx.Err() == nil {
				cv.SetCode(fmt.Sprintf("Error: %s\n\n%s", err, output))
			} else {
				// Try to pretty-print JSON
				formatted := formatJSONPretty(output)
				cv.SetCode(formatted)
			}
		})
	}()
}

// runCommandWorkflows runs a command, parses JSONL output, and pushes a WorkflowList.
func (a *App) runCommandWorkflows(name, expandedCmd string, _ config.CommandConfig) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, runErr := command.Run(ctx, expandedCmd)

		// Try to parse output even on non-zero exit (CLI may still produce valid JSONL)
		workflows, parseErr := command.ParseWorkflowsOutput(output)
		if parseErr != nil {
			// If both run and parse failed, show the run error (more useful)
			errMsg := parseErr.Error()
			if runErr != nil {
				errMsg = runErr.Error()
			}
			a.app.QueueUpdateDraw(func() {
				a.ToastError(fmt.Sprintf("Command %q: %s", name, errMsg))
				a.refocusCurrent()
			})
			return
		}

		a.app.QueueUpdateDraw(func() {
			wl := NewWorkflowListWithData(a, a.CurrentNamespace(), workflows)
			a.app.Pages().Push(wl)
			a.app.SetFocus(wl)
		})
	}()
}

// runCommandWorkflow runs a command, parses workflow ID/run ID, and opens Preview.
func (a *App) runCommandWorkflow(name, expandedCmd string, _ config.CommandConfig) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, runErr := command.Run(ctx, expandedCmd)

		workflowID, runID, parseErr := command.ParseWorkflowOutput(output)
		if parseErr != nil {
			errMsg := parseErr.Error()
			if runErr != nil {
				errMsg = runErr.Error()
			}
			a.app.QueueUpdateDraw(func() {
				a.ToastError(fmt.Sprintf("Command %q: %s", name, errMsg))
				a.refocusCurrent()
			})
			return
		}

		a.app.QueueUpdateDraw(func() {
			a.OpenWorkflowPreview(workflowID, runID)
		})
	}()
}
