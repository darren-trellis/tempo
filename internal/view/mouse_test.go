package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestIgnoreMouseFocusOnlyConsumesInside(t *testing.T) {
	box := tview.NewBox()
	box.SetRect(0, 0, 10, 2)
	ignoreMouseFocus(box)
	capture := box.GetMouseCapture()
	if capture == nil {
		t.Fatal("expected mouse capture")
	}

	action, event := capture(tview.MouseLeftClick, tcell.NewEventMouse(5, 1, tcell.Button1, tcell.ModNone))
	if action != tview.MouseConsumed || event != nil {
		t.Fatal("click on chrome should be consumed")
	}

	action, event = capture(tview.MouseLeftClick, tcell.NewEventMouse(50, 50, tcell.Button1, tcell.ModNone))
	if event == nil || action != tview.MouseLeftClick {
		t.Fatal("click outside chrome should reach the table")
	}
}

func TestEnableAppMouse(t *testing.T) {
	app := tview.NewApplication()
	box := tview.NewBox()
	enableAppMouse(app, box)
	if app.GetMouseCapture() != nil {
		t.Fatal("enableAppMouse should not install an app-level mouse capture")
	}
}

func TestWorkflowListMouseHandlerOutsideRect(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	handler := wl.MouseHandler()
	if handler == nil {
		t.Fatal("expected mouse handler")
	}
	event := tcell.NewEventMouse(9999, 9999, tcell.Button1, tcell.ModNone)
	consumed, capture := handler(tview.MouseLeftClick, event, func(tview.Primitive) {})
	if consumed || capture != nil {
		t.Fatalf("click outside layout should be ignored, consumed=%v capture=%v", consumed, capture)
	}
}

func TestBindTableDoubleClick(t *testing.T) {
	table := components.NewTable()
	selected := false
	table.SetOnSelect(func(row int) {
		selected = true
	})
	table.SetHeaders("ID")
	table.AddRow("one")
	table.SelectRow(0)
	bindTableDoubleClick(table)

	handler := table.MouseHandler()
	event := tcell.NewEventMouse(0, 0, tcell.Button1, tcell.ModNone)
	consumed, _ := handler(tview.MouseLeftDoubleClick, event, func(tview.Primitive) {})
	if !consumed {
		t.Fatal("double-click should be consumed")
	}
	if !selected {
		t.Fatal("double-click should activate the selected row")
	}
}
