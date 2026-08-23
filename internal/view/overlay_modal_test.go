package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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
}
