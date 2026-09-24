package view

import (
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

// eventIcon returns an icon for the event type.
func eventIcon(eventType string) string {
	switch {
	case contains(eventType, "Started"):
		return theme.IconRunning
	case contains(eventType, "Completed"):
		return theme.IconCompleted
	case contains(eventType, "Failed"):
		return theme.IconError
	case contains(eventType, "Scheduled"):
		return theme.IconPending
	case contains(eventType, "Timer"):
		return theme.IconTimedOut
	case contains(eventType, "Signal"):
		return theme.IconActivity
	case contains(eventType, "Child"):
		return theme.IconWorkflow
	default:
		return theme.IconEvent
	}
}

// eventColor returns a color for the event type.
func eventColor(eventType string) tcell.Color {
	switch {
	case contains(eventType, "Started"):
		return temporal.StatusRunning.Color()
	case contains(eventType, "Completed"):
		return temporal.StatusCompleted.Color()
	case contains(eventType, "Failed"):
		return temporal.StatusFailed.Color()
	case contains(eventType, "Scheduled"):
		return theme.FgDim()
	default:
		return theme.Fg()
	}
}

// eventColorTag returns a color tag for the event type.
func eventColorTag(eventType string) string {
	switch {
	case contains(eventType, "Started"):
		return temporal.StatusRunning.ColorTag()
	case contains(eventType, "Completed"):
		return temporal.StatusCompleted.ColorTag()
	case contains(eventType, "Failed"):
		return temporal.StatusFailed.ColorTag()
	default:
		return theme.TagFg()
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
