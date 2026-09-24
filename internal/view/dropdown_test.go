package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestFilterTypeaheadOptionsPrefersPrefix(t *testing.T) {
	opts := []string{"OrderWorkflow", "PaymentWorkflow", "ReorderWorkflow"}
	got := filterDropdownOptions(opts, "ord")
	if len(got) != 2 || got[0] != "OrderWorkflow" || got[1] != "ReorderWorkflow" {
		t.Fatalf("prefix should sort first, got %v", got)
	}
}

func TestTypeaheadTabCompletesSelectedSuggestion(t *testing.T) {
	field := newDropdownField("workflowType", "Workflow Type", []string{"OrderWorkflow", "PaymentWorkflow"})
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

func TestDropdownChangedFuncFiresOnAccept(t *testing.T) {
	field := newOrderedDropdownField("key", "Key", []string{"WorkflowId", "StartTime"})
	field.SetValue("WorkflowId")
	var got string
	field.SetChangedFunc(func(v string) { got = v })
	field.openList()
	field.moveSelection(1)
	if !field.acceptSuggestion() {
		t.Fatal("accept should apply the highlighted key")
	}
	if got != "StartTime" {
		t.Fatalf("onChange=%q", got)
	}
}

func TestTypeaheadEscapeClosesDropdownNotForm(t *testing.T) {
	field := newDropdownField("workflowType", "Workflow Type", []string{"OrderWorkflow", "PaymentWorkflow"})
	field.moveSelection(1)
	if !field.expanded {
		t.Fatal("dropdown should be open")
	}
	capture := dropdownFormCapture(field)
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

func TestTypeaheadFormTabWalksSeveralOptions(t *testing.T) {
	field := newDropdownField("taskQueue", "Task Queue", []string{"orders", "payments"})
	field.Focus(func(tview.Primitive) {})
	field.openList()
	capture := dropdownFormCapture(field)
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
		t.Fatal("tab should walk the open list, not reach the form")
	}
	if field.selected != 1 {
		t.Fatalf("tab should move the highlight, selected %d", field.selected)
	}
	if field.GetValue() != "" {
		t.Fatalf("tab should not autocomplete with several options, got %q", field.GetValue())
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
		t.Fatal("tab should keep walking the open list")
	}
	if field.selected != 0 {
		t.Fatalf("tab should wrap around, selected %d", field.selected)
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Fatal("enter should be consumed by the open list")
	}
	if field.GetValue() != "orders" {
		t.Fatalf("enter should complete the highlighted option, got %q", field.GetValue())
	}
}

func TestTypeaheadFormTabCompletesTheOnlyOption(t *testing.T) {
	field := newDropdownField("taskQueue", "Task Queue", []string{"orders", "payments"})
	field.Focus(func(tview.Primitive) {})
	field.SetValue("ord")
	capture := dropdownFormCapture(field)
	if len(field.matches) != 1 {
		t.Fatalf("expected a single match, got %v", field.matches)
	}
	field.expanded = true
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
		t.Fatal("tab should complete the only option")
	}
	if field.GetValue() != "orders" {
		t.Fatalf("completed %q", field.GetValue())
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev == nil {
		t.Fatal("a second tab should move to the next field")
	}
}

func TestTypeaheadTabWhenClosedMovesToNextField(t *testing.T) {
	field := newDropdownField("taskQueue", "Task Queue", []string{"orders", "payments"})
	field.Focus(func(tview.Primitive) {})
	field.SetValue("ord")
	if field.expanded {
		t.Fatal("dropdown should stay closed after SetValue")
	}
	capture := dropdownFormCapture(field)
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev == nil {
		t.Fatal("tab should move to the next field when the dropdown is closed")
	}
	if field.GetValue() != "ord" {
		t.Fatalf("closed dropdown should not autocomplete, got %q", field.GetValue())
	}
}

func TestTypeaheadTypesJKAsLetters(t *testing.T) {
	field := newDropdownField("workflowType", "Workflow Type", []string{"ProjectWorkflow"})
	field.Focus(func(tview.Primitive) {})
	handler := field.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone), func(tview.Primitive) {})
	handler(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone), func(tview.Primitive) {})
	if field.GetValue() != "jk" {
		t.Fatalf("got %q", field.GetValue())
	}
}

func TestTypeaheadDropdownScrollsThroughAllMatches(t *testing.T) {
	opts := make([]string, dropdownMaxSuggestions+4)
	for i := range opts {
		opts[i] = string(rune('A'+i)) + "Workflow"
	}
	field := newDropdownField("workflowType", "Workflow Type", opts)
	field.Focus(func(tview.Primitive) {})
	field.moveSelection(1)
	if !field.expanded {
		t.Fatal("down should open the dropdown")
	}
	if field.GetFieldHeight() != 4+dropdownMaxSuggestions+2 {
		t.Fatalf("dropdown should stay %d rows tall, height=%d", 4+dropdownMaxSuggestions+2, field.GetFieldHeight())
	}
	if len(field.matches) != len(opts) {
		t.Fatalf("matches should keep every option, got %d", len(field.matches))
	}

	start, end := field.visibleMatches()
	if start != 0 || end != dropdownMaxSuggestions {
		t.Fatalf("initial window [%d,%d)", start, end)
	}

	for i := 0; i < dropdownMaxSuggestions; i++ {
		field.moveSelection(1)
	}
	if field.selected != dropdownMaxSuggestions {
		t.Fatalf("selected=%d", field.selected)
	}
	start, end = field.visibleMatches()
	if start != 1 || end != dropdownMaxSuggestions+1 {
		t.Fatalf("window should follow the selection, got [%d,%d)", start, end)
	}
	if field.matches[field.selected] != opts[dropdownMaxSuggestions] {
		t.Fatalf("selected %q", field.matches[field.selected])
	}

	field.selected = 0
	field.ensureListVisible()
	field.moveSelection(-1)
	if field.selected != len(opts)-1 {
		t.Fatalf("up from the top should wrap, selected=%d", field.selected)
	}
	start, end = field.visibleMatches()
	if end != len(opts) || start != len(opts)-dropdownMaxSuggestions {
		t.Fatalf("wrapped window [%d,%d)", start, end)
	}
}

