package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestShadowedModalDrawsBackground(t *testing.T) {
	bg := tview.NewTextView().SetText("KEEP")
	modal := newModal(components.ModalConfig{Title: "Profile", Width: 20, Height: 8})
	modal.setModalBackground(bg)
	modal.SetContent(tview.NewBox())
	modal.SetRect(0, 0, 80, 24)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.Draw(screen)

	r, _, _, _ := screen.GetContent(0, 0)
	if r != 'K' {
		t.Fatalf("profile-style modal should keep the view behind it, got %q", string(r))
	}
}

func TestOverlayModalDrawsBackground(t *testing.T) {
	bg := tview.NewTextView().SetText("KEEP")
	modal := newOverlayModal(components.ModalConfig{
		Title:  "Columns",
		Width:  20,
		Height: 8,
	}, bg)
	modal.SetContent(tview.NewBox())
	modal.SetRect(0, 0, 80, 24)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.Draw(screen)

	r, _, _, _ := screen.GetContent(0, 0)
	if r != 'K' {
		t.Fatalf("background should remain visible, got %q", string(r))
	}
}

func TestPushedModalDrawsTheViewBehindItOnce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	wl.allWorkflows = []temporal.Workflow{{ID: "wf-1", RunID: "r1", Status: "Running"}}
	wl.applyFilter()
	a.app.Pages().Push(wl)
	draws := 0
	wl.table.Table.SetDrawFunc(func(_ tcell.Screen, x, y, w, h int) (int, int, int, int) {
		draws++
		return x, y, w, h
	})

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 40)
	pages := a.app.Pages()
	pages.SetRect(0, 0, 120, 40)

	first := newOverlayModal(components.ModalConfig{Title: "First", Width: 30, Height: 8}, wl)
	first.SetContent(tview.NewTextView().SetText("FIRST"))
	a.PushModal(first)
	draws = 0
	pages.Draw(screen)
	if draws != 1 {
		t.Fatalf("the workflows view should draw once under a modal, drew %d times", draws)
	}

	second := newOverlayModal(components.ModalConfig{Title: "Second", Width: 30, Height: 8}, wl)
	second.SetContent(tview.NewBox())
	second.SetRect(0, 0, 120, 40)
	a.PushModal(second)
	pages.Draw(screen)
	for y := 0; y < 40; y++ {
		if strings.Contains(rowText(screen, y, 120), "FIRST") {
			t.Fatal("a modal over a modal should still cover the lower modal")
		}
	}
}

func TestOverlayModalHints(t *testing.T) {
	modal := newOverlayModal(components.ModalConfig{Title: "Columns", Width: 20, Height: 8}, tview.NewBox())
	modal.SetHints([]components.KeyHint{{Key: "enter", Description: "Save"}})
	hints := modal.Hints()
	if len(hints) != 1 || hints[0].Key != "enter" || hints[0].Description != "Save" {
		t.Fatalf("hints=%+v", hints)
	}
	if bar := modal.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("jig hint bar should stay unused, got %+v", bar.Hints)
	}
}

func TestOverlayModalFramelessHidesParentPane(t *testing.T) {
	inner := tview.NewBox()
	modal := newOverlayModal(components.ModalConfig{
		Title:     "Input/Output",
		MinWidth:  20,
		MinHeight: 10,
	}, tview.NewBox())
	modal.frameless = true
	modal.SetContent(inner)
	modal.SetRect(0, 0, 80, 24)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.Draw(screen)

	px, py, pw, ph := modal.GetPanel().GetRect()
	cx, cy, cw, ch := inner.GetRect()
	if cx != px || cy != py || cw != pw || ch != ph {
		t.Fatalf("content should fill the modal without a parent pane, panel=%d,%d %dx%d content=%d,%d %dx%d", px, py, pw, ph, cx, cy, cw, ch)
	}
}

