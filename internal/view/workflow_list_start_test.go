package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestStartWorkflowHints(t *testing.T) {
	hints := startWorkflowHints()
	if desc := hintDescription(hints, "Tab"); desc != "Complete / Next" {
		t.Fatalf("tab hint: %q", desc)
	}
	if desc := hintDescription(hints, "Enter"); desc != "Execute" {
		t.Fatalf("execute hint: %q", desc)
	}
	if desc := hintDescription(hints, "Ctrl+S"); desc != "" {
		t.Fatal("ctrl+s should not be the execute binding")
	}
}

func TestStartWorkflowModalKeepsEscForDropdown(t *testing.T) {
	modal := newOverlayModal(components.ModalConfig{
		Title:  "Start Workflow",
		Width:  70,
		Height: startWorkflowModalHeight,
	}, tview.NewBox())
	modal.SetDismissOnEsc(false)
	if modal.GetBehavior().DismissOnEsc {
		t.Fatal("esc should reach the form so an open dropdown can close first")
	}

	field := newTypeaheadField("workflowType", "Workflow Type", []string{"OrderWorkflow"})
	field.moveSelection(1)
	modal.interceptEscape = func() bool {
		return collapseOpenTypeahead(field)
	}
	modal.SetOnDismiss(func() bool {
		return !collapseOpenTypeahead(field)
	})
	if !modal.InterceptEscape() {
		t.Fatal("esc should close the open dropdown")
	}
	if field.expanded {
		t.Fatal("dropdown should be closed")
	}
	if !modal.OnDismiss() {
		t.Fatal("a second esc should allow the dialog to close")
	}
}

func TestStartWorkflowModalHintsStayInFooter(t *testing.T) {
	modal := newOverlayModal(components.ModalConfig{
		Title:  "Start Workflow",
		Width:  70,
		Height: startWorkflowModalHeight,
	}, tview.NewBox())
	modal.SetHints(startWorkflowHints())
	if hintDescription(modal.Hints(), "Enter") != "Execute" {
		t.Fatalf("footer hints: %+v", modal.Hints())
	}
	if bar := modal.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("hints should stay in the footer, got in-modal %+v", bar.Hints)
	}
}

func TestStartWorkflowModalFitsFields(t *testing.T) {
	need := (startWorkflowFieldCount-1)*startWorkflowFieldHeight + startWorkflowInputHeight + (startWorkflowFieldCount - 1)
	inner := startWorkflowModalHeight - 2
	if inner < need {
		t.Fatalf("height %d only leaves %d rows, need %d for %d fields", startWorkflowModalHeight, inner, need, startWorkflowFieldCount)
	}
}

func TestStartWorkflowInputEnterInsertsNewline(t *testing.T) {
	input := components.NewTextArea("input").SetValue(`{"a":1}`)
	input.Focus(func(tview.Primitive) {})
	capture := startWorkflowFormCapture(input)
	if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Fatal("enter in the input field should insert a newline instead of submitting")
	}
	if got := input.GetValue(); got != "\n{\"a\":1}" {
		t.Fatalf("got %q", got)
	}
}

func TestStartWorkflowSubmitRoutesOnSignal(t *testing.T) {
	plain := startWorkflowSubmit{WorkflowID: "wf", WorkflowType: "Type", TaskQueue: "q"}
	if err := plain.validate(); err != nil {
		t.Fatalf("plain start: %v", err)
	}
	if plain.withSignal() {
		t.Fatal("empty signal name should start without a signal")
	}

	signaled := startWorkflowSubmit{WorkflowID: "wf", WorkflowType: "Type", TaskQueue: "q", SignalName: " ready "}
	if err := signaled.validate(); err != nil {
		t.Fatalf("signal start: %v", err)
	}
	if !signaled.withSignal() || signaled.signalName() != "ready" {
		t.Fatalf("signal name: %q with=%v", signaled.signalName(), signaled.withSignal())
	}

	invalid := startWorkflowSubmit{SignalInput: `{"x":1}`}
	if err := invalid.validate(); err == nil {
		t.Fatal("signal input without a name should fail")
	}
}
