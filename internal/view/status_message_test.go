package view

import (
	"testing"

	"github.com/atterpac/jig/layout"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

func TestToastUsesHintBar(t *testing.T) {
	a := &App{menu: layout.NewMenu()}
	a.menu.SetHints([]KeyHint{{Key: "y", Description: "Copy ID"}})
	a.ToastSuccess("Copied workflow ID")
	if a.hintBarMessage() != "Copied workflow ID" {
		t.Fatalf("hint bar: %q", a.hintBarMessage())
	}
	if got := a.menu.GetHints(); len(got) != 1 || got[0].Key != "y" {
		t.Fatal("status should keep key hints for after it clears")
	}

	a.ToastError("Failed to copy")
	if a.hintBarMessage() != "Failed to copy" {
		t.Fatalf("error should replace the previous message, got %q", a.hintBarMessage())
	}

	a.setStatusMessage("")
	if a.hintBarMessage() != "" {
		t.Fatal("clearing should empty the hint bar status")
	}
}

func TestHintStatusCoversBarLeftAligned(t *testing.T) {
	menu := layout.NewMenu()
	menu.SetHints([]KeyHint{{Key: "y", Description: "Copy ID"}})
	menu.SetRect(0, 0, 40, 1)
	a := &App{menu: menu}
	a.setStatusMessage("Copied workflow ID")

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 1)
	menu.Draw(screen)
	a.drawHintStatus(screen)

	main, _, style, _ := screen.GetContent(1, 0)
	if main != 'C' {
		t.Fatalf("status should start on the left, got %q", main)
	}
	if fg, _, _ := style.Decompose(); fg != theme.Fg() {
		t.Fatalf("status should use the app foreground, got %v want %v", fg, theme.Fg())
	}

	a.setStatusMessage("")
	menu.Draw(screen)
	a.drawHintStatus(screen)
	main, _, _, _ = screen.GetContent(1, 0)
	if main == 'C' {
		t.Fatal("cleared status should leave the bottom bar empty of that message")
	}
}