func TestWorkflowIOModalOmitsButtons(t *testing.T) {
	modal := newOverlayModal(components.ModalConfig{
		Title:     "Input/Output",
		MinWidth:  40,
		MinHeight: 12,
	}, tview.NewBox())
	modal.frameless = true
	modal.SetContent(tview.NewTextView().SetText("payload"))
	modal.SetHints(workflowIOHints(false, false))
	if hintDescription(modal.Hints(), "w") != "Wrap" ||
		hintDescription(modal.Hints(), "y") != "Copy" ||
		hintDescription(modal.Hints(), "esc") != "Close" {
		t.Fatalf("IO hints should still be available, got %+v", modal.Hints())
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.SetRect(0, 0, 80, 24)
	modal.Draw(screen)
	var b strings.Builder
	for y := 0; y < 24; y++ {
		for x := 0; x < 80; x++ {
			ch, _, _, _ := screen.GetContent(x, y)
			if ch != 0 {
				b.WriteRune(ch)
			}
		}
	}
	got := b.String()
	if strings.Contains(got, "Copy") || strings.Contains(got, "Close") {
		t.Fatalf("Copy/Close should not be drawn in the IO window, got %q", got)
	}
}

func TestFramelessModalDoesNotDuplicateButtons(t *testing.T) {
	modal := newOverlayModal(components.ModalConfig{
		Title:     "Input/Output",
		MinWidth:  40,
		MinHeight: 12,
	}, tview.NewBox())
	modal.frameless = true
	modal.SetContent(tview.NewTextView().SetText("payload"))
	modal.SetHints([]components.KeyHint{
		{Key: "y", Description: "Copy"},
		{Key: "esc", Description: "Close"},
	})
	modal.SetRect(0, 0, 80, 24)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.Draw(screen)

	var b strings.Builder
	for y := 0; y < 24; y++ {
		for x := 0; x < 80; x++ {
			ch, _, _, _ := screen.GetContent(x, y)
			if ch != 0 {
				b.WriteRune(ch)
			}
		}
	}
	got := b.String()
	if strings.Contains(got, "Copy") || strings.Contains(got, "Close") {
		t.Fatalf("Copy/Close should not be drawn in the IO window, got %q", got)
	}
}

func TestOverlayModalMaximize(t *testing.T) {
	modal := newOverlayModal(components.ModalConfig{
		Title:     "IO",
		MinWidth:  20,
		MinHeight: 10,
	}, tview.NewTextView().SetText("KEEP"))
	modal.SetContent(tview.NewBox())
	modal.SetRect(0, 0, 80, 24)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.Draw(screen)
	_, _, restW, restH := modal.GetPanel().GetRect()

	modal.toggleMaximize()
	modal.Draw(screen)
	_, _, maxW, maxH := modal.GetPanel().GetRect()
	if maxW != 80 || maxH != 24 {
		t.Fatalf("maximize should fill the page area, got %dx%d", maxW, maxH)
	}
	if maxW <= restW || maxH <= restH {
		t.Fatalf("maximize should grow the panel: rest=%dx%d max=%dx%d", restW, restH, maxW, maxH)
	}
}

func TestShadowedModalHintsStayInFooter(t *testing.T) {
	modal := newModal(components.ModalConfig{Title: "Cancel", Width: 20, Height: 8})
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
	})
	if hintDescription(modal.Hints(), "Enter") != "Confirm" {
		t.Fatalf("hints: %+v", modal.Hints())
	}
	if hintDescription(modal.Hints(), "Ctrl+S") != "" {
		t.Fatal("ctrl+s should not be advertised")
	}
	if bar := modal.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("jig hint bar should stay unused, got %+v", bar.Hints)
	}
}

func TestProfileFormHintsStayInFooter(t *testing.T) {
	form := NewProfileForm()
	if hintDescription(form.Hints(), "Enter") != "Save" {
		t.Fatalf("profile form hints: %+v", form.Hints())
	}
	if hintDescription(form.Hints(), "Ctrl+S") != "" {
		t.Fatal("ctrl+s should not be advertised")
	}
	if bar := form.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("jig hint bar should stay unused, got %+v", bar.Hints)
	}
}

func TestProfileModalHintsStayInFooter(t *testing.T) {
	modal := NewProfileModal()
	hints := modal.Hints()
	if hintDescription(hints, "n") != "New" || hintDescription(hints, "e") != "Edit" {
		t.Fatalf("profile hints: %+v", hints)
	}
	if bar := modal.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("jig hint bar should stay unused, got %+v", bar.Hints)
	}
}
