package view

import (
	"testing"

	"github.com/atterpac/jig/layout"
)

func TestToastUsesHintBar(t *testing.T) {
	a := &App{menu: layout.NewMenu()}
	a.ToastSuccess("Copied workflow ID")
	if a.hintBarMessage() != "Copied workflow ID" {
		t.Fatalf("hint bar: %q", a.hintBarMessage())
	}
	if got := a.menu.GetHints(); len(got) != 0 {
		t.Fatal("status should not replace key hints")
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
