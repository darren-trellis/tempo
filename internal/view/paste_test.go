package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type scriptedScreen struct {
	tcell.Screen
	events []tcell.Event
}

func (s *scriptedScreen) PollEvent() tcell.Event {
	if len(s.events) == 0 {
		return nil
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event
}

func runeEvent(r rune) tcell.Event {
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
}

func TestPasteScreenFoldsABurstIntoOneEvent(t *testing.T) {
	var pasted []string
	screen := &pasteScreen{
		Screen: &scriptedScreen{events: []tcell.Event{
			tcell.NewEventPaste(true),
			runeEvent('a'),
			tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
			runeEvent('b'),
			tcell.NewEventPaste(false),
			runeEvent('z'),
		}},
		onPaste: func(text string) { pasted = append(pasted, text) },
	}

	event, ok := screen.PollEvent().(*tcell.EventKey)
	if !ok || event.Rune() != 'z' {
		t.Fatalf("expected the keystroke after the paste, got %#v", event)
	}
	if len(pasted) != 1 || pasted[0] != "a\nb" {
		t.Fatalf("expected one paste of %q, got %q", "a\nb", pasted)
	}
}

func TestPasteScreenStopsWhenTheScreenClosesMidPaste(t *testing.T) {
	var pasted []string
	screen := &pasteScreen{
		Screen: &scriptedScreen{events: []tcell.Event{
			tcell.NewEventPaste(true),
			runeEvent('a'),
		}},
		onPaste: func(text string) { pasted = append(pasted, text) },
	}

	if event := screen.PollEvent(); event != nil {
		t.Fatalf("expected a nil event once the screen closed, got %#v", event)
	}
	if len(pasted) != 1 || pasted[0] != "a" {
		t.Fatalf("expected the partial paste to be delivered, got %q", pasted)
	}
}

func TestNormalizePastedTextCollapsesLinesForSingleLineFields(t *testing.T) {
	got := normalizePastedText("  one\r\ntwo\tthree\n", false)
	if got != "one two three" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizePastedTextKeepsLinesForMultilineFields(t *testing.T) {
	got := normalizePastedText("{\r\n  \"a\": 1\r\n}\n", true)
	if got != "{\n  \"a\": 1\n}" {
		t.Fatalf("got %q", got)
	}
}

func TestPasteLandsInTheFocusedFieldOnly(t *testing.T) {
	key := newOrderedDropdownField("key", "Key", []string{"WorkflowId", "CustomerId"})
	op := newOrderedDropdownField("op", "Operator", []string{"=", "!="})
	value := newDropdownField("value", "Value", nil).SetPlaceholder("Value")
	form := components.NewFormBuilder().
		AddField(key).
		AddField(op).
		AddField(value).
		Build()
	form.Focus(func(tview.Primitive) {})
	form.InputHandler()(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), func(tview.Primitive) {})
	form.InputHandler()(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), func(tview.Primitive) {})
	if !value.HasFocus() {
		t.Fatal("expected the value field to have focus")
	}

	pasteInto(form, "cust_2xwKbQ\tfhYpB\nntVUx\n", func(tview.Primitive) {})

	if got := value.GetValue(); got != "cust_2xwKbQ fhYpB ntVUx" {
		t.Fatalf("value field got %q", got)
	}
	if key.GetValue() != "" || op.GetValue() != "" {
		t.Fatalf("paste leaked into other fields: key=%q op=%q", key.GetValue(), op.GetValue())
	}
}

func TestPasteKeepsLineBreaksInATextArea(t *testing.T) {
	input := components.NewTextArea("input")
	form := components.NewFormBuilder().
		Text("workflowId", "Workflow ID").
		Done().
		AddField(input).
		Build()
	form.Focus(func(tview.Primitive) {})
	form.InputHandler()(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), func(tview.Primitive) {})
	if !input.HasFocus() {
		t.Fatal("expected the text area to have focus")
	}

	pasteInto(form, "{\n  \"a\": 1\n}", func(tview.Primitive) {})

	if got := input.GetValue(); got != "{\n  \"a\": 1\n}" {
		t.Fatalf("text area got %q", got)
	}
}
