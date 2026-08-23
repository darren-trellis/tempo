package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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
