package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
	if hintDescription(wl.Hints(), "[/]/1-2") != "" {
		t.Fatal("list tab keys should stay off the footer")
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
	if hintDescription(wl.Hints(), "esc") != "" {
		t.Fatalf("esc should stay off the footer, got %q", hintDescription(wl.Hints(), "esc"))
	}

	if !wl.HandleEscape() || wl.taskQueuesActive() {
		t.Fatal("escape should return to workflows")
	}
	if wl.workflowTab.Name != "Workflows (List)" {
		t.Fatalf("workflows tab title: %q", wl.workflowTab.Name)
	}
}

func TestEmptyWorkflowsStillSwitchListTabs(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.Start()
	wl.allWorkflows = nil
	wl.workflows = nil
	wl.populateTable()

	if wl.workflowTab == nil || wl.workflowTab.Content != wl.tableScroll {
		t.Fatal("empty workflows should keep the table mounted so tab keys still work")
	}

	var focused tview.Primitive
	wl.Focus(func(p tview.Primitive) { focused = p })
	if focused != wl.table {
		t.Fatal("empty workflows should keep focus on the table")
	}

	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ev != nil {
		t.Fatal("] should switch tabs when the workflows list is empty")
	}
	if !wl.taskQueuesActive() {
		t.Fatal("] should open task queues when the workflows list is empty")
	}
}

func TestWorkflowListSchedulesAndWorkersTabs(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if hintDescription(wl.Hints(), "s") != "" {
		t.Fatal("schedules should not have a dedicated footer key")
	}

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '3', 0)) {
		t.Fatal("3 should switch to schedules")
	}
	if !wl.schedulesActive() {
		t.Fatal("schedules tab should be active")
	}
	if wl.Name() != "schedules" {
		t.Fatalf("schedules name: %q", wl.Name())
	}
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("schedules should show a preview pane, got %d", wl.mainFlex.GetItemCount())
	}
	if hintDescription(wl.Hints(), "P") != "Pause/Unpause" {
		t.Fatalf("schedules hints: %q", hintDescription(wl.Hints(), "P"))
	}
	if hintDescription(wl.Hints(), "esc") != "" || hintDescription(wl.Hints(), "enter") != "" {
		t.Fatal("enter/esc should stay off the footer")
	}

	if !wl.HandleEscape() || !wl.workflowsActive() {
		t.Fatal("escape should return to workflows")
	}

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '4', 0)) {
		t.Fatal("4 should switch to workers")
	}
	if !wl.workersActive() {
		t.Fatal("workers tab should be active")
	}
	if wl.Name() != "workers" {
		t.Fatalf("workers name: %q", wl.Name())
	}
	if hintDescription(wl.Hints(), "/") != "Search" {
		t.Fatalf("workers hints: %q", hintDescription(wl.Hints(), "/"))
	}

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should wrap from workers to workflows")
	}
	if !wl.workflowsActive() {
		t.Fatal("] from workers should return to workflows")
	}
}

func TestWorkflowsTabDoesNotStealFocusCycle(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	capture := wl.listTabs.GetInputCapture()
	if capture == nil {
		t.Fatal("workflows tabs should capture keys")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
		t.Fatal("tab should be consumed")
	}
	if wl.taskQueuesActive() {
		t.Fatal("tab should not switch to task queues")
	}

	wl.loadMockData()
	wl.togglePreviewMode()
	if ev := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); ev != nil {
		t.Fatal("tab should still be consumed in preview")
	}
	if wl.taskQueuesActive() {
		t.Fatal("tab should cycle panes, not list tabs")
	}
	if wl.focusPane == focusWorkflows {
		t.Fatal("tab should move focus out of the workflows pane")
	}
}

func TestWorkflowListPreviewKeepsTabKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should switch list tabs when the main window is focused")
	}
	if !wl.taskQueuesActive() {
		t.Fatal("] should open task queues from the main window")
	}

	wl.setListKind(listWorkflows)
	wl.focusPane = focusEvents
	if wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should stay on preview tabs when preview is focused")
	}
	if !wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should still switch preview tabs")
	}
}

func TestTaskQueuesShowPollersAlongside(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.mainFlex.GetItemCount() != 1 {
		t.Fatalf("workflows-only layout: %d", wl.mainFlex.GetItemCount())
	}
	wl.setListKind(listTaskQueues)
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("pollers should open beside queues, got %d panes", wl.mainFlex.GetItemCount())
	}
	if wl.taskQueues.GetItemCount() != 0 {
		t.Fatal("pollers should not be nested inside the task queues tab")
	}
	if hintDescription(wl.Hints(), "enter") != "" || hintDescription(wl.Hints(), "esc") != "" {
		t.Fatal("enter/esc should stay off the footer")
	}

	wl.setFocusPane(focusPollers)
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("pollers should stay beside queues, got %d panes", wl.mainFlex.GetItemCount())
	}
	if wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("list tab keys should not work from pollers")
	}
	if !wl.HandleEscape() || wl.pollersVisible || wl.focusPane != focusWorkflows || !wl.taskQueuesActive() {
		t.Fatal("escape from pollers should hide the pane and return to queues")
	}
	if wl.mainFlex.GetItemCount() != 1 {
		t.Fatalf("hidden pollers should leave queues full width, got %d panes", wl.mainFlex.GetItemCount())
	}

	wl.setListKind(listWorkflows)
	wl.setListKind(listTaskQueues)
	if wl.pollersVisible || wl.mainFlex.GetItemCount() != 1 {
		t.Fatalf("hidden pollers should stay hidden after a tab switch, visible=%v panes=%d", wl.pollersVisible, wl.mainFlex.GetItemCount())
	}

	wl.setPollersVisible(true)
	wl.setListKind(listWorkflows)
	wl.setListKind(listTaskQueues)
	if !wl.pollersVisible || wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("visible pollers should stay open after a tab switch, visible=%v panes=%d", wl.pollersVisible, wl.mainFlex.GetItemCount())
	}
}

func TestTaskQueueKeysSurviveModalRestart(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.setListKind(listTaskQueues)
	if !wl.taskQueuesActive() {
		t.Fatal("task queues should be active")
	}
	if wl.shouldFocusWorkflowTable() {
		t.Fatal("workflow table should not take focus on the task queues tab")
	}

	wl.taskQueues.Stop()
	if wl.taskQueues.queueTable.GetInputCapture() != nil {
		t.Fatal("stop should clear queue captures")
	}

	wl.Start()
	if wl.taskQueues.queueTable.GetInputCapture() == nil {
		t.Fatal("start should restore queue keybindings after a modal")
	}
	if wl.shouldFocusWorkflowTable() {
		t.Fatal("start should keep focus on task queues")
	}
	if hintDescription(wl.Hints(), "[/]/1-2") != "" {
		t.Fatalf("list tab keys should stay off the footer, got %q", hintDescription(wl.Hints(), "[/]/1-2"))
	}
}
