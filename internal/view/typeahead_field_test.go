package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestFilterTypeaheadOptionsPrefersPrefix(t *testing.T) {
	opts := []string{"OrderWorkflow", "PaymentWorkflow", "ReorderWorkflow"}
	got := filterTypeaheadOptions(opts, "ord")
	if len(got) != 2 || got[0] != "OrderWorkflow" || got[1] != "ReorderWorkflow" {
		t.Fatalf("prefix should sort first, got %v", got)
	}
}

func TestTypeaheadTabCompletesSelectedSuggestion(t *testing.T) {
	field := newTypeaheadField("workflowType", "Workflow Type", []string{"OrderWorkflow", "PaymentWorkflow"})
	field.Focus(func(tview.Primitive) {})
	field.SetValue("Ord")
	field.openList()
	if !field.acceptSuggestion() {
		t.Fatal("tab should complete the matching type")
	}
	if field.GetValue() != "OrderWorkflow" {
		t.Fatalf("completed %q", field.GetValue())
	}
	if field.acceptSuggestion() {
		t.Fatal("a completed value should let tab move to the next field")
	}
}

func TestTypeaheadEscapeClosesDropdownNotForm(t *testing.T) {
	field := newTypeaheadField("workflowType", "Workflow Type", []string{"OrderWorkflow", "PaymentWorkflow"})
	field.moveSelection(1)
	if !field.expanded {
		t.Fatal("dropdown should be open")
	}
	capture := typeaheadFormTabCapture(field)
	if ev := capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Fatal("esc should close the dropdown even if the field is not marked focused")
	}
	if field.expanded {
		t.Fatal("dropdown should be closed")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev == nil {
		t.Fatal("a second esc should close the dialog")
	}
}

func TestTypeaheadFormTabCapture(t *testing.T) {
	field := newTypeaheadField("taskQueue", "Task Queue", []string{"orders", "payments"})
	field.Focus(func(tview.Primitive) {})
	field.SetValue("ord")
	field.openList()
	capture := typeaheadFormTabCapture(field)
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
		t.Fatal("tab should complete the suggestion")
	}
	if field.GetValue() != "orders" {
		t.Fatalf("completed %q", field.GetValue())
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev == nil {
		t.Fatal("a second tab should move to the next field")
	}
}

func TestTypeaheadTabWhenClosedMovesToNextField(t *testing.T) {
	field := newTypeaheadField("taskQueue", "Task Queue", []string{"orders", "payments"})
	field.Focus(func(tview.Primitive) {})
	field.SetValue("ord")
	if field.expanded {
		t.Fatal("dropdown should stay closed after SetValue")
	}
	capture := typeaheadFormTabCapture(field)
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev == nil {
		t.Fatal("tab should move to the next field when the dropdown is closed")
	}
	if field.GetValue() != "ord" {
		t.Fatalf("closed dropdown should not autocomplete, got %q", field.GetValue())
	}
}

func TestTypeaheadTypesJKAsLetters(t *testing.T) {
	field := newTypeaheadField("workflowType", "Workflow Type", []string{"ProjectWorkflow"})
	field.Focus(func(tview.Primitive) {})
	handler := field.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone), func(tview.Primitive) {})
	handler(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone), func(tview.Primitive) {})
	if field.GetValue() != "jk" {
		t.Fatalf("got %q", field.GetValue())
	}
}