func TestTypeaheadSetOptionsReplacesValues(t *testing.T) {
	field := newDropdownField("workflowType", "Workflow Type", []string{"PageWorkflow"})
	field.SetOptions([]string{"OrderWorkflow", "PaymentWorkflow"})
	got := filterDropdownOptions(field.options, "")
	if len(got) != 2 || got[0] != "OrderWorkflow" || got[1] != "PaymentWorkflow" {
		t.Fatalf("options=%v", got)
	}
}

func TestTypeaheadOpenShowsAllOptionsWhenValueSet(t *testing.T) {
	field := newDropdownField("workflowType", "Workflow Type", []string{"OrderWorkflow", "PaymentWorkflow", "ShippingWorkflow"})
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
	field := newDropdownField("workflowType", "Workflow Type", []string{"OrderWorkflow"})
	field.SetValue("OrderWorkflow")
	field.openList()
	field.SetOptions([]string{"PaymentWorkflow", "ShippingWorkflow"})
	if len(field.matches) != 2 {
		t.Fatalf("a catalog update should keep the full list open, got %v", field.matches)
	}
}

func TestOrderedDropdownKeepsInsertionOrder(t *testing.T) {
	opts := []string{"Last Hour", "Last 24 Hours", "Today", "Yesterday"}
	field := newOrderedDropdownField("preset", "Time Range", opts)
	if len(field.options) != 4 || field.options[0] != "Last Hour" || field.options[3] != "Yesterday" {
		t.Fatalf("ordered options=%v", field.options)
	}
	sorted := newDropdownField("preset", "Time Range", opts)
	if sorted.options[0] != "Last 24 Hours" {
		t.Fatalf("searchable dropdown should sort, got %v", sorted.options)
	}
}

func TestDropdownModalEscDismissesWhenClosed(t *testing.T) {
	field := newOrderedDropdownField("point", "Reset Point", []string{"#1  first"})
	field.SetValue("#1  first")
	form := components.NewFormBuilder().AddField(field).Build()
	modal := newModal(components.ModalConfig{Title: "Reset Workflow", Width: 40, Height: 12})
	modal.bindDropdowns(form, field)
	if !modal.GetBehavior().DismissOnEsc {
		t.Fatal("esc should still dismiss the modal when the dropdown is closed")
	}

	field.moveSelection(1)
	if modal.OnDismiss() {
		t.Fatal("esc should close the open dropdown first")
	}
	if field.expanded {
		t.Fatal("dropdown should be closed")
	}
	if !modal.OnDismiss() {
		t.Fatal("a second esc should dismiss the modal")
	}
}

func TestTypeaheadEnterSelectsOptionWithoutSubmitting(t *testing.T) {
	field := newDropdownField("taskQueue", "Task Queue", []string{"orders", "payments"})
	field.Focus(func(tview.Primitive) {})
	field.openList()
	field.moveSelection(1)
	capture := dropdownFormCapture(field)

	if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Fatal("enter should be consumed by the open dropdown, not reach the modal")
	}
	if field.GetValue() != "payments" {
		t.Fatalf("enter should select the highlighted option, got %q", field.GetValue())
	}
	if field.expanded {
		t.Fatal("dropdown should close after selecting")
	}
}

func TestTypeaheadEnterWhenClosedReachesModal(t *testing.T) {
	field := newDropdownField("taskQueue", "Task Queue", []string{"orders", "payments"})
	field.Focus(func(tview.Primitive) {})
	field.SetValue("orders")
	if field.expanded {
		t.Fatal("dropdown should be closed")
	}
	capture := dropdownFormCapture(field)
	if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev == nil {
		t.Fatal("enter should reach the modal when the dropdown is closed")
	}
	if field.GetValue() != "orders" {
		t.Fatalf("value should be untouched, got %q", field.GetValue())
	}
}

// Enter on an unfocused dropdown belongs to the modal even if some other
// dropdown on the form happens to be open.
func TestTypeaheadEnterIgnoresUnfocusedOpenDropdown(t *testing.T) {
	focused := newDropdownField("key", "Key", []string{"WorkflowId"})
	other := newDropdownField("op", "Operator", []string{"Equals", "Not Equals"})
	focused.Focus(func(tview.Primitive) {})
	other.openList()

	capture := dropdownFormCapture(focused, other)
	if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev == nil {
		t.Fatal("enter should reach the modal when the focused dropdown is closed")
	}
	if other.GetValue() != "" {
		t.Fatalf("unfocused dropdown should not be completed, got %q", other.GetValue())
	}
}
