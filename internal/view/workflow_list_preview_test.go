package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestSelectModeKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.Start()
	wl.loadMockData()

	if desc := hintDescription(wl.Hints(), "a"); desc != "Auto-refresh" {
		t.Fatalf("a should toggle auto-refresh, got %q", desc)
	}

	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'v', 0)); ev != nil {
		t.Fatal("v should enter select mode")
	}
	if !wl.selectionMode {
		t.Fatal("v should enable select mode")
	}
	if desc := hintDescription(wl.Hints(), "a"); desc != "Select All" {
		t.Fatalf("select mode a: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "Ctrl+A"); desc != "" {
		t.Fatalf("ctrl+a should not be hinted, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "v"); desc != "" {
		t.Fatalf("exit select should not use v, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "esc"); desc != "" {
		t.Fatalf("esc should stay off the footer, got %q", desc)
	}

	if desc := hintDescription(wl.Hints(), "d"); desc != "" {
		t.Fatalf("delete should stay hidden until a row is selected, got %q", desc)
	}

	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'a', 0)); ev != nil {
		t.Fatal("a should select all")
	}
	if wl.autoRefresh {
		t.Fatal("a should not toggle auto-refresh in select mode")
	}
	if desc := hintDescription(wl.Hints(), "d"); desc != "Delete" {
		t.Fatalf("select mode d: %q", desc)
	}

	if !wl.HandleEscape() || wl.selectionMode {
		t.Fatal("esc should exit select mode")
	}
}

func TestSelectModeSpaceAdvancesRow(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.Start()
	wl.loadMockData()
	wl.table.SelectRow(0)

	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'v', 0)); ev != nil {
		t.Fatal("v should enter select mode")
	}
	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ' ', 0)); ev != nil {
		t.Fatal("space should select the current row")
	}
	if len(wl.table.GetSelectedRows()) != 1 {
		t.Fatal("space should select the row that was highlighted")
	}
	if wl.table.SelectedRow() != 1 {
		t.Fatalf("space should move highlight to the next row, got %d", wl.table.SelectedRow())
	}

	last := wl.table.RowCount() - 1
	wl.table.SelectRow(last)
	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ' ', 0)); ev != nil {
		t.Fatal("space should select the last row")
	}
	if wl.table.SelectedRow() != last {
		t.Fatalf("space on the last row should stay put, got %d", wl.table.SelectedRow())
	}
}

func TestSelectModeDelete(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.Start()
	wl.loadMockData()
	wl.table.SelectRow(0)

	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'v', 0)); ev != nil {
		t.Fatal("v should enter select mode")
	}
	if desc := hintDescription(wl.Hints(), "d"); desc != "" {
		t.Fatalf("delete should stay hidden until a row is selected, got %q", desc)
	}

	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ' ', 0)); ev != nil {
		t.Fatal("space should select the current row")
	}
	if desc := hintDescription(wl.Hints(), "d"); desc != "Delete" {
		t.Fatalf("select mode d: %q", desc)
	}

	got := wl.selectedWorkflowIndices()
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("selected indices: %v, want [0]", got)
	}

	if ev := wl.table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'd', 0)); ev != nil {
		t.Fatal("d should open delete confirm in select mode")
	}
}

func TestNewWorkflowListDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewWorkflowList panicked: %v", r)
		}
	}()
	NewWorkflowList(&App{}, "default")
}

func TestWorkflowTableKeepsHorizontalScrollKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()

	if wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) {
		t.Fatal("left should not switch preview tabs from the workflows table")
	}
	if wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyRune, 'h', 0)) {
		t.Fatal("h should not switch preview tabs from the workflows table")
	}

	wl.tableScroll.SetRect(0, 0, 20, 10)
	wl.tableScroll.scrollTo(4)
	if !wl.handleWorkflowScroll(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) {
		t.Fatal("left should scroll the workflows table")
	}
	if wl.tableScroll.offset != 3 {
		t.Fatalf("left should scroll one character, offset=%d", wl.tableScroll.offset)
	}
	if !wl.handleWorkflowScroll(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)) {
		t.Fatal("end should scroll by column")
	}
	if wl.tableScroll.offset <= 3 {
		t.Fatalf("end should jump to the next column, offset=%d", wl.tableScroll.offset)
	}

	if wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should not switch preview tabs from the main window")
	}
	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should switch list tabs from the main window")
	}
	if !wl.taskQueuesActive() {
		t.Fatal("] should open task queues from the main window")
	}
	wl.setListKind(listWorkflows)
	if desc := hintDescription(wl.Hints(), "h/l"); desc != "" {
		t.Fatalf("obvious scroll keys should stay off the footer, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "[/]/1-2"); desc != "" {
		t.Fatalf("list tab keys should stay off the footer, got %q", desc)
	}

	wl.focusPane = focusEvents
	if desc := hintDescription(wl.Hints(), "[/]/1-3"); desc != "" {
		t.Fatalf("preview tab keys should stay off the footer, got %q", desc)
	}
	if !wl.handlePreviewTabKey(tcell.NewEventKey(tcell.KeyRune, ']', 0)) {
		t.Fatal("] should switch preview tabs when preview is focused")
	}
	if wl.previewKind != previewEvents {
		t.Fatalf("] should go to events, got %d", wl.previewKind)
	}
}

