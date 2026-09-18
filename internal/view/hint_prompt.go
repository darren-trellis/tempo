package view

import (
	"strings"

	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type hintPrompt struct {
	input         *tview.InputField
	commandMode   bool
	suggestion    string
	onSubmit      func(string)
	onCancel      func()
	onComplete    func(string) []string
	completions   []string
	completeIdx   int
	suggestFn     func(string) []commandSuggestion
	suggestions   commandCompletionState
	history       *commandHistory
	searchHistory *commandHistory
	applying      bool
}

func newHintPrompt() *hintPrompt {
	p := &hintPrompt{
		input:       tview.NewInputField(),
		completeIdx: -1,
	}
	p.applyTheme()
	p.input.SetInputCapture(p.capture)
	return p
}

func (p *hintPrompt) applyTheme() {
	if p == nil || p.input == nil {
		return
	}
	bg := theme.Bg()
	p.input.SetBackgroundColor(bg)
	p.input.SetFieldStyle(tcell.StyleDefault.Background(bg).Foreground(theme.Fg()))
	p.input.SetPlaceholderStyle(tcell.StyleDefault.Background(bg).Foreground(theme.FgMuted()))
	p.input.SetLabelStyle(tcell.StyleDefault.Background(bg).Foreground(theme.Accent()))
}

func (p *hintPrompt) capture(event *tcell.EventKey) *tcell.EventKey {
	if p == nil || event == nil {
		return event
	}
	if p.suggestFn != nil {
		return p.captureCommand(event)
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
	case tcell.KeyUp:
		p.applySearchHistory(p.searchHistory.prev(p.input.GetText()))
		return nil
	case tcell.KeyDown:
		p.applySearchHistory(p.searchHistory.next(p.input.GetText()))
		return nil
	}
	p.clearCompletions()
	return event
}

func (p *hintPrompt) applySearchHistory(text string) {
	if p == nil || p.input == nil {
		return
	}
	p.setInputText(text)
}

func (p *hintPrompt) captureCommand(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		if p.input != nil && strings.TrimSpace(p.input.GetText()) == "" {
			if p.onCancel != nil {
				p.onCancel()
			}
			return nil
		}
		if len(p.suggestions.items) > 0 {
			p.suggestions.clear()
			return nil
		}
		if p.onCancel != nil {
			p.onCancel()
		}
		return nil
	case tcell.KeyEnter:
		if p.suggestions.browsed && p.suggestions.selectedItem() != nil {
			p.applySelectedSuggestion()
			return nil
		}
		if p.onSubmit != nil {
			p.onSubmit(p.input.GetText())
		}
		return nil
	case tcell.KeyTab:
		p.tabComplete(true)
		return nil
	case tcell.KeyBacktab:
		p.tabComplete(false)
		return nil
	case tcell.KeyDown:
		if p.history.browsing() {
			p.applyHistory(p.history.next(p.input.GetText()))
			return nil
		}
		p.suggestions.step(1)
		return nil
	case tcell.KeyUp:
		if p.suggestions.browsed {
			p.suggestions.step(-1)
			return nil
		}
		p.applyHistory(p.history.prev(p.input.GetText()))
		return nil
	}
	return event
}

func (p *hintPrompt) applyHistory(text string) {
	if p == nil || p.input == nil {
		return
	}
	p.setInputText(text)
	p.refreshSuggestions()
}

func (p *hintPrompt) refreshSuggestions() {
	if p == nil {
		return
	}
	if p.suggestFn == nil {
		p.suggestions.clear()
		return
	}
	text := ""
	if p.input != nil {
		text = p.input.GetText()
	}
	p.suggestions.items = p.suggestFn(text)
	p.suggestions.selected = nil
	p.suggestions.browsed = false
	p.suggestions.scroll = 0
}

func (p *hintPrompt) setInputText(text string) {
	if p == nil || p.input == nil {
		return
	}
	p.applying = true
	p.input.SetText(text)
	p.applying = false
}

func (p *hintPrompt) applySelectedSuggestion() {
	if p == nil || p.input == nil {
		return
	}
	sel := p.suggestions.selectedItem()
	if sel == nil || sel.Text == "" {
		return
	}
	buf := p.input.GetText()
	from := sel.ReplaceFrom
	if from < 0 {
		from = 0
	}
	if from > len(buf) {
		from = len(buf)
	}
	p.setInputText(buf[:from] + sel.Text)
	p.suggestions.browsed = false
}

func (p *hintPrompt) selectionApplied() bool {
	sel := p.suggestions.selectedItem()
	if sel == nil || p.input == nil {
		return false
	}
	buf := p.input.GetText()
	from := sel.ReplaceFrom
	if from < 0 || from > len(buf) {
		return false
	}
	return buf[from:] == sel.Text
}

func (p *hintPrompt) tabComplete(next bool) {
	if p == nil {
		return
	}
	if len(p.suggestions.items) == 0 {
		p.refreshSuggestions()
		if len(p.suggestions.items) == 0 {
			return
		}
	}
	if p.suggestions.selected == nil || p.selectionApplied() {
		if next {
			p.suggestions.selectNext()
		} else {
			p.suggestions.selectPrev()
		}
	}
	p.applySelectedSuggestion()
}

func (p *hintPrompt) enter(prompt, placeholder string) {
	p.commandMode = true
	p.suggestion = ""
	p.clearCompletions()
	p.suggestions.clear()
	p.history.resetBrowse()
	p.searchHistory.resetBrowse()
	p.input.SetLabel(prompt)
	p.input.SetPlaceholder(placeholder)
	p.setInputText("")
	p.applyTheme()
}

func (p *hintPrompt) exit() {
	p.commandMode = false
	p.suggestion = ""
	p.suggestFn = nil
	p.clearCompletions()
	p.suggestions.clear()
	p.history.resetBrowse()
	p.searchHistory.resetBrowse()
	p.input.SetChangedFunc(nil)
	p.setInputText("")
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
	p.applyTheme()
	p.input.SetRect(x, y, width, height)
	p.input.Draw(screen)
	a.drawCommandSuggestions(screen, x, y, width)
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

func (a *App) drawCommandSuggestions(screen tcell.Screen, x, menuY, width int) {
	if a.IsFilterMode() || screen == nil {
		return
	}
	p := a.hintPrompt
	if p == nil || p.suggestFn == nil || len(p.suggestions.items) == 0 {
		return
	}
	h := p.suggestions.desiredHeight(menuY)
	if h < 2 || width < 8 {
		return
	}
	y := menuY - h
	if y < 0 {
		h += y
		y = 0
	}
	drawCommandSuggestionBox(screen, x, y, width, h, &p.suggestions)
}
