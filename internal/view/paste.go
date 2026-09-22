package view

import (
	"strings"
	"unicode"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// installPasteScreen wraps the terminal screen so a bracketed paste arrives as
// one edit instead of a burst of keystrokes. Replayed as keystrokes, a newline
// in the clipboard submits the form and a tab walks to the next field, so the
// tail of the paste lands wherever the focus moved to.
func (a *App) installPasteScreen() {
	if a == nil || a.app == nil {
		return
	}
	tapp := a.app.GetApplication()
	if tapp == nil {
		return
	}
	base, err := tcell.NewScreen()
	if err != nil {
		return
	}
	screen := &pasteScreen{Screen: base}
	if err := screen.Init(); err != nil {
		return
	}
	screen.onPaste = func(text string) {
		tapp.QueueUpdateDraw(func() { deliverPaste(tapp, text) })
	}
	tapp.SetScreen(screen)
	// tview never sees a paste event, but the flag keeps bracketed paste on
	// when tview swaps in a replacement screen.
	tapp.EnablePaste(true)
	if a.mouseEnabled {
		screen.EnableMouse()
	}
}

type pasteScreen struct {
	tcell.Screen
	onPaste func(string)
	ready   bool
}

func (s *pasteScreen) Init() error {
	if s.ready {
		return nil
	}
	if err := s.Screen.Init(); err != nil {
		return err
	}
	s.ready = true
	s.Screen.EnablePaste()
	return nil
}

func (s *pasteScreen) Fini() {
	s.ready = false
	s.Screen.Fini()
}

func (s *pasteScreen) PollEvent() tcell.Event {
	for {
		event := s.Screen.PollEvent()
		paste, ok := event.(*tcell.EventPaste)
		if !ok {
			return event
		}
		if !paste.Start() {
			continue
		}
		text, closed := s.collectPaste()
		if text != "" && s.onPaste != nil {
			s.onPaste(text)
		}
		if closed {
			return nil
		}
	}
}

// collectPaste drains the keys the terminal sends between the paste markers.
// The bool reports that the screen shut down before the paste ended.
func (s *pasteScreen) collectPaste() (string, bool) {
	var text strings.Builder
	for {
		switch event := s.Screen.PollEvent().(type) {
		case nil:
			return text.String(), true
		case *tcell.EventPaste:
			if event.End() {
				return text.String(), false
			}
		case *tcell.EventKey:
			switch event.Key() {
			case tcell.KeyRune:
				text.WriteRune(event.Rune())
			case tcell.KeyEnter:
				text.WriteRune('\n')
			case tcell.KeyTab:
				text.WriteRune('\t')
			}
		}
	}
}

func deliverPaste(app *tview.Application, text string) {
	if app == nil {
		return
	}
	pasteInto(app.GetFocus(), text, func(p tview.Primitive) { app.SetFocus(p) })
}

func pasteInto(focus tview.Primitive, text string, setFocus func(tview.Primitive)) {
	if focus == nil || text == "" {
		return
	}
	target := pasteTarget(focus)
	switch field := target.(type) {
	case *tview.InputField:
		field.PasteHandler()(normalizePastedText(text, false), setFocus)
		return
	case *tview.TextArea:
		field.PasteHandler()(normalizePastedText(text, true), setFocus)
		return
	}
	handler := target.InputHandler()
	if handler == nil {
		return
	}
	for _, event := range pasteKeyEvents(text, acceptsPastedLines(target)) {
		handler(event, setFocus)
	}
}

// pasteTarget reaches past a form to the field that has focus. A form reads
// Enter as submit, so a multi-line field only keeps its line breaks when the
// text is handed to it directly.
func pasteTarget(focus tview.Primitive) tview.Primitive {
	form, ok := focus.(*components.Form)
	if !ok {
		return focus
	}
	for name := range form.GetValues() {
		if field := form.GetField(name); field != nil && field.HasFocus() {
			return field
		}
	}
	return focus
}

func acceptsPastedLines(target tview.Primitive) bool {
	_, ok := target.(*components.TextArea)
	return ok
}

func normalizePastedText(text string, multiline bool) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", " ")
	if !multiline {
		text = strings.ReplaceAll(text, "\n", " ")
		return strings.TrimSpace(text)
	}
	return strings.TrimRight(text, "\n ")
}

func pasteKeyEvents(text string, multiline bool) []*tcell.EventKey {
	text = normalizePastedText(text, multiline)
	events := make([]*tcell.EventKey, 0, len(text))
	for _, r := range text {
		switch {
		case r == '\n':
			events = append(events, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
		case unicode.IsPrint(r):
			events = append(events, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
		}
	}
	return events
}
