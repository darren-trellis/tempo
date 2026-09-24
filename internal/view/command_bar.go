package view

import (
	"fmt"
	"strings"

	"github.com/galaxy-io/tempo/internal/command"
)

func (a *App) showCommandBar() {
	p := a.prompt()
	p.suggestFn = func(input string) []commandSuggestion {
		return suggestionsFor(input, a.commandCatalog(), a.commandExtras())
	}
	p.input.SetChangedFunc(func(string) {
		if p.applying {
			return
		}
		p.history.resetBrowse()
		p.refreshSuggestions()
	})
	a.enterPrompt(": ", "command...")
	p.refreshSuggestions()
}

// restoreDefaultCommandCallbacks restores the default command bar callbacks.
func (a *App) restoreDefaultCommandCallbacks() {
	p := a.prompt()
	p.input.SetChangedFunc(nil)
	p.onComplete = nil
	p.suggestFn = nil
	p.suggestions.clear()
	p.onSubmit = func(text string) {
		a.recordCommand(text)
		a.exitPrompt()
		a.handleCommandInput(text)
	}
	p.onCancel = func() {
		a.exitPrompt()
	}
}

// CommandContextProvider is implemented by views that can provide workflow context for commands.
type CommandContextProvider interface {
	CommandContext() (workflowID, runID, workflowType string)
}

// handleCommandInput processes command bar input, dispatching to built-in or user commands.
func (a *App) handleCommandInput(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		if current := a.app.Pages().Current(); current != nil {
			a.app.SetFocus(current)
		}
		return
	}

	fields := strings.Fields(text)
	if a.executeBuiltinCommand(fields) {
		if a.app != nil && a.app.Pages() != nil && !a.app.Pages().CurrentIsModal() {
			a.refocusCurrent()
		}
		return
	}

	cmdName := fields[0]
	args := fields[1:]
	if a.config != nil {
		commands := a.config.GetMergedCommands(a.activeProfile)
		if cfg, ok := commands[cmdName]; ok {
			a.executeUserCommand(cmdName, cfg, args)
			return
		}
	}

	a.ToastWarning(fmt.Sprintf("Unknown command: %s", cmdName))
	a.refocusCurrent()
}

// buildCommandContext resolves all context variables from current app state.
func (a *App) buildCommandContext(args []string) command.Context {
	ctx := command.Context{
		Namespace: a.CurrentNamespace(),
		Profile:   a.activeProfile,
		Args:      args,
	}

	// Get connection details from active profile config
	if a.config != nil {
		if profile, ok := a.config.GetProfile(a.activeProfile); ok {
			expanded := profile.ExpandEnv()
			ctx.Address = expanded.Address
			ctx.TLSCertPath = expanded.TLS.Cert
			ctx.TLSKeyPath = expanded.TLS.Key
			ctx.TLSCAPath = expanded.TLS.CA
			ctx.TLSServerName = expanded.TLS.ServerName
			ctx.TLSSkipVerify = expanded.TLS.SkipVerify
			ctx.APIKey = expanded.APIKey
			ctx.CodecEndpoint = expanded.CodecEndpoint
		}
	}

	// Get workflow info from current view if it implements CommandContextProvider.
	// Walk the stack from top to bottom to find the first provider with data.
	stack := a.app.Pages().GetStack()
	for i := len(stack) - 1; i >= 0; i-- {
		if provider, ok := stack[i].(CommandContextProvider); ok {
			wfID, rID, wfType := provider.CommandContext()
			if wfID != "" {
				ctx.WorkflowID = wfID
				ctx.RunID = rID
				ctx.WorkflowType = wfType
				break
			}
		}
	}

	return ctx
}

// refocusCurrent restores focus to the current view.
func (a *App) refocusCurrent() {
	if current := a.app.Pages().Current(); current != nil {
		a.app.SetFocus(current)
	}
}
