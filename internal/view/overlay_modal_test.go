package view

import (
	"testing"

	"github.com/atterpac/jig/components"
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

func TestOverlayModalHints(t *testing.T) {
	modal := newOverlayModal(components.ModalConfig{Title: "Columns", Width: 20, Height: 8}, tview.NewBox())
	modal.SetHints([]components.KeyHint{{Key: "enter", Description: "Save"}})
	hints := modal.Hints()
	if len(hints) != 1 || hints[0].Key != "enter" || hints[0].Description != "Save" {
		t.Fatalf("hints=%+v", hints)
	}
	if bar := modal.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("overlay hints should stay in the footer, got in-modal %+v", bar.Hints)
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
		t.Fatalf("footer hints: %+v", modal.Hints())
	}
	if hintDescription(modal.Hints(), "Ctrl+S") != "" {
		t.Fatal("ctrl+s should not be advertised")
	}
	if bar := modal.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("dialog hints should stay in the footer, got in-modal %+v", bar.Hints)
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
		t.Fatalf("profile form hints should stay in the footer, got in-modal %+v", bar.Hints)
	}
}

func TestProfileModalHintsStayInFooter(t *testing.T) {
	modal := NewProfileModal()
	hints := modal.Hints()
	if hintDescription(hints, "n") != "New" || hintDescription(hints, "e") != "Edit" {
		t.Fatalf("profile hints: %+v", hints)
	}
	if bar := modal.GetHintBar(); bar != nil && len(bar.Hints) != 0 {
		t.Fatalf("profile hints should stay in the footer, got in-modal %+v", bar.Hints)
	}
}
