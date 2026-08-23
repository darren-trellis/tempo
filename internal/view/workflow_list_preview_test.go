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
