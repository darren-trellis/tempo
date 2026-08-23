package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
)

func TestNewWorkflowListDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewWorkflowList panicked: %v", r)
		}
	}()
	NewWorkflowList(&App{}, "default")
}

func TestPreviewModeLayout(t *testing.T) {
	off := NewWorkflowList(&App{}, "default")
	if off.GetItemCount() != 1 {
		t.Fatalf("default layout should be workflows only, got %d items", off.GetItemCount())
	}
	if desc := hintDescription(off.Hints(), "enter"); desc != "Detail" {
		t.Fatalf("default enter hint: got %q", desc)
	}

	on := true
	wl := NewWorkflowList(&App{config: &config.Config{PreviewMode: &on}}, "default")
	if wl.GetItemCount() != 2 {
		t.Fatalf("preview mode should show events pane, got %d items", wl.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "Events" {
		t.Fatalf("preview enter hint: got %q", desc)
	}

	wl.app.config.SetPreviewMode(false)
	wl.applyPreviewLayout()
	if wl.GetItemCount() != 1 {
		t.Fatalf("disabling preview should hide events pane, got %d items", wl.GetItemCount())
	}
}

func hintDescription(hints []KeyHint, key string) string {
	for _, h := range hints {
		if h.Key == key {
			return h.Description
		}
	}
	return ""
}
