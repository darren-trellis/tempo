package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestPrimaryPaneHasNoTitle(t *testing.T) {
	a := &App{chromeProfile: "prod"}
	wl := NewWorkflowList(a, "default")
	if got := paneTitle(wl.workflowsPanel); got != "" {
		t.Fatalf("primary pane should have no title, got %q", got)
	}
	a.app = nil
	a.setProfile("staging")
	wl.applyProfileTitle()
	if got := paneTitle(wl.workflowsPanel); got != "" {
		t.Fatalf("profile should not return to the pane, got %q", got)
	}
}

func TestPrimaryPaneOmitsCountsAndFilter(t *testing.T) {
	a := &App{chromeProfile: "prod"}
	wl := NewWorkflowList(a, "default")
	wl.allWorkflows = []temporal.Workflow{
		{ID: "wf-1", RunID: "run-1", Status: "Running"},
		{ID: "wf-2", RunID: "run-2", Status: "Completed"},
		{ID: "wf-3", RunID: "run-3", Status: "Running"},
	}
	wl.workflows = wl.allWorkflows[:1]
	wl.populateTable()
	if got := paneTitle(wl.workflowsPanel); got != "" {
		t.Fatalf("counts should not live on the pane, got %q", got)
	}
	if wl.loadedWorkflowCount() != 3 || wl.displayedWorkflowCount() != 1 {
		t.Fatalf("loaded=%d displayed=%d", wl.loadedWorkflowCount(), wl.displayedWorkflowCount())
	}
	wl.filterText = "wf-1"
	wl.applyProfileTitle()
	if got := paneTitle(wl.workflowsPanel); got != "" {
		t.Fatalf("filter should not live on the pane, got %q", got)
	}
	if wl.workflowTab != nil && strings.Contains(wl.workflowTab.Name, "wf-1") {
		t.Fatalf("filter should not live on the workflows tab, got %q", wl.workflowTab.Name)
	}
}

func paneTitle(panel interface{ Draw(tcell.Screen) }) string {
	if panel == nil {
		return ""
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		return ""
	}
	screen.SetSize(40, 8)
	if setter, ok := panel.(interface{ SetRect(int, int, int, int) }); ok {
		setter.SetRect(0, 0, 40, 8)
	}
	panel.Draw(screen)
	var b strings.Builder
	for x := 0; x < 40; x++ {
		ch, _, _, _ := screen.GetContent(x, 0)
		if ch != 0 && ch != ' ' && ch != '─' && ch != '╭' && ch != '╮' && ch != '┌' && ch != '┐' {
			b.WriteRune(ch)
		}
	}
	return strings.TrimSpace(b.String())
}

