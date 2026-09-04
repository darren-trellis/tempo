package view

import (
	"testing"
	"time"
)

func TestCopyWorkflowIDDoesNotBlock(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	if len(wl.workflows) == 0 {
		t.Fatal("expected mock workflows")
	}
	wl.table.SelectRow(0)

	done := make(chan struct{})
	go func() {
		wl.copyWorkflowID()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("copying a workflow ID should not block the UI thread")
	}
}