func TestPreviewHierarchyTab(t *testing.T) {
	if previewHierarchy.title() != "Hierarchy" {
		t.Fatalf("hierarchy title: %q", previewHierarchy.title())
	}

	wl := NewWorkflowList(&App{}, "default")
	wl.showWorkflowGraph()
	if !wl.previewModeEnabled() {
		t.Fatal("o should enable preview")
	}
	if wl.previewKind != previewHierarchy {
		t.Fatalf("o should open hierarchy, got %d", wl.previewKind)
	}
	if wl.rightFlex.GetItemCount() != 2 {
		t.Fatalf("hierarchy should show the graph pane, got %d items", wl.rightFlex.GetItemCount())
	}
	if wl.focusPane != focusEvents {
		t.Fatalf("o should focus the hierarchy tree, got %d", wl.focusPane)
	}
	if desc := hintDescription(wl.Hints(), "space"); desc != "Collapse/Expand" {
		t.Fatalf("hierarchy hints: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "c"); desc != "" {
		t.Fatalf("tree pane should not show center graph, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "i"); desc != "" {
		t.Fatalf("hierarchy should not show io, got %q", desc)
	}

	wl.setFocusPane(focusEventDetail)
	if desc := hintDescription(wl.Hints(), "c"); desc != "Center Graph" {
		t.Fatalf("graph pane should show center, got %q", desc)
	}

	wl.hierarchyView.loading = true
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, ']', 0)); ev != nil {
		t.Fatal("] should switch tabs while hierarchy is loading")
	}
	if wl.previewKind != previewDetails {
		t.Fatalf("] should leave hierarchy while loading, got %d", wl.previewKind)
	}

	wl.setPreviewKind(previewActivities)
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'o', 0)); ev != nil {
		t.Fatal("o should switch preview to hierarchy")
	}
	if wl.previewKind != previewHierarchy {
		t.Fatalf("o should switch to hierarchy, got %d", wl.previewKind)
	}
}

