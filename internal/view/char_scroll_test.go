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
	bindTableCharScroll(table, view, nil)

	handler := table.MouseHandler()
	event := tcell.NewEventMouse(1, 1, tcell.WheelRight, tcell.ModNone)
	consumed, _ := handler(tview.MouseScrollRight, event, func(tview.Primitive) {})
	if !consumed {
		t.Fatal("horizontal wheel should be consumed")
	}
	if view.offset != 1 {
		t.Fatalf("mouse should scroll one character, offset=%d", view.offset)
	}

	view.offset = 0
	bindTableCharScroll(table, view, func() int { return 3 })
	consumed, _ = handler(tview.MouseScrollRight, event, func(tview.Primitive) {})
	if !consumed || view.offset != 3 {
		t.Fatalf("configured step should scroll 3 characters, consumed=%v offset=%d", consumed, view.offset)
	}
}

// TestWheelAtABoundaryIsDropped is the bug: tview consumes every wheel tick and
// only clamps when it draws, so flicking against a boundary queued a full
// redraw per tick and the next gesture waited behind them.
func TestWheelAtABoundaryIsDropped(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("A", "B")
	for i := 0; i < 40; i++ {
		table.AddRow("1", "2")
	}
	table.SetRect(0, 0, 10, 6)
	view := newCharScrollView(table, func() int { return 20 })
	view.SetRect(0, 0, 10, 6)
	bindTableCharScroll(table, view, nil)
	capture := table.GetMouseCapture()

	up := tcell.NewEventMouse(1, 1, tcell.WheelUp, tcell.ModNone)
	action, event := capture(tview.MouseScrollUp, up)
	if action == tview.MouseConsumed || event != nil {
		t.Fatal("scrolling up at the top should be dropped, not redrawn")
	}

	// Once scrolled away from the top, the wheel reaches the table again.
	table.Select(20, 0)
	table.Draw(tcell.NewSimulationScreen("UTF-8"))
	if offset, _ := table.GetOffset(); offset == 0 {
		t.Fatal("test setup should scroll the table away from the top")
	}
	if _, event := capture(tview.MouseScrollUp, up); event == nil {
		t.Fatal("scrolling up mid-list should reach the table")
	}

	// The same applies horizontally once the pan is against the right edge.
	view.scrollTo(view.maxOffset())
	right := tcell.NewEventMouse(1, 1, tcell.WheelRight, tcell.ModNone)
	action, event = capture(tview.MouseScrollRight, right)
	if action == tview.MouseConsumed || event != nil {
		t.Fatal("panning past the right edge should be dropped")
	}
}

func TestVerticalMouseDeltaIgnoresShift(t *testing.T) {
	if got := verticalMouseDelta(tview.MouseScrollUp, nil); got != -1 {
		t.Fatalf("up=%d", got)
	}
	if got := verticalMouseDelta(tview.MouseScrollDown, nil); got != 1 {
		t.Fatalf("down=%d", got)
	}
	// Shift turns the wheel into a horizontal gesture, so it is not vertical.
	shift := tcell.NewEventMouse(0, 0, tcell.WheelDown, tcell.ModShift)
	if got := verticalMouseDelta(tview.MouseScrollDown, shift); got != 0 {
		t.Fatalf("shift+down should not be vertical, got %d", got)
	}
	if got := verticalMouseDelta(tview.MouseScrollRight, nil); got != 0 {
		t.Fatalf("horizontal wheel should not be vertical, got %d", got)
	}
}

func TestTableColumnLayoutMeasuresCells(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("ID", "NAME")
	table.AddRow("1", "a-long-workflow-name")

	cols := tableColumnLayout(table)
	if len(cols) != 2 {
		t.Fatalf("columns: %d", len(cols))
	}
	if cols[0].width != 2 || cols[1].width != len("a-long-workflow-name") {
		t.Fatalf("widths: %+v", cols)
	}
	// Columns render one space apart.
	if got := tableContentWidth(table); got != 2+1+len("a-long-workflow-name") {
		t.Fatalf("content width: %d", got)
	}
	if got := tableContentWidth(nil); got != 0 {
		t.Fatalf("nil table: %d", got)
	}
}

func TestHandleTableCharScrollKeys(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("ID", "NAME")
	table.AddRow("1", "a-long-workflow-name")
	view := attachTableCharScroll(table, nil)
	view.SetRect(0, 0, 8, 5)

	if !handleTableCharScroll(view, table, tcell.NewEventKey(tcell.KeyRune, 'l', 0)) || view.offset != 1 {
		t.Fatalf("l should scroll one char, offset=%d", view.offset)
	}
	if !handleTableCharScroll(view, table, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)) || view.offset != 2 {
		t.Fatalf("right should scroll one char, offset=%d", view.offset)
	}
	if !handleTableCharScroll(view, table, tcell.NewEventKey(tcell.KeyRune, 'h', 0)) || view.offset != 1 {
		t.Fatalf("h should scroll back one char, offset=%d", view.offset)
	}
	if !handleTableCharScroll(view, table, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) || view.offset != 0 {
		t.Fatalf("left should scroll back one char, offset=%d", view.offset)
	}

	// Home/End jump a whole column: the second column starts after "ID" plus a space.
	if !handleTableCharScroll(view, table, tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)) || view.offset != 3 {
		t.Fatalf("end should jump to the next column, offset=%d", view.offset)
	}
	if !handleTableCharScroll(view, table, tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)) || view.offset != 0 {
		t.Fatalf("home should jump back a column, offset=%d", view.offset)
	}

	if handleTableCharScroll(view, table, tcell.NewEventKey(tcell.KeyRune, 'j', 0)) {
		t.Fatal("unrelated keys should fall through")
	}
	if handleTableCharScroll(nil, table, tcell.NewEventKey(tcell.KeyRune, 'l', 0)) {
		t.Fatal("a view-less table should fall through")
	}
}
