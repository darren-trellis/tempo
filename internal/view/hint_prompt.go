package view

import (
	"strings"

	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type hintPrompt struct {
	input       *tview.InputField
	commandMode bool
	suggestion  string
	onSubmit    func(string)
	onCancel    func()
	onComplete  func(string) []string
	completions []string
	completeIdx int
}

func newHintPrompt() *hintPrompt {
	p := &hintPrompt{
		input:       tview.NewInputField(),
		completeIdx: -1,
	}
	p.input.SetBackgroundColor(theme.Bg())
	p.input.SetFieldBackgroundColor(theme.Bg())
	p.input.SetFieldTextColor(theme.Fg())
	p.input.SetLabelColor(theme.Accent())
	p.input.SetPlaceholderTextColor(theme.FgMuted())
	p.input.SetInputCapture(p.capture)
	return p
}

func (p *hintPrompt) capture(event *tcell.EventKey) *tcell.EventKey {
	if p == nil || event == nil {
		return event
	}
	switch event.Key() {
	case tcell.KeyTab:
		if p.acceptSuggestion() {
			return nil
		}
		p.complete(1)
		return nil
	case tcell.KeyBacktab:
		p.complete(-1)
		return nil
	case tcell.KeyRight:
		if p.acceptSuggestion() {
			return nil
		}
		return event
	case tcell.KeyEnter:
		if p.completeIdx >= 0 && p.completeIdx < len(p.completions) {
			p.input.SetText(p.completions[p.completeIdx])
			p.clearCompletions()
			return nil
		}
		if p.onSubmit != nil {
			p.onSubmit(p.input.GetText())
		}
		return nil
	case tcell.KeyEscape:
		if len(p.completions) > 0 {
			p.clearCompletions()
			return nil
		}
		if p.onCancel != nil {
			p.onCancel()
		}
		return nil
	}
	p.clearCompletions()
	return event
}

func (p *hintPrompt) enter(prompt, placeholder string) {
	p.commandMode = true
	p.suggestion = ""
	p.clearCompletions()
	p.input.SetLabel(prompt)
	p.input.SetPlaceholder(placeholder)
	p.input.SetText("")
	p.input.SetBackgroundColor(theme.Bg())
	p.input.SetFieldBackgroundColor(theme.Bg())
	p.input.SetFieldTextColor(theme.Fg())
	p.input.SetLabelColor(theme.Accent())
	p.input.SetPlaceholderTextColor(theme.FgMuted())
}

func (p *hintPrompt) exit() {
	p.commandMode = false
	p.suggestion = ""
	p.clearCompletions()
	p.input.SetChangedFunc(nil)
	p.input.SetText("")
}

func (p *hintPrompt) acceptSuggestion() bool {
	if p == nil || p.suggestion == "" {
		return false
	}
	current := p.input.GetText()
	if current == "" || !strings.HasPrefix(strings.ToLower(p.suggestion), strings.ToLower(current)) {
		return false
	}
	if p.suggestion == current {
		return false
	}
	p.input.SetText(p.suggestion)
	return true
}

func (p *hintPrompt) complete(delta int) {
	if p == nil || p.onComplete == nil {
		return
	}
	matches := p.onComplete(p.input.GetText())
	if len(matches) == 0 {
		p.clearCompletions()
		return
	}
	if len(p.completions) == 0 {
		p.completions = matches
		p.completeIdx = 0
	} else {
		p.completeIdx += delta
		if p.completeIdx < 0 {
			p.completeIdx = len(p.completions) - 1
		}
		if p.completeIdx >= len(p.completions) {
			p.completeIdx = 0
		}
	}
	p.input.SetText(p.completions[p.completeIdx])
}

func (p *hintPrompt) clearCompletions() {
	p.completions = nil
	p.completeIdx = -1
}

func (a *App) prompt() *hintPrompt {
	if a == nil {
		return nil
	}
	if a.hintPrompt == nil {
		a.hintPrompt = newHintPrompt()
	}
	return a.hintPrompt
}

func (a *App) promptActive() bool {
	return a != nil && a.hintPrompt != nil && a.hintPrompt.commandMode
}

func (a *App) handlePromptKey(event *tcell.EventKey) bool {
	if a == nil || !a.promptActive() || event == nil {
		return false
	}
	p := a.hintPrompt
	if p.input == nil {
		return false
	}
	handler := p.input.InputHandler()
	if handler == nil {
		return false
	}
	handler(event, func(tview.Primitive) {})
	return true
}

func (a *App) enterPrompt(prompt, placeholder string) {
	p := a.prompt()
	p.enter(prompt, placeholder)
	if a.app != nil {
		a.app.SetFocus(p.input)
	}
}

func (a *App) exitPrompt() {
	if a == nil || a.hintPrompt == nil {
		return
	}
	a.hintPrompt.exit()
	if a.app != nil && a.app.Pages() != nil {
		if current := a.app.Pages().Current(); current != nil {
			a.app.SetFocus(current)
		}
	}
}

func (a *App) drawHintPrompt(screen tcell.Screen) {
	if !a.promptActive() || a.menu == nil || screen == nil {
		return
	}
	x, y, width, height := a.menu.GetRect()
	if width < 1 || height < 1 {
		return
	}
	p := a.hintPrompt
	p.input.SetRect(x, y, width, height)
	p.input.Draw(screen)
	if p.suggestion == "" {
		return
	}
	current := p.input.GetText()
	if current == "" || !strings.HasPrefix(strings.ToLower(p.suggestion), strings.ToLower(current)) {
		return
	}
	suffix := p.suggestion[len(current):]
	if suffix == "" {
		return
	}
	ix, iy, iw, _ := p.input.GetInnerRect()
	col := ix + len([]rune(p.input.GetLabel())) + len([]rune(current))
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.FgMuted())
	for _, r := range suffix {
		if col >= ix+iw {
			break
		}
		screen.SetContent(col, iy, r, nil, style)
		col++
	}
}
