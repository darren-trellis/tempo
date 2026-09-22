package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestModalOmitsInWindowButtons(t *testing.T) {
	modal := newModal(components.ModalConfig{Title: "Delete", Width: 40, Height: 10})
	modal.SetContent(tview.NewTextView().SetText("Sure?"))
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
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
	if strings.Contains(got, "Confirm") {
		t.Fatalf("modal window should not draw action buttons, got %q", got)
	}
}

func TestModalShowsFooterHintsAutomatically(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	if a.modalHintsOn || len(a.menu.GetHints()) != 0 {
		t.Fatal("workflow list should not show footer hints")
	}

	modal := newModal(components.ModalConfig{Title: "Cancel", Width: 40, Height: 10})
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Confirm"},
		{Key: "Esc", Description: "Cancel"},
	})
	a.PushModal(modal)
	if !a.modalHintsOn {
		t.Fatal("opening a modal should show footer hints without ?")
	}
	if hintDescription(a.menu.GetHints(), "Enter") != "Confirm" || hintDescription(a.menu.GetHints(), "Esc") != "Cancel" {
		t.Fatalf("bottom bar hints: %+v", a.menu.GetHints())
	}

	a.app.Pages().DismissModal()
	if a.modalHintsOn || len(a.menu.GetHints()) != 0 {
		t.Fatal("closing the modal should hide footer hints")
	}
}

func TestQuestionMarkStaysTypeableInsideAModal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	if !a.handleQuestionMark() {
		t.Fatal("? should open help from a pane view")
	}
	a.closeHelp()

	a.PushModal(newModal(components.ModalConfig{Title: "Cancel", Width: 40, Height: 10}))
	if a.handleQuestionMark() {
		t.Fatal("? should reach the modal's fields instead of being swallowed")
	}
}
