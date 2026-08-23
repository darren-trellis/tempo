package view

import "testing"

func TestNewWorkflowListDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewWorkflowList panicked: %v", r)
		}
	}()
	NewWorkflowList(&App{}, "default")
}

func TestPreviewModeLayout(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.GetItemCount() != 1 {
		t.Fatalf("default layout should be workflows only, got %d items", wl.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "Detail" {
		t.Fatalf("default enter hint: got %q", desc)
	}

	wl.togglePreviewMode()
	if wl.GetItemCount() != 2 {
		t.Fatalf("preview should show events pane, got %d items", wl.GetItemCount())
	}
	if desc := hintDescription(wl.Hints(), "enter"); desc != "Activities" {
		t.Fatalf("preview enter hint: got %q", desc)
	}

	wl.togglePreviewMode()
	if wl.GetItemCount() != 1 {
		t.Fatalf("hiding preview should show workflows only, got %d items", wl.GetItemCount())
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
