package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func maximizeTestList(t *testing.T) *WorkflowList {
	t.Helper()
	wl := NewWorkflowList(&App{}, "default")
	wl.togglePreviewMode()
	if !wl.previewModeEnabled() {
		t.Fatal("preview should be open")
	}
	return wl
}

func pressM(wl *WorkflowList) *tcell.EventKey {
	return wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, 'm', tcell.ModNone))
}

// onlyItem reports the one thing the list lays out, or nil when it holds
// more. The normal layout without a timeline is just the main flex.
func onlyItem(wl *WorkflowList) tview.Primitive {
	if wl.GetItemCount() != 1 {
		return nil
	}
	return wl.GetItem(0)
}

func TestMMaximizesTheFocusedPreviewPane(t *testing.T) {
	for _, tc := range []struct {
		focus workflowFocusPane
		pane  previewPane
		frame func(*WorkflowList) *paneFrame
	}{
		{focusEvents, previewPaneSecondary, func(wl *WorkflowList) *paneFrame { return wl.previewPanel }},
		{focusEventDetail, previewPaneTertiary, func(wl *WorkflowList) *paneFrame { return wl.eventDetailPanel }},
	} {
		wl := maximizeTestList(t)
		wl.timelineVisible = true
		wl.applyMainLayout()
		wl.focusPane = tc.focus

		if ev := pressM(wl); ev != nil {
			t.Fatalf("m should be handled in pane %d", tc.focus)
		}
		frame := tc.frame(wl)
		if wl.maximizedPane != tc.pane || onlyItem(wl) != tview.Primitive(frame) || !frame.isMaximized() {
			t.Fatalf("pane %d should fill the view, items=%d", tc.focus, wl.GetItemCount())
		}
		if wl.focusPane != tc.focus {
			t.Fatalf("the maximized pane should keep focus, got %d", wl.focusPane)
		}
		hints := wl.Hints()
		if hintDescription(hints, "m") != "Minimize" {
			t.Fatalf("hints should offer to minimize, got %+v", hints)
		}

		pressM(wl)
		if wl.maximizedPane != previewPaneNone || onlyItem(wl) != nil || frame.isMaximized() {
			t.Fatalf("m again should put the layout back, items=%d", wl.GetItemCount())
		}
		if hintDescription(wl.Hints(), "m") != "Maximize" {
			t.Fatal("hints should offer to maximize")
		}
	}
}

func TestMovingFocusOrEscapeRestoresAMaximizedPane(t *testing.T) {
	wl := maximizeTestList(t)
	wl.focusPane = focusEvents
	pressM(wl)
	wl.cycleFocus(1)
	if wl.maximizedPane != previewPaneNone || onlyItem(wl) == tview.Primitive(wl.previewPanel) {
		t.Fatal("tab to another pane should put the layout back")
	}
	if wl.focusPane != focusEventDetail {
		t.Fatalf("focus should move on, got %d", wl.focusPane)
	}

	pressM(wl)
	if !wl.HandleEscape() || wl.maximizedPane != previewPaneNone || !wl.previewModeEnabled() {
		t.Fatal("the first escape should only put the layout back")
	}
	if !wl.HandleEscape() || wl.previewModeEnabled() {
		t.Fatal("the next escape should close the preview")
	}
}

func TestClosingThePreviewForgetsAMaximizedPane(t *testing.T) {
	wl := maximizeTestList(t)
	wl.focusPane = focusEventDetail
	pressM(wl)
	wl.togglePreviewMode()
	if onlyItem(wl) == tview.Primitive(wl.eventDetailPanel) {
		t.Fatal("a closed preview should not stay maximized")
	}
	wl.togglePreviewMode()
	if wl.maximizedPane != previewPaneNone || onlyItem(wl) != tview.Primitive(wl.mainFlex) {
		t.Fatal("reopening the preview should show the normal layout")
	}
}

func TestPaneFrameSizeButton(t *testing.T) {
	maximized := false
	frame := newPaneFrame(func() bool { return maximized }, func() { maximized = !maximized })
	frame.SetRect(0, 0, 40, 10)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 10)

	label := func() string {
		frame.Draw(screen)
		x, y := paneSizeButtonOrigin(frame)
		out := ""
		for i := 0; i < paneSizeButton; i++ {
			r, _, _, _ := screen.GetContent(x+i, y)
			out += string(r)
		}
		return out
	}
	if got := label(); got != "[+]" {
		t.Fatalf("button=%q", got)
	}
	x, y := paneSizeButtonOrigin(frame)
	click := tcell.NewEventMouse(x+1, y, tcell.Button1, tcell.ModNone)
	if handled, _ := frame.MouseHandler()(tview.MouseLeftClick, click, func(tview.Primitive) {}); !handled || !maximized {
		t.Fatal("clicking the button should toggle the pane")
	}
	if got := label(); got != "[-]" {
		t.Fatalf("button=%q", got)
	}
}
