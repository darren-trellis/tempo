package view

import (
	"testing"

	"github.com/atterpac/jig/components"
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
	_, _, style, _ := screen.GetContent(9, 1)
	_, bg, _ := style.Decompose()
	if bg == tcell.ColorDefault {
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
	shown := newCharScrollView(table, func() int { return 8 }).withApp(nil)
	shown.SetRect(0, 0, 10, 6)
	shownScreen := tcell.NewSimulationScreen("UTF-8")
	if err := shownScreen.Init(); err != nil {
		t.Fatal(err)
	}
	shownScreen.SetSize(10, 6)
	shown.Draw(shownScreen)
	_, _, offStyle, _ := screen.GetContent(9, 1)
	_, _, onStyle, _ := shownScreen.GetContent(9, 1)
	_, offBg, _ := offStyle.Decompose()
	_, onBg, _ := onStyle.Decompose()
	if onBg == offBg {
		t.Fatal("disabling show_scrollbars should change the right-edge paint")
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
	_, _, style, _ := screen.GetContent(11, 0)
	_, bg, _ := style.Decompose()
	if bg == tcell.ColorDefault {
		t.Fatal("expected a text view scrollbar")
	}
}

func TestScrollMetricsOverflow(t *testing.T) {
	if (scrollMetrics{offset: 0, visible: 10, total: 10}).overflow() {
		t.Fatal("exact fit should not overflow")
	}
	if !(scrollMetrics{offset: 0, visible: 10, total: 11}).overflow() {
		t.Fatal("one extra row should overflow")
	}
}
