package view

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestScrollTextViewStopsAtTop(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true)
	view.SetText(strings.Repeat("line\n", 50))
	view.ScrollTo(5, 0)

	scrollTextView(view, -100)
	if row, _ := view.GetScrollOffset(); row != 0 {
		t.Fatalf("scroll should clamp at top, got %d", row)
	}
}

func TestHandleTextViewScrollHorizontal(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true).SetWrap(false)
	view.SetText(strings.Repeat("x", 80))
	view.SetRect(0, 0, 20, 5)

	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)) {
		t.Fatal("right should scroll horizontally")
	}
	if _, col := view.GetScrollOffset(); col != 1 {
		t.Fatalf("right should move one column, col=%d", col)
	}
	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyRune, 'l', 0)) {
		t.Fatal("l should scroll horizontally")
	}
	if _, col := view.GetScrollOffset(); col != 2 {
		t.Fatalf("l should move one column, col=%d", col)
	}
	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) {
		t.Fatal("left should scroll horizontally")
	}
	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyRune, 'h', 0)) {
		t.Fatal("h should scroll horizontally")
	}
	if _, col := view.GetScrollOffset(); col != 0 {
		t.Fatalf("left/h should move back, col=%d", col)
	}

	handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyRune, 'h', 0))
	if _, col := view.GetScrollOffset(); col != 0 {
		t.Fatalf("horizontal scroll should clamp at 0, col=%d", col)
	}
}

func TestHomeAndEndScrollHorizontally(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true).SetWrap(false)
	view.SetText(strings.Repeat("x", 80) + "\n" + strings.Repeat("y", 80))
	view.SetRect(0, 0, 20, 5)
	view.ScrollTo(1, 4)

	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)) {
		t.Fatal("home should scroll")
	}
	row, col := view.GetScrollOffset()
	if row != 1 || col != 0 {
		t.Fatalf("home should return to the first column of the current row, row=%d col=%d", row, col)
	}
	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)) {
		t.Fatal("end should scroll")
	}
	row, col = view.GetScrollOffset()
	if row != 1 || col != 60 {
		t.Fatalf("end should reach the last column, row=%d col=%d", row, col)
	}
}

func TestSetTextViewWrapResetsHorizontalOffset(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true).SetWrap(false)
	view.SetRect(0, 0, 10, 3)
	view.SetText(strings.Repeat("x", 80))
	scrollTextViewHoriz(view, 12)
	if _, col := view.GetScrollOffset(); col == 0 {
		t.Fatal("test setup should horizontally scroll")
	}
	setTextViewWrap(view, true)
	if _, col := view.GetScrollOffset(); col != 0 {
		t.Fatalf("enabling wrap should reset horizontal offset, got %d", col)
	}
}

func TestHandleTextViewScrollPageUpStopsAtTop(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true)
	view.SetText(strings.Repeat("line\n", 50))

	if !handleTextViewScroll(view, tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone)) {
		t.Fatal("page up should be handled")
	}
	if row, _ := view.GetScrollOffset(); row < 0 {
		t.Fatalf("page up should not scroll above top, got %d", row)
	}
}