func TestWorkflowListTitleListMode(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.workflowTab == nil || wl.workflowTab.Name != "Workflows" {
		t.Fatalf("tree title: %+v", wl.workflowTab)
	}
	wl.toggleWorkflowTree()
	if wl.workflowTab.Name != "Workflows" {
		t.Fatalf("list title: %q", wl.workflowTab.Name)
	}
	if workflowTreeChromeText(wl.workflowTreeMode) != theme.IconList {
		t.Fatal("list mode should use the list glyph")
	}
	wl.toggleWorkflowTree()
	if wl.workflowTab.Name != "Workflows" {
		t.Fatalf("restored tree title: %q", wl.workflowTab.Name)
	}
	if workflowTreeChromeText(wl.workflowTreeMode) != theme.IconNamespace {
		t.Fatal("tree mode should use the tree glyph")
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
	if hintDescription(wl.Hints(), "a") != "Auto-refresh" {
		t.Fatalf("task queues should hint auto-refresh, got %q", hintDescription(wl.Hints(), "a"))
	}

	// Escape from the queue list is not a tab switch: it falls through to the
	// app's own back navigation, leaving the tab where it was.
	if wl.HandleEscape() {
		t.Fatal("escape on the list should not be handled by the view")
	}
	if !wl.taskQueuesActive() {
		t.Fatal("escape should leave the task queues tab open")
	}
}

func TestEmptyWorkflowsStillSwitchListTabs(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.Start()
	wl.allWorkflows = nil
	wl.workflows = nil
	wl.populateTable()

	if wl.workflowTab == nil || wl.workflowTab.Content != wl.workflowStack {
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

	if wl.HandleEscape() || !wl.schedulesActive() {
		t.Fatal("escape on the schedule list should not switch tabs")
	}
	wl.setListKind(listWorkflows)

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '4', 0)) {
		t.Fatal("4 should switch to workers")
	}
	if !wl.workersActive() {
		t.Fatal("workers tab should be active")
	}
	if wl.Name() != "workers" {
		t.Fatalf("workers name: %q", wl.Name())
	}
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("workers should show a detail pane, got %d", wl.mainFlex.GetItemCount())
	}
	if hintDescription(wl.Hints(), "/") != "Search" {
		t.Fatalf("workers hints: %q", hintDescription(wl.Hints(), "/"))
	}
	if hintDescription(wl.Hints(), "Enter") != "Detail" {
		t.Fatalf("workers hint: %q", hintDescription(wl.Hints(), "Enter"))
	}
	if desc := hintDescription(wl.Hints(), "space"); desc != "" {
		t.Fatalf("a flat worker list has nothing to collapse, got %q", desc)
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

func TestWorkerDetailPaneHidesOnEscape(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.setListKind(listWorkers)
	if !wl.workerDetailVisible || wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("workers should open with a detail pane, visible=%v panes=%d", wl.workerDetailVisible, wl.mainFlex.GetItemCount())
	}

	wl.setFocusPane(focusWorkerDetail)
	if !wl.HandleEscape() || wl.workerDetailVisible || wl.focusPane != focusWorkflows || !wl.workersActive() {
		t.Fatal("escape from the worker detail should hide the pane and return to the worker list")
	}
	if wl.mainFlex.GetItemCount() != 1 {
		t.Fatalf("hidden detail should leave the worker list full width, got %d panes", wl.mainFlex.GetItemCount())
	}
	if order := wl.previewFocusOrder(); len(order) != 1 || order[0] != focusWorkflows {
		t.Fatalf("hidden detail should drop out of the focus cycle, got %v", order)
	}

	wl.setListKind(listWorkflows)
	wl.setListKind(listWorkers)
	if wl.workerDetailVisible || wl.mainFlex.GetItemCount() != 1 {
		t.Fatalf("hidden detail should stay hidden after a tab switch, visible=%v panes=%d", wl.workerDetailVisible, wl.mainFlex.GetItemCount())
	}

	wl.setWorkerDetailVisible(true)
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("detail should reopen beside the worker list, got %d panes", wl.mainFlex.GetItemCount())
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

func TestEscapeKeepsTheOpenTab(t *testing.T) {
	cases := []struct {
		name    string
		kind    listKind
		active  func(*WorkflowList) bool
		sidebar workflowFocusPane
		closed  func(*WorkflowList) bool
	}{
		{
			name:    "task queues",
			kind:    listTaskQueues,
			active:  func(wl *WorkflowList) bool { return wl.taskQueuesActive() },
			sidebar: focusPollers,
			closed:  func(wl *WorkflowList) bool { return !wl.pollersVisible },
		},
		{
			name:    "schedules",
			kind:    listSchedules,
			active:  func(wl *WorkflowList) bool { return wl.schedulesActive() },
			sidebar: focusScheduleDetail,
			closed:  func(wl *WorkflowList) bool { return !wl.scheduleDetailVisible },
		},
		{
			name:    "workers",
			kind:    listWorkers,
			active:  func(wl *WorkflowList) bool { return wl.workersActive() },
			sidebar: focusWorkerDetail,
			closed:  func(wl *WorkflowList) bool { return !wl.workerDetailVisible },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wl := NewWorkflowList(&App{}, "default")
			wl.setListKind(tc.kind)

			// Sidebar first: escape closes it and stays on the tab.
			wl.setFocusPane(tc.sidebar)
			if !wl.HandleEscape() || !tc.closed(wl) {
				t.Fatal("escape should close the sidebar")
			}
			if !tc.active(wl) {
				t.Fatal("closing the sidebar should not switch tabs")
			}

			// From the list itself, escape is the app's business, and the tab stays.
			if wl.HandleEscape() {
				t.Fatal("escape on the list should fall through to the app")
			}
			if !tc.active(wl) {
				t.Fatal("escape on the list should not switch tabs")
			}
			if wl.workflowsActive() {
				t.Fatal("escape should not reset the tab to workflows")
			}
		})
	}
}

func TestEscapeStillClearsWorkflowFilters(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.filterText = "orders"
	if !wl.HandleEscape() {
		t.Fatal("escape should clear a filter on the workflows tab")
	}
	if wl.filterText != "" {
		t.Fatalf("filter should be cleared, got %q", wl.filterText)
	}

	// A workflow filter is not the schedule tab's business.
	wl.filterText = "orders"
	wl.setListKind(listSchedules)
	if wl.HandleEscape() {
		t.Fatal("escape on another tab should not reach the workflow filters")
	}
	if wl.filterText != "orders" {
		t.Fatal("the workflow filter should be left alone")
	}
}
