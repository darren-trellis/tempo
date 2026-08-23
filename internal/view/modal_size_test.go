package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
)

func TestResizableModalToggleMaximize(t *testing.T) {
	modal := newResizableModal(components.ModalConfig{
		Title:     "IO",
		MinWidth:  20,
		MinHeight: 10,
	})
	modal.SetRect(0, 0, 80, 24)
	if modal.maximized {
		t.Fatal("modal should start restored")
	}

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
	if maxW <= restW || maxH <= restH {
		t.Fatalf("maximize should grow the panel: rest=%dx%d max=%dx%d", restW, restH, maxW, maxH)
	}
	if maxW != 80 || maxH != 24 {
		t.Fatalf("maximized size should be full screen, got %dx%d", maxW, maxH)
	}

	modal.toggleMaximize()
	modal.Draw(screen)
	_, _, backW, backH := modal.GetPanel().GetRect()
	if backW != restW || backH != restH {
		t.Fatalf("restore should return to %dx%d, got %dx%d", restW, restH, backW, backH)
	}
}
