package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestMatchWorkerInstanceRanksCandidates(t *testing.T) {
	workers := sortWorkerInstances([]temporal.Worker{
		{Host: "host-001", Identity: "worker-1", TaskQueue: "orders"},
		{Host: "host-001", Identity: "worker-1", TaskQueue: "payments"},
		{Host: "host-001", Identity: "worker-9", TaskQueue: "orders"},
		{Host: "host-002", Identity: "worker-2", TaskQueue: "shipments"},
	})

	// An identity plus queue match is the best answer.
	if got := matchWorkerInstance(workers, "worker-1", "payments"); got == nil || got.TaskQueue != "payments" {
		t.Fatalf("identity+queue: %+v", got)
	}
	// Identity alone still beats a same-host instance.
	if got := matchWorkerInstance(workers, "worker-1", "unknown-queue"); got == nil || got.Identity != "worker-1" {
		t.Fatalf("identity only: %+v", got)
	}
	// A poller identity that carries the host resolves through the host.
	got := matchWorkerInstance(workers, "someone@host-002", "")
	if got == nil || got.Identity != "worker-2" {
		t.Fatalf("host fallback: %+v", got)
	}
	// Same host, and the queue picks which instance.
	got = matchWorkerInstance(workers, "someone@host-001", "orders")
	if got == nil || got.TaskQueue != "orders" {
		t.Fatalf("host+queue: %+v", got)
	}
	if got := matchWorkerInstance(workers, "nobody@host-404", "orders"); got != nil {
		t.Fatalf("unknown host should not match: %+v", got)
	}
	if got := matchWorkerInstance(workers, "", ""); got != nil {
		t.Fatalf("empty identity should not match: %+v", got)
	}
}

func TestRevealInstanceSelectsTheRow(t *testing.T) {
	wv := NewWorkerView(&App{})
	wv.loadMockData()
	if len(wv.rows) != len(wv.allWorkers) {
		t.Fatalf("the tab lists instances flat: %d rows for %d workers", len(wv.rows), len(wv.allWorkers))
	}
	wv.table.SelectRow(0)

	if !wv.RevealInstance("worker-3@host-002", "") {
		t.Fatal("revealing a poller identity should find its instance")
	}
	row, ok := wv.selectedRow()
	if !ok || row.Worker.Identity != "worker-3" {
		t.Fatalf("selected row: %+v ok=%v", row, ok)
	}
	if infoRowValue(wv.detailRows, "identity") != "worker-3" {
		t.Fatalf("the detail pane should follow: %+v", wv.detailRows)
	}
}

func TestRevealInstanceWaitsForLoad(t *testing.T) {
	wv := NewWorkerView(&App{})
	if wv.RevealInstance("worker-2@host-001", "payment-tasks") {
		t.Fatal("there are no workers loaded yet, so nothing can be selected")
	}
	if wv.pending == nil {
		t.Fatal("the request should be remembered until the workers land")
	}

	wv.loadMockData()
	if wv.pending != nil {
		t.Fatal("the pending request should be cleared once applied")
	}
	row, ok := wv.selectedRow()
	if !ok || row.Worker.Identity != "worker-2" {
		t.Fatalf("selected row after load: %+v ok=%v", row, ok)
	}
}

func TestPollerEnterOpensWorkerInstance(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.setListKind(listTaskQueues)
	tq := wl.taskQueues
	tq.pollers = []temporal.Poller{
		{Identity: "worker-1@host-001", TaskQueueType: "Workflow"},
		{Identity: "worker-3@host-002", TaskQueueType: "Activity"},
	}
	tq.populatePollerTable("")
	if len(tq.visiblePollers) != 2 {
		t.Fatalf("visible pollers: %d", len(tq.visiblePollers))
	}

	// Only the rows on screen count: filtering by type must not shift the mapping.
	tq.populatePollerTable("Activity")
	if len(tq.visiblePollers) != 1 || tq.visiblePollers[0].Identity != "worker-3@host-002" {
		t.Fatalf("filtered pollers: %+v", tq.visiblePollers)
	}
	tq.pollerTable.SelectRow(0)
	poller, ok := tq.selectedPoller()
	if !ok || poller.Identity != "worker-3@host-002" {
		t.Fatalf("selected poller: %+v ok=%v", poller, ok)
	}

	capture := tq.pollerTable.GetInputCapture()
	if capture == nil {
		t.Fatal("the poller table has no input capture")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Fatal("enter should be consumed")
	}
	if !wl.workersActive() {
		t.Fatal("enter on a poller should open the workers tab")
	}
	row, ok := wl.workers.selectedRow()
	if !ok || row.Worker.Identity != "worker-3" {
		t.Fatalf("workers should land on the poller's instance: %+v ok=%v", row, ok)
	}
	if wl.focusPane != focusWorkflows {
		t.Fatalf("focus should sit on the worker list, got %d", wl.focusPane)
	}

	wl.setListKind(listTaskQueues)
	wl.focusPane = focusPollers
	if hintDescription(wl.Hints(), "Enter") != "Show Worker" {
		t.Fatalf("pollers hint: %q", hintDescription(wl.Hints(), "Enter"))
	}
}
