package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/rivo/tview"
)

func TestStartWorkflowHints(t *testing.T) {
	hints := startWorkflowHints()
	if desc := hintDescription(hints, "Enter"); desc != "Execute" {
		t.Fatalf("execute hint: %q", desc)
	}
	if desc := hintDescription(hints, "Ctrl+S"); desc != "" {
		t.Fatal("ctrl+s should not be the execute binding")
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
	need := startWorkflowFieldCount*startWorkflowFieldHeight + (startWorkflowFieldCount - 1)
	inner := startWorkflowModalHeight - 2
	if inner < need {
		t.Fatalf("height %d only leaves %d rows, need %d for %d fields", startWorkflowModalHeight, inner, need, startWorkflowFieldCount)
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
