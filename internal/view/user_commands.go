package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/command"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
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

	env := command.Env(cmdCtx)
	if cfg.Confirm {
		a.showCommandConfirm(name, cfg, expandedCmd, env)
		return
	}

	a.runCommand(name, expandedCmd, cfg, env)
}

// showCommandConfirm shows a confirmation modal before executing a command.
func (a *App) showCommandConfirm(name string, cfg config.CommandConfig, expandedCmd string, env []string) {
	a.showRunConfirm(name, cfg.Description, "Command", expandedCmd, func() {
		a.runCommand(name, expandedCmd, cfg, env)
	})
}

// showRunConfirm asks before run is called, showing what it will do. Either
// answer returns focus to wherever it was, such as the tree row a trigger
// fired from. The dialog has no fields, and a jig form without fields ignores
// every key, so the text view takes Enter itself.
func (a *App) showRunConfirm(name, description, label, detail string, run func()) {
	prevFocus := a.app.GetApplication().GetFocus()
	below := a.app.Pages().Current()
	// jig dismisses on Esc by itself and then focuses the page below, which
	// in the input/output window is always the input pane. Put focus back
	// once it is done, unless run opened a page of its own.
	refocus := func() bool {
		go a.app.QueueUpdateDraw(func() {
			if a.app.Pages().Current() != below {
				return
			}
			if prevFocus != nil {
				a.app.SetFocus(prevFocus)
			} else if current := a.app.Pages().Current(); current != nil {
				a.app.SetFocus(current)
			}
		})
		return true
	}

	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	infoText.SetBackgroundColor(theme.Bg())
	infoText.SetText(fmt.Sprintf("[%s]%s:[-] [%s]%s[-]\n\n[%s]%s[-]",
		theme.TagFgDim(), label, theme.TagAccent(), escapeForTView(name),
		theme.TagFg(), escapeForTView(detail)))
	infoText.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			a.app.Pages().DismissModal()
			run()
			return nil
		}
		return event
	})

	title := fmt.Sprintf("Confirm: %s", name)
	if description != "" {
		title = fmt.Sprintf("Confirm: %s", description)
	}

	modal := newModal(components.ModalConfig{
		Title:    title,
		Width:    70,
		Height:   12,
		Backdrop: true,
	})
	modal.SetContent(infoText)
	modal.SetOnDismiss(refocus)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
	})

	a.PushModal(modal)
	a.app.SetFocus(infoText)
}

// runCommand dispatches to the appropriate output handler based on config.
// env is added to the command's environment.
func (a *App) runCommand(name, expandedCmd string, cfg config.CommandConfig, env []string) {
	outputType := cfg.Output
	if outputType == "" {
		outputType = config.OutputLog
	}

	switch outputType {
	case config.OutputLog:
		a.runCommandLog(name, expandedCmd, cfg, env)
	case config.OutputJSON:
		a.runCommandJSON(name, expandedCmd, cfg, env)
	case config.OutputWorkflows:
		a.runCommandWorkflows(name, expandedCmd, env)
	case config.OutputWorkflow:
		a.runCommandWorkflow(name, expandedCmd, env)
	case config.OutputNone:
		a.runCommandNone(name, expandedCmd, env)
	default:
		a.runCommandLog(name, expandedCmd, cfg, env)
	}
}

// runCommandNone runs a command in the background, reporting only whether it
// worked. It has no timeout, so a script may hand off to a long-lived program.
func (a *App) runCommandNone(name, expandedCmd string, env []string) {
	go func() {
		output, err := command.Run(context.Background(), expandedCmd, env...)
		a.app.QueueUpdateDraw(func() {
			if err != nil {
				msg := fmt.Sprintf("%s: %s", name, err)
				if last := lastOutputLine(output); last != "" {
					msg = fmt.Sprintf("%s: %s", name, last)
				}
				a.ToastError(msg)
				return
			}
			a.ToastSuccess("Ran " + name)
		})
	}()
}

// lastOutputLine is the last non-blank line of output, which is usually where
// a failing script says why.
func lastOutputLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// runCommandLog creates a CommandOutputView with LogViewer, pushes it, and streams output.
func (a *App) runCommandLog(name, expandedCmd string, cfg config.CommandConfig, env []string) {
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
		}, env...)

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
func (a *App) runCommandJSON(name, expandedCmd string, cfg config.CommandConfig, env []string) {
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
		output, err := command.Run(ctx, expandedCmd, env...)
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
func (a *App) runCommandWorkflows(name, expandedCmd string, env []string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, runErr := command.Run(ctx, expandedCmd, env...)

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
func (a *App) runCommandWorkflow(name, expandedCmd string, env []string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		output, runErr := command.Run(ctx, expandedCmd, env...)

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
