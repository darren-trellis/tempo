package view

import (
	"fmt"
	"strings"

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

func formatFailureSidePanel(ev *temporal.EnhancedHistoryEvent) string {
	if ev == nil || (ev.Failure == "" && ev.FailureSource == "" && ev.FailureStackTrace == "" && ev.FailureCause == "") {
		return ""
	}

	var result strings.Builder
	if ev.FailureSource != "" {
		result.WriteString(fmt.Sprintf("\n\n[%s::b]Source[-:-:-]\n[%s]%s[-]",
			theme.TagAccent(), theme.TagFg(), escapeForTView(ev.FailureSource)))
	}
	if ev.FailureStackTrace != "" {
		result.WriteString(fmt.Sprintf("\n\n[%s::b]Stack Trace[-:-:-]\n[%s]%s[-]",
			theme.TagAccent(), theme.TagFgDim(), escapeForTView(ev.FailureStackTrace)))
	}
	if ev.FailureCause != "" {
		result.WriteString(fmt.Sprintf("\n\n[%s::b]Cause[-:-:-]\n[%s]%s[-]",
			theme.TagAccent(), theme.TagFgDim(), escapeForTView(ev.FailureCause)))
	}
	return result.String()
}

// formatSidePanelDetails formats event details with pretty-printed JSON and syntax highlighting.
func formatSidePanelDetails(details string) string {
	if details == "" {
		return fmt.Sprintf("[%s]No details[-]", theme.TagFgDim())
	}

	// First try to pretty print if it's pure JSON
	trimmed := strings.TrimSpace(details)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		formatted := prettyPrintJSON(details)
		return highlightFormattedJSON(formatted)
	}

	// Handle key-value format like "WorkflowType: Foo, TaskQueue: bar, Input: {...}"
	return formatKeyValueDetails(details)
}

// formatKeyValueDetails formats key-value style details with embedded JSON.
func formatKeyValueDetails(details string) string {
	var result strings.Builder

	// Split by common delimiters but preserve JSON objects
	parts := splitPreservingJSON(details)

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if i > 0 {
			result.WriteString("\n")
		}

		// Check if this part has a key: value structure
		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			key := strings.TrimSpace(part[:colonIdx])
			value := strings.TrimSpace(part[colonIdx+1:])

			// Write the key in accent color
			result.WriteString(fmt.Sprintf("[%s]%s:[-] ", theme.TagAccent(), key))

			// Check if value is JSON
			if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
				formatted := prettyPrintJSON(value)
				if formatted != value {
					// JSON was successfully formatted - indent it
					lines := strings.Split(formatted, "\n")
					for j, line := range lines {
						if j == 0 {
							result.WriteString(highlightJSONValueLine(line))
						} else {
							result.WriteString("\n  ")
							result.WriteString(highlightJSONValueLine(line))
						}
					}
				} else {
					result.WriteString(highlightJSONValueLine(value))
				}
			} else {
				result.WriteString(highlightJSONValueLine(value))
			}
		} else {
			// No key-value structure, just highlight as value
			result.WriteString(highlightJSONValueLine(part))
		}
	}

	return result.String()
}

// splitPreservingJSON splits a string by commas while preserving JSON objects.
func splitPreservingJSON(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for _, ch := range s {
		switch ch {
		case '{', '[':
			depth++
			current.WriteRune(ch)
		case '}', ']':
			depth--
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// highlightFormattedJSON applies syntax highlighting to already-formatted JSON.
func highlightFormattedJSON(formatted string) string {
	lines := strings.Split(formatted, "\n")
	var result []string
	for _, line := range lines {
		result = append(result, highlightJSONValueLine(line))
	}
	return strings.Join(result, "\n")
}

// highlightJSONValueLine highlights a single line of JSON content.
func highlightJSONValueLine(line string) string {
	// Check for key: value pattern
	if colonIdx := strings.Index(line, ":"); colonIdx > 0 {
		prefix := line[:colonIdx]
		suffix := line[colonIdx+1:]

		trimmed := strings.TrimSpace(prefix)
		if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
			// JSON key with quotes - use accent color
			return fmt.Sprintf("[%s]%s[-]:[%s]%s[-]", theme.TagAccent(), escapeForTView(prefix), theme.TagFg(), highlightValues(suffix))
		}
	}

	return highlightValues(line)
}

// highlightValues highlights JSON values (booleans, null, numbers).
func highlightValues(s string) string {
	result := escapeForTView(s)
	result = strings.ReplaceAll(result, "true", fmt.Sprintf("[%s]true[-]", temporal.StatusCompleted.ColorTag()))
	result = strings.ReplaceAll(result, "false", fmt.Sprintf("[%s]false[-]", temporal.StatusFailed.ColorTag()))
	result = strings.ReplaceAll(result, "null", fmt.Sprintf("[%s]null[-]", theme.TagFgDim()))
	return result
}

func prettyPrintJSON(s string) string {
	return formatJSONPretty(s)
}
