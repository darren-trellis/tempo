package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestWorkflowColumnOffsets(t *testing.T) {
	cols := []workflowColumn{{width: 10}, {width: 5}, {width: 8}}
	if got := workflowTableContentWidth(cols); got != 25 {
		t.Fatalf("width=%d", got)
	}
	offs := workflowColumnOffsets(cols)
	if len(offs) != 3 || offs[0] != 0 || offs[1] != 11 || offs[2] != 17 {
		t.Fatalf("offs=%v", offs)
	}
	if got := scrollOffsetByColumn(0, cols, -1); got != 0 {
		t.Fatalf("left of first=%d", got)
	}
	if got := scrollOffsetByColumn(3, cols, -1); got != 0 {
		t.Fatalf("snap to column start=%d", got)
	}
	if got := scrollOffsetByColumn(0, cols, 1); got != 11 {
		t.Fatalf("next column=%d", got)
	}
	if got := scrollOffsetByColumn(11, cols, 1); got != 17 {
		t.Fatalf("last column=%d", got)
	}
}

func TestCharScrollViewStepsAndClamps(t *testing.T) {
	box := tview.NewBox()
	view := newCharScrollView(box, func() int { return 40 })
	view.SetRect(0, 0, 10, 5)
	view.scrollChars(3)
	if view.offset != 3 {
		t.Fatalf("offset=%d", view.offset)
	}
	view.scrollChars(-100)
	if view.offset != 0 {
		t.Fatalf("left clamp=%d", view.offset)
	}
	view.scrollTo(100)
	if view.offset != 30 {
		t.Fatalf("right clamp=%d", view.offset)
	}
}

func TestBindTableCharScroll(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("A", "B")
	table.AddRow("1", "2")
	view := newCharScrollView(table, func() int { return 20 })
	view.SetRect(0, 0, 10, 5)
	bindTableCharScroll(table, view)

	handler := table.MouseHandler()
	event := tcell.NewEventMouse(1, 1, tcell.WheelRight, tcell.ModNone)
	consumed, _ := handler(tview.MouseScrollRight, event, func(tview.Primitive) {})
	if !consumed {
		t.Fatal("horizontal wheel should be consumed")
	}
	if view.offset != 1 {
		t.Fatalf("mouse should scroll one character, offset=%d", view.offset)
	}
}
