package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestScrollbarThumb(t *testing.T) {
	pos, size := scrollbarThumb(0, 10, 100, 10)
	if size != 1 || pos != 0 {
		t.Fatalf("top: pos=%d size=%d", pos, size)
	}
	pos, size = scrollbarThumb(90, 10, 100, 10)
	if size != 1 || pos != 9 {
		t.Fatalf("bottom: pos=%d size=%d", pos, size)
	}
	pos, size = scrollbarThumb(0, 10, 10, 10)
	if pos != 0 || size != 10 {
		t.Fatalf("no overflow should fill: pos=%d size=%d", pos, size)
	}
	pos, size = scrollbarThumb(-4, 5, 20, 8)
	if pos != 0 {
		t.Fatalf("negative offset should clamp: pos=%d", pos)
	}
}

func TestAppShowsScrollbars(t *testing.T) {
	if !appShowsScrollbars(nil) {
		t.Fatal("nil app should show scrollbars")
	}
	off := false
	app := &App{config: &config.Config{ShowScrollbars: &off}}
	if appShowsScrollbars(app) {
		t.Fatal("show_scrollbars: false should hide scrollbars")
	}
}

func TestCharScrollViewDrawsScrollbar(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("ID")
	for i := 0; i < 20; i++ {
		table.AddRow("row")
	}
	view := newCharScrollView(table, func() int { return 8 }).withApp(nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(10, 6)
	view.SetRect(0, 0, 10, 6)
	view.Draw(screen)
	screen.Show()
	if !edgeHasGlyph(screen, 9, 0, 6, scrollbarThinVert) {
		t.Fatal("expected a vertical scrollbar on the right edge")
	}
}

func TestCharScrollViewHidesScrollbarWhenDisabled(t *testing.T) {
	off := false
	table := components.NewTable()
	table.SetHeaders("ID")
	for i := 0; i < 20; i++ {
		table.AddRow("row")
	}
	view := newCharScrollView(table, func() int { return 8 }).withApp(&App{
		config: &config.Config{ShowScrollbars: &off},
	})
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(10, 6)
	view.SetRect(0, 0, 10, 6)
	view.Draw(screen)
	screen.Show()
	shown := newCharScrollView(table, func() int { return 8 }).withApp(nil)
	shown.SetRect(0, 0, 10, 6)
	shownScreen := tcell.NewSimulationScreen("UTF-8")
	if err := shownScreen.Init(); err != nil {
		t.Fatal(err)
	}
	shownScreen.SetSize(10, 6)
	shown.Draw(shownScreen)
	shownScreen.Show()
	if edgeHasGlyph(screen, 9, 0, 6, scrollbarThinVert) {
		t.Fatal("disabling show_scrollbars should hide the vertical scrollbar")
	}
	if !edgeHasGlyph(shownScreen, 9, 0, 6, scrollbarThinVert) {
		t.Fatal("enabled scrollbars should draw the thin vertical glyph")
	}
}

func TestTextViewScrollbarReservesColumn(t *testing.T) {
	view := tview.NewTextView().SetScrollable(true).SetWrap(false)
	view.SetText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
	attachTextViewScrollbar(view, nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(12, 4)
	view.SetRect(0, 0, 12, 4)
	view.Draw(screen)
	screen.Show()
	if !edgeHasGlyph(screen, 11, 0, 4, scrollbarThinVert) {
		t.Fatal("expected a text view scrollbar")
	}
}

func TestScrollbarFollowsTableAfterPageJump(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("ID")
	for i := 0; i < 40; i++ {
		table.AddRow("row")
	}
	view := newCharScrollView(table, func() int { return 8 }).withApp(nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(10, 6)
	view.SetRect(0, 0, 10, 6)
	table.Select(0, 0)
	view.Draw(screen)
	screen.Show()
	top := scrollbarThumbY(screen, 9, 6)

	table.Select(35, 0)
	before, _ := table.GetOffset()
	view.Draw(screen)
	screen.Show()
	after, _ := table.GetOffset()
	if after <= before {
		t.Fatalf("table should scroll after select, before=%d after=%d", before, after)
	}
	bottom := scrollbarThumbY(screen, 9, 6)
	if bottom <= top {
		t.Fatalf("scrollbar should move on the same draw, top=%d bottom=%d offset=%d", top, bottom, after)
	}
}

func scrollbarThumbY(screen tcell.SimulationScreen, x, height int) int {
	for y := 0; y < height; y++ {
		mainc, _, style, _ := screen.GetContent(x, y)
		if mainc != scrollbarThinVert {
			continue
		}
		fg, _, _ := style.Decompose()
		if fg != theme.FgDim() {
			return y
		}
	}
	return -1
}

func TestHorizontalScrollbarIsThin(t *testing.T) {
	table := components.NewTable()
	table.SetHeaders("ID")
	table.AddRow("x")
	view := newCharScrollView(table, func() int { return 40 }).withApp(nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(10, 6)
	view.SetRect(0, 0, 10, 6)
	view.Draw(screen)
	screen.Show()
	mainc, _, _, _ := screen.GetContent(1, 5)
	if mainc != scrollbarThinHoriz {
		t.Fatalf("horizontal scrollbar should use a thin glyph, got %q", mainc)
	}
}

func edgeHasGlyph(screen tcell.SimulationScreen, x, y0, height int, glyph rune) bool {
	for y := y0; y < y0+height; y++ {
		mainc, _, _, _ := screen.GetContent(x, y)
		if mainc == glyph {
			return true
		}
	}
	return false
}

func TestScrollMetricsOverflow(t *testing.T) {
	if (scrollMetrics{offset: 0, visible: 10, total: 10}).overflow() {
		t.Fatal("exact fit should not overflow")
	}
	if !(scrollMetrics{offset: 0, visible: 10, total: 11}).overflow() {
		t.Fatal("one extra row should overflow")
	}
}
