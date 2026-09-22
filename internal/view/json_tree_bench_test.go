package view

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func largeJSONTreePayload(tb testing.TB, items int) string {
	tb.Helper()
	list := make([]map[string]any, items)
	for i := range list {
		list[i] = map[string]any{
			"id":     fmt.Sprintf("item-%d", i),
			"amount": i * 3,
			"note":   "a fairly long note that makes the row wider than a narrow pane would show",
		}
	}
	data, err := json.Marshal(map[string]any{"items": list})
	if err != nil {
		tb.Fatal(err)
	}
	return string(data)
}

func TestJSONTreeReportsItsSizeWhileShown(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	tree := newJSONTreeSelection(view)
	if !tree.setContent(`{"a":1,"bb":"a longer value"}`, true) {
		t.Fatal("payload should render as a tree")
	}
	lines, width, ok := knownTextViewSize(view)
	if !ok || lines != len(tree.rows) {
		t.Fatalf("lines=%d ok=%v rows=%d", lines, ok, len(tree.rows))
	}
	want := 0
	for _, row := range tree.rows {
		if w := tview.TaggedStringWidth(row.text); w > want {
			want = w
		}
	}
	if width != want {
		t.Fatalf("width=%d want %d", width, want)
	}

	view.SetText("Loading...")
	if _, _, ok := knownTextViewSize(view); ok {
		t.Fatal("a status message replaces the tree, so the scrollbar should measure the text")
	}
	if tree.handleKey(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)) {
		t.Fatal("keys should not redraw a tree the view no longer shows")
	}
	if view.GetText(false) != "Loading..." {
		t.Fatalf("text=%q", view.GetText(false))
	}
}

func benchmarkJSONTreeStep(b *testing.B, wrap bool) {
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	setTextViewWrap(view, wrap)
	attachTextViewScrollbar(view, nil)
	tree := newJSONTreeSelection(view)
	view.SetRect(0, 0, 60, 40)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	screen.SetSize(60, 40)
	if !tree.setContent(largeJSONTreePayload(b, 500), true) {
		b.Fatal("payload should render as a tree")
	}
	view.Draw(screen)
	if len(tree.rows) < 2000 {
		b.Fatalf("rows=%d", len(tree.rows))
	}
	tree.selectRow(len(tree.rows) / 2)
	view.Draw(screen)
	down := tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)
	up := tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			tree.handleKey(down)
		} else {
			tree.handleKey(up)
		}
		view.Draw(screen)
	}
}

func BenchmarkWorkflowIOModalStep(b *testing.B) {
	b.Run("text", func(b *testing.B) { benchmarkWorkflowIOModalStep(b, false) })
	b.Run("tree", func(b *testing.B) { benchmarkWorkflowIOModalStep(b, true) })
}

func benchmarkWorkflowIOModalStep(b *testing.B, tree bool) {
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	cfg := config.DefaultConfig()
	cfg.IOTree = &tree
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	payload := largeJSONTreePayload(b, 500)
	showWorkflowIO(a, tview.NewBox(), "Order", payload, payload, nil)
	modal, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		b.Fatalf("current=%T", a.app.Pages().Current())
	}
	focused := firstTextView(modal.GetPanel())
	capture := focused.GetInputCapture()
	if _, _, ok := knownTextViewSize(focused); ok != tree {
		b.Fatalf("tree=%v should decide whether the tree reports its size", tree)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	screen.SetSize(200, 50)
	pages := a.app.Pages()
	pages.SetRect(0, 0, 200, 50)
	pages.Draw(screen)
	for i := 0; i < 1000; i++ {
		capture(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	}
	pages.Draw(screen)
	down := tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)
	up := tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			capture(down)
		} else {
			capture(up)
		}
		pages.Draw(screen)
	}
}

func BenchmarkJSONTreeStep(b *testing.B) {
	b.Run("nowrap", func(b *testing.B) { benchmarkJSONTreeStep(b, false) })
	b.Run("wrap", func(b *testing.B) { benchmarkJSONTreeStep(b, true) })
}
