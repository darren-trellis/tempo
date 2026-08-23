package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestWorkflowListTitleListMode(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.workflowTab == nil || wl.workflowTab.Name != "Workflows (List)" {
		t.Fatalf("list title: %+v", wl.workflowTab)
	}
	wl.toggleWorkflowTree()
	if wl.workflowTab.Name != "Workflows (Tree)" {
		t.Fatalf("tree title: %q", wl.workflowTab.Name)
	}
	wl.toggleWorkflowTree()
	if wl.workflowTab.Name != "Workflows (List)" {
		t.Fatalf("restored list title: %q", wl.workflowTab.Name)
	}
}

func TestWorkflowListTaskQueueTab(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if hintDescription(wl.Hints(), "t") != "" {
		t.Fatal("task queues should not have a dedicated key")
	}
	if hintDescription(wl.Hints(), "[/]/1-2") != "View" {
		t.Fatalf("list tabs should use [/]/1-2, got %q", hintDescription(wl.Hints(), "[/]/1-2"))
	}
	if wl.Name() != "workflows" {
		t.Fatalf("name: %q", wl.Name())
	}

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should switch to task queues")
	}
	if !wl.taskQueuesActive() {
		t.Fatal("task queues tab should be active")
	}
	if wl.Name() != "task-queues" {
		t.Fatalf("task queue name: %q", wl.Name())
	}
	if hintDescription(wl.Hints(), "t") != "" {
		t.Fatal("task queues tab should not restore the t key")
	}
	if hintDescription(wl.Hints(), "esc") != "Workflows" {
		t.Fatalf("esc hint: %q", hintDescription(wl.Hints(), "esc"))
	}

	if !wl.HandleEscape() || wl.taskQueuesActive() {
		t.Fatal("escape should return to workflows")
	}
	if wl.workflowTab.Name != "Workflows (List)" {
		t.Fatalf("workflows tab title: %q", wl.workflowTab.Name)
	}
}

func TestWorkflowListPreviewKeepsTabKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	if wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should stay on preview tabs while preview is open")
	}
	if !wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should still switch preview tabs")
	}
}
