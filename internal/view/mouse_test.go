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

func TestRouteModalMouseIgnoresNormalPages(t *testing.T) {
	event := tcell.NewEventMouse(5, 5, tcell.Button1, tcell.ModNone)
	if routeModalMouse(NewWorkflowList(&App{}, "default"), tview.MouseLeftClick, event, func(tview.Primitive) {}) {
		t.Fatal("list clicks should not be treated as modal")
	}
}

func TestRouteModalMouseConsumesBackdrop(t *testing.T) {
	modal := newModal(components.ModalConfig{Width: 20, Height: 10, Backdrop: true})
	modal.SetRect(0, 0, 80, 24)
	modal.GetPanel().SetRect(30, 7, 20, 10)

	event := tcell.NewEventMouse(0, 0, tcell.Button1, tcell.ModNone)
	if !routeModalMouse(modal, tview.MouseLeftClick, event, func(tview.Primitive) {}) {
		t.Fatal("backdrop click should be consumed")
	}
}

func TestRouteModalMouseFocusesPanel(t *testing.T) {
	modal := newModal(components.ModalConfig{Width: 20, Height: 10, Backdrop: true})
	modal.SetRect(0, 0, 80, 24)
	modal.GetPanel().SetRect(30, 7, 20, 10)

	focused := false
	event := tcell.NewEventMouse(35, 10, tcell.Button1, tcell.ModNone)
	if !routeModalMouse(modal, tview.MouseLeftDown, event, func(tview.Primitive) { focused = true }) {
		t.Fatal("click on modal panel should be consumed")
	}
	if !focused {
		t.Fatal("click on modal panel should focus the modal")
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

func TestHorizontalMouseDelta(t *testing.T) {
	if got := horizontalMouseDelta(tview.MouseScrollLeft, nil); got != -1 {
		t.Fatalf("left=%d", got)
	}
	if got := horizontalMouseDelta(tview.MouseScrollRight, nil); got != 1 {
		t.Fatalf("right=%d", got)
	}
	if got := horizontalMouseDelta(tview.MouseScrollUp, tcell.NewEventMouse(0, 0, tcell.WheelUp, tcell.ModNone)); got != 0 {
		t.Fatalf("vertical up should stay vertical, got %d", got)
	}
	if got := horizontalMouseDelta(tview.MouseScrollDown, tcell.NewEventMouse(0, 0, tcell.WheelDown, tcell.ModShift)); got != 1 {
		t.Fatalf("shift+down should scroll right, got %d", got)
	}
}

func TestScrollTableColumns(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("A", "B", "C")
	table.AddRow("1", "2", "3")
	table.SetOffset(0, 1)
	scrollTableColumns(table, -1)
	if _, col := table.GetOffset(); col != 0 {
		t.Fatalf("col=%d", col)
	}
	scrollTableColumns(table, -1)
	if _, col := table.GetOffset(); col != 0 {
		t.Fatalf("should clamp left, col=%d", col)
	}
	scrollTableColumns(table, 10)
	if _, col := table.GetOffset(); col != 2 {
		t.Fatalf("should clamp to last column, col=%d", col)
	}
}

func TestBindTableHorizontalScroll(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("A", "B", "C")
	table.AddRow("1", "2", "3")
	bindTableHorizontalScroll(table)
	handler := table.MouseHandler()
	event := tcell.NewEventMouse(0, 1, tcell.WheelRight, tcell.ModNone)
	consumed, _ := handler(tview.MouseScrollRight, event, func(tview.Primitive) {})
	if !consumed {
		t.Fatal("horizontal wheel should be consumed")
	}
	if _, col := table.GetOffset(); col != 1 {
		t.Fatalf("right scroll col=%d", col)
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