func TestPreviewHintsArePaneSpecific(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if desc := hintDescription(wl.Hints(), "i"); desc != "Input/Output" {
		t.Fatalf("workflows pane should show io without preview, got %q", desc)
	}

	wl.togglePreviewMode()
	if desc := hintDescription(wl.Hints(), "b"); desc != "Tree" {
		t.Fatalf("workflows pane should show tree, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "tab"); desc != "" {
		t.Fatalf("tab should not be hinted, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "i"); desc != "Input/Output" {
		t.Fatalf("workflows pane should show io, got %q", desc)
	}

	wl.setPreviewKind(previewActivities)
	wl.focusPane = focusEvents
	if desc := hintDescription(wl.Hints(), "b"); desc != "" {
		t.Fatalf("preview should not show tree, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "i"); desc != "Input/Output" {
		t.Fatalf("activities should show io, got %q", desc)
	}

	wl.setPreviewKind(previewDetails)
	wl.focusPane = focusWorkflows
	if desc := hintDescription(wl.Hints(), "i"); desc != "Input/Output" {
		t.Fatalf("workflows pane should keep io on details tab, got %q", desc)
	}

	wl.focusPane = focusEventDetail
	if desc := hintDescription(wl.Hints(), "i"); desc != "" {
		t.Fatalf("details should not show io, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "y"); desc != "Yank" {
		t.Fatalf("details should show yank, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "j/k"); desc != "" {
		t.Fatalf("obvious nav keys should stay off the footer, got %q", desc)
	}
}

func TestPreviewModeLayout(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.GetItemCount() != 1 {
		t.Fatalf("default layout should be workflows only, got %d items", wl.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "" {
		t.Fatalf("enter should stay off the footer, got %q", desc)
	}

	wl.togglePreviewMode()
	if wl.mainFlex.GetItemCount() != 2 {
		t.Fatalf("preview should show events pane, got %d items", wl.mainFlex.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "" {
		t.Fatalf("preview enter should stay off the footer, got %q", desc)
	}

	wl.togglePreviewMode()
	if wl.GetItemCount() != 1 {
		t.Fatalf("hiding preview should show workflows only, got %d items", wl.GetItemCount())
	}
}

func TestPreviewTextViewPageUpStopsAtTop(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	capture := wl.eventDetail.GetInputCapture()
	if capture == nil {
		t.Fatal("event detail should capture keys")
	}
	wl.eventDetail.SetText(strings.Repeat("line\n", 80))
	wl.eventDetail.ScrollTo(4, 0)

	if ev := capture(tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone)); ev != nil {
		t.Fatal("page up should be consumed")
	}
	if row, _ := wl.eventDetail.GetScrollOffset(); row < 0 {
		t.Fatalf("page up should not leave a negative offset, got %d", row)
	}
}

func TestTimelineToggleLayout(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.GetItemCount() != 1 {
		t.Fatalf("default should hide timeline, got %d items", wl.GetItemCount())
	}
	wl.toggleTimeline()
	if !wl.timelineVisible || wl.GetItemCount() != 2 {
		t.Fatalf("timeline should add a bottom pane, visible=%v items=%d", wl.timelineVisible, wl.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "z"); desc != "Timeline" {
		t.Fatalf("timeline hint: %q", desc)
	}
	wl.toggleTimeline()
	if wl.timelineVisible || wl.GetItemCount() != 1 {
		t.Fatal("toggling again should hide the timeline")
	}
}

func TestTimelineSizeMatchesWorkflowsPane(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.toggleTimeline()
	wl.SetRect(0, 0, 100, 40)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 40)
	wl.Draw(screen)

	_, _, fullW, _ := wl.timelinePanel.GetRect()
	_, _, listW, _ := wl.GetRect()
	if fullW != listW {
		t.Fatalf("maximized timeline width=%d list=%d", fullW, listW)
	}
	if desc := hintDescription(wl.Hints(), "m"); desc != "" && desc != "Minimize" {
		t.Fatalf("maximized hint: %q", desc)
	}

	wl.toggleTimelineSize()
	wl.Draw(screen)
	_, _, tw, _ := wl.timelinePanel.GetRect()
	_, _, ww, _ := wl.workflowsPanel.GetRect()
	if tw != ww {
		t.Fatalf("narrow timeline width=%d workflows=%d", tw, ww)
	}
	if tw >= fullW {
		t.Fatalf("narrow timeline should be narrower than full width, %d >= %d", tw, fullW)
	}
	if wl.GetItemCount() != 1 {
		t.Fatalf("docked timeline should live under workflows, items=%d", wl.GetItemCount())
	}

	wl.focusPane = focusTimeline
	if desc := hintDescription(wl.Hints(), "m"); desc != "Maximize" {
		t.Fatalf("narrow hint: %q", desc)
	}
}

func TestTimelineHighlightsSelectedActivity(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	wl.toggleTimeline()
	wl.showPreviewEvents(temporal.Workflow{ID: "wf", RunID: "run"}, []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: time.Now().Add(-time.Minute)},
		{ID: 5, Type: "ActivityTaskScheduled", Time: time.Now().Add(-50 * time.Second), ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: time.Now().Add(-40 * time.Second), ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: time.Now().Add(-30 * time.Second), ActivityType: "Second"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: time.Now().Add(-20 * time.Second), ScheduledEventID: 8},
	})
	if wl.eventTable.RowCount() < 2 {
		t.Fatalf("expected two activities, got %d", wl.eventTable.RowCount())
	}
	wl.eventTable.SelectRow(1)
	wl.updatePreviewSelection(2)
	if wl.highlightedActivityID != 8 {
		t.Fatalf("highlighted activity=%d", wl.highlightedActivityID)
	}
	wl.setFocusPane(focusTimeline)
	lane := wl.timelineView.SelectedLane()
	if lane == nil || timelineLaneScheduledID(*lane) != 8 {
		t.Fatal("timeline should highlight the selected activity bar")
	}
}

func hintDescription(hints []KeyHint, key string) string {
	for _, h := range hints {
		if h.Key == key {
			return h.Description
		}
	}
	return ""
}