func TestTypeaheadDropdownScrollsThroughAllMatches(t *testing.T) {
	opts := make([]string, typeaheadMaxSuggestions+4)
	for i := range opts {
		opts[i] = string(rune('A'+i)) + "Workflow"
	}
	field := newTypeaheadField("workflowType", "Workflow Type", opts)
	field.Focus(func(tview.Primitive) {})
	field.moveSelection(1)
	if !field.expanded {
		t.Fatal("down should open the dropdown")
	}
	if field.GetFieldHeight() != 4+typeaheadMaxSuggestions+2 {
		t.Fatalf("dropdown should stay %d rows tall, height=%d", 4+typeaheadMaxSuggestions+2, field.GetFieldHeight())
	}
	if len(field.matches) != len(opts) {
		t.Fatalf("matches should keep every option, got %d", len(field.matches))
	}

	start, end := field.visibleMatches()
	if start != 0 || end != typeaheadMaxSuggestions {
		t.Fatalf("initial window [%d,%d)", start, end)
	}

	for i := 0; i < typeaheadMaxSuggestions; i++ {
		field.moveSelection(1)
	}
	if field.selected != typeaheadMaxSuggestions {
		t.Fatalf("selected=%d", field.selected)
	}
	start, end = field.visibleMatches()
	if start != 1 || end != typeaheadMaxSuggestions+1 {
		t.Fatalf("window should follow the selection, got [%d,%d)", start, end)
	}
	if field.matches[field.selected] != opts[typeaheadMaxSuggestions] {
		t.Fatalf("selected %q", field.matches[field.selected])
	}

	field.selected = 0
	field.ensureListVisible()
	field.moveSelection(-1)
	if field.selected != len(opts)-1 {
		t.Fatalf("up from the top should wrap, selected=%d", field.selected)
	}
	start, end = field.visibleMatches()
	if end != len(opts) || start != len(opts)-typeaheadMaxSuggestions {
		t.Fatalf("wrapped window [%d,%d)", start, end)
	}
}

func TestTypeaheadSetOptionsReplacesValues(t *testing.T) {
	field := newTypeaheadField("workflowType", "Workflow Type", []string{"PageWorkflow"})
	field.SetOptions([]string{"OrderWorkflow", "PaymentWorkflow"})
	got := filterTypeaheadOptions(field.options, "")
	if len(got) != 2 || got[0] != "OrderWorkflow" || got[1] != "PaymentWorkflow" {
		t.Fatalf("options=%v", got)
	}
}

func TestTypeaheadOpenShowsAllOptionsWhenValueSet(t *testing.T) {
	field := newTypeaheadField("workflowType", "Workflow Type", []string{"OrderWorkflow", "PaymentWorkflow", "ShippingWorkflow"})
	field.SetValue("OrderWorkflow")
	if len(field.matches) != 1 || field.matches[0] != "OrderWorkflow" {
		t.Fatalf("a filled value should filter matches, got %v", field.matches)
	}
	field.moveSelection(1)
	if !field.expanded {
		t.Fatal("down should open the dropdown")
	}
	if len(field.matches) != 3 {
		t.Fatalf("open should show every option, got %v", field.matches)
	}
	if field.matches[field.selected] != "OrderWorkflow" {
		t.Fatalf("should highlight the current value, selected=%q", field.matches[field.selected])
	}
}

func TestTypeaheadSetOptionsKeepsFullListWhenOpen(t *testing.T) {
	field := newTypeaheadField("workflowType", "Workflow Type", []string{"OrderWorkflow"})
	field.SetValue("OrderWorkflow")
	field.openList()
	field.SetOptions([]string{"PaymentWorkflow", "ShippingWorkflow"})
	if len(field.matches) != 2 {
		t.Fatalf("a catalog update should keep the full list open, got %v", field.matches)
	}
}

func TestStartWorkflowSuggestionsFromLoadedLists(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.allWorkflows = []temporal.Workflow{
		{Type: "OrderWorkflow", TaskQueue: "orders"},
		{Type: "PaymentWorkflow", TaskQueue: "payments"},
		{Type: "OrderWorkflow", TaskQueue: "orders"},
	}
	types, queues := suggestionsFromWorkflowList(wl)
	if len(types) != 2 || types[0] != "OrderWorkflow" || types[1] != "PaymentWorkflow" {
		t.Fatalf("types=%v", types)
	}
	if len(queues) != 2 || queues[0] != "orders" || queues[1] != "payments" {
		t.Fatalf("queues=%v", queues)
	}
}
