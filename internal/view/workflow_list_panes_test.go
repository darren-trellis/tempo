package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func paneResizeEvent(key tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(key, 0, tcell.ModShift|tcell.ModAlt)
}

func TestFocusedListPaneRoles(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	cases := []struct {
		pane workflowFocusPane
		role listPane
	}{
		{focusWorkflows, panePrimary},
		{focusEvents, paneSecondary},
		{focusEventDetail, paneTertiary},
		{focusPollers, paneSecondary},
		{focusScheduleDetail, paneSecondary},
		{focusScheduleRuns, paneTertiary},
		{focusWorkerDetail, paneSecondary},
		{focusTimeline, panePrimary},
	}
	for _, tc := range cases {
		wl.focusPane = tc.pane
		if got := wl.focusedListPane(); got != tc.role {
			t.Fatalf("pane %d: role=%d want %d", tc.pane, got, tc.role)
		}
	}
}

func TestPaneResizeKeyRequiresShiftOption(t *testing.T) {
	if _, ok := paneResizeArrow(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)); ok {
		t.Fatal("plain arrow should not resize")
	}
	if _, ok := paneResizeArrow(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModShift)); ok {
		t.Fatal("shift alone should not resize")
	}
	if _, ok := paneResizeArrow(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt)); ok {
		t.Fatal("option alone should not resize")
	}
	if key, ok := paneResizeArrow(paneResizeEvent(tcell.KeyUp)); !ok || key != tcell.KeyUp {
		t.Fatal("shift+option+arrow should resize")
	}
	if key, ok := paneResizeArrow(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModShift|tcell.ModMeta)); !ok || key != tcell.KeyLeft {
		t.Fatal("shift+meta+arrow should resize")
	}
}

func TestSecondaryUpShrinksPreviewAndGrowsTertiary(t *testing.T) {
	wl := previewResizeList(t)
	wl.focusPane = focusEvents
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyUp)) {
		t.Fatal("secondary shift+option+up should resize")
	}
	if wl.tertiaryHeight != 17 {
		t.Fatalf("secondary+up should grow tertiary by 1, got %d", wl.tertiaryHeight)
	}

	wl.focusPane = focusEventDetail
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyUp)) {
		t.Fatal("tertiary shift+option+up should resize")
	}
	if wl.tertiaryHeight != 18 {
		t.Fatalf("tertiary+up should grow tertiary by 1, got %d", wl.tertiaryHeight)
	}

	wl.focusPane = focusEvents
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyDown)) {
		t.Fatal("secondary shift+option+down should resize")
	}
	if wl.tertiaryHeight != 17 {
		t.Fatalf("secondary+down should shrink tertiary by 1, got %d", wl.tertiaryHeight)
	}
}

func TestPrimaryAndSecondaryArrowsMoveTheSameDivider(t *testing.T) {
	wl := previewResizeList(t)
	wl.focusPane = focusWorkflows
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyLeft)) {
		t.Fatal("primary shift+option+left should resize")
	}
	if wl.primaryWidth != 54 {
		t.Fatalf("primary+left should shrink primary by 1, got %d", wl.primaryWidth)
	}

	wl.focusPane = focusEvents
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyLeft)) {
		t.Fatal("secondary shift+option+left should resize")
	}
	if wl.primaryWidth != 53 {
		t.Fatalf("secondary+left should keep moving the same divider, got %d", wl.primaryWidth)
	}

	wl.focusPane = focusEventDetail
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyRight)) {
		t.Fatal("tertiary shift+option+right should resize")
	}
	if wl.primaryWidth != 54 {
		t.Fatalf("tertiary+right should grow primary by 1, got %d", wl.primaryWidth)
	}
}

func TestPaneResizeSurvivesLayoutRebuild(t *testing.T) {
	wl := previewResizeList(t)
	wl.focusPane = focusEvents
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyUp)) {
		t.Fatal("expected resize")
	}
	if !wl.handlePaneResizeKey(paneResizeEvent(tcell.KeyLeft)) {
		t.Fatal("expected resize")
	}
	wantTertiary, wantPrimary := wl.tertiaryHeight, wl.primaryWidth
	wl.applyPreviewPage()
	wl.applyMainLayout()
	if wl.tertiaryHeight != wantTertiary || wl.primaryWidth != wantPrimary {
		t.Fatalf("layout rebuild dropped sizes: tertiary=%d primary=%d", wl.tertiaryHeight, wl.primaryWidth)
	}
}

func TestPaneResizeIsConsumedOnPreviewAndList(t *testing.T) {
	wl := previewResizeList(t)
	wl.focusPane = focusEvents
	if ev := wl.handlePreviewKeys(paneResizeEvent(tcell.KeyUp)); ev != nil {
		t.Fatal("preview should consume shift+option+up")
	}
	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)); ev == nil {
		t.Fatal("plain up should not be consumed as resize")
	}
	wl.focusPane = focusWorkflows
	wl.keepDataOnStart = true
	wl.Start()
	if ev := wl.table.GetInputCapture()(paneResizeEvent(tcell.KeyRight)); ev != nil {
		t.Fatal("primary pane should consume shift+option+right")
	}
}

func previewResizeList(t *testing.T) *WorkflowList {
	t.Helper()
	wl := NewWorkflowList(&App{}, "default")
	wl.previewMode = true
	wl.previewKind = previewActivities
	wl.applyMainLayout()
	wl.applyPreviewPage()
	wl.mainFlex.SetRect(0, 0, 100, 40)
	wl.workflowsPanel.SetRect(0, 0, 55, 40)
	wl.rightFlex.SetRect(55, 0, 45, 40)
	wl.previewPanel.SetRect(55, 0, 45, 24)
	wl.eventDetailPanel.SetRect(55, 24, 45, 16)
	return wl
}
