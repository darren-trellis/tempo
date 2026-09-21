package view

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

const (
	workflowInfoID        = "ID"
	workflowInfoParent    = "Parent"
	workflowInfoType      = "Type"
	workflowInfoStatus    = "Status"
	workflowInfoStarted   = "Started"
	workflowInfoDuration  = "Duration"
	workflowInfoTaskQueue = "Task Queue"
	workflowInfoRunID     = "Run ID"
	workflowInfoLabelPad  = 13
)

type workflowInfoRow struct {
	Key      string
	Label    string
	Value    string
	Display  string
	Color    tcell.Color
	ColorTag string
}

func workflowInfoRows(now time.Time, w temporal.Workflow) []workflowInfoRow {
	statusLabel, statusHandle := temporal.WorkflowDisplayStatus(w)
	durationStr := "In progress"
	if w.EndTime != nil {
		durationStr = w.EndTime.Sub(w.StartTime).Round(time.Second).String()
	} else if w.Status == "Running" {
		durationStr = now.Sub(w.StartTime).Round(time.Second).String()
	}

	rows := []workflowInfoRow{
		{Key: workflowInfoID, Label: "ID", Value: w.ID, Color: theme.Fg(), ColorTag: theme.TagFg()},
	}
	if w.ParentID != nil && *w.ParentID != "" {
		rows = append(rows, workflowInfoRow{
			Key:      workflowInfoParent,
			Label:    "Parent",
			Value:    *w.ParentID,
			Color:    theme.Fg(),
			ColorTag: theme.TagFg(),
		})
	}
	rows = append(rows,
		workflowInfoRow{Key: workflowInfoType, Label: "Type", Value: w.Type, Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{
			Key:      workflowInfoStatus,
			Label:    "Status",
			Value:    statusLabel,
			Display:  statusHandle.Icon() + " " + statusLabel,
			Color:    statusHandle.Color(),
			ColorTag: statusHandle.ColorTag(),
		},
		workflowInfoRow{Key: workflowInfoStarted, Label: "Started", Value: formatRelativeTime(now, w.StartTime), Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{Key: workflowInfoDuration, Label: "Duration", Value: durationStr, Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{Key: workflowInfoTaskQueue, Label: "Task Queue", Value: w.TaskQueue, Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{Key: workflowInfoRunID, Label: "Run ID", Value: w.RunID, Color: theme.FgDim(), ColorTag: theme.TagFgDim()},
	)
	return rows
}

func (r workflowInfoRow) displayText() string {
	if r.Display != "" {
		return r.Display
	}
	return r.Value
}

func workflowInfoContentWidth(rows []workflowInfoRow) int {
	labelWidth := 0
	valueWidth := 0
	for _, row := range rows {
		if n := len(row.Label); n > labelWidth {
			labelWidth = n
		}
		if n := len(row.displayText()); n > valueWidth {
			valueWidth = n
		}
	}
	if labelWidth == 0 && valueWidth == 0 {
		return 0
	}
	return labelWidth + 1 + valueWidth
}

func formatWorkflowInfo(w temporal.Workflow) string {
	rows := workflowInfoRows(time.Now(), w)
	var b strings.Builder
	b.WriteByte('\n')
	for i, row := range rows {
		label := row.Label
		if len(label) < workflowInfoLabelPad {
			label += strings.Repeat(" ", workflowInfoLabelPad-len(label))
		}
		fmt.Fprintf(&b, "[%s::b]%s[-:-:-] [%s]%s[-]", theme.TagFgDim(), label, row.ColorTag, row.displayText())
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func formatEventDetails(details string) string {
	if details == "" {
		return fmt.Sprintf("[%s]No details[-]", theme.TagFgDim())
	}

	// First check if the whole thing is JSON
	trimmed := strings.TrimSpace(details)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		formatted := formatJSONPretty(details)
		return highlightFormattedJSONWorkflow(formatted)
	}

	// Handle key-value format with embedded JSON
	return formatKeyValueDetailsWorkflow(details)
}

// formatKeyValueDetailsWorkflow formats key-value style details with embedded JSON.
func formatKeyValueDetailsWorkflow(details string) string {
	var result strings.Builder

	// Split by commas while preserving JSON objects
	parts := splitPreservingJSONWorkflow(details)

	// First pass: find max key length for alignment
	type kvPair struct {
		key   string
		value string
	}
	var pairs []kvPair
	maxKeyLen := 0

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Find the key-value split point (first colon not inside JSON)
		colonIdx := findKeyColonIndex(part)
		if colonIdx > 0 {
			key := strings.TrimSpace(part[:colonIdx])
			value := strings.TrimSpace(part[colonIdx+1:])
			pairs = append(pairs, kvPair{key, value})
			if len(key) > maxKeyLen {
				maxKeyLen = len(key)
			}
		} else {
			pairs = append(pairs, kvPair{"", part})
		}
	}

	// Second pass: format with aligned keys
	for i, kv := range pairs {
		if i > 0 {
			result.WriteString("\n")
		}

		if kv.key != "" {
			// Pad key for alignment
			paddedKey := kv.key + strings.Repeat(" ", maxKeyLen-len(kv.key))

			// Check if value is JSON
			value := strings.TrimSpace(kv.value)
			if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
				formatted := formatJSONPretty(value)
				if formatted != value {
					// JSON was successfully formatted - put it on next line at left margin
					result.WriteString(fmt.Sprintf("[%s::b]%s[-:-:-]\n", theme.TagFgDim(), paddedKey))
					result.WriteString(highlightFormattedJSONWorkflow(formatted))
				} else {
					result.WriteString(fmt.Sprintf("[%s::b]%s[-:-:-]  ", theme.TagFgDim(), paddedKey))
					result.WriteString(highlightJSONLineWorkflow(value))
				}
			} else {
				result.WriteString(fmt.Sprintf("[%s::b]%s[-:-:-]  ", theme.TagFgDim(), paddedKey))
				result.WriteString(fmt.Sprintf("[%s]%s[-]", theme.TagFg(), highlightValuesWorkflow(value)))
			}
		} else {
			result.WriteString(fmt.Sprintf("[%s]%s[-]", theme.TagFg(), escapeForTView(kv.value)))
		}
	}

	return result.String()
}

// splitPreservingJSONWorkflow splits a string by commas while preserving JSON objects.
func splitPreservingJSONWorkflow(s string) []string {
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

// findKeyColonIndex finds the index of the colon that separates key from value.
// It ignores colons inside JSON objects or strings.
func findKeyColonIndex(s string) int {
	depth := 0
	inString := false
	for i, ch := range s {
		switch ch {
		case '"':
			inString = !inString
		case '{', '[':
			if !inString {
				depth++
			}
		case '}', ']':
			if !inString {
				depth--
			}
		case ':':
			if depth == 0 && !inString {
				return i
			}
		}
	}
	return -1
}

func formatJSONPretty(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	values, ok := decodeJSONValues(s)
	if !ok {
		return s
	}

	parts := make([]string, 0, len(values))
	for _, v := range values {
		pretty, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return s
		}
		parts = append(parts, string(pretty))
	}
	return strings.Join(parts, ",\n")
}

func decodeJSONValues(s string) ([]any, bool) {
	var values []any
	for {
		dec := json.NewDecoder(strings.NewReader(s))
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		values = append(values, v)
		rest := strings.TrimSpace(s[dec.InputOffset():])
		if rest == "" {
			return values, true
		}
		if rest[0] != ',' {
			return nil, false
		}
		s = strings.TrimSpace(rest[1:])
		if s == "" {
			return nil, false
		}
	}
}

// highlightFormattedJSONWorkflow applies syntax highlighting to formatted JSON.
func highlightFormattedJSONWorkflow(formatted string) string {
	lines := strings.Split(formatted, "\n")
	var result []string
	for _, line := range lines {
		result = append(result, highlightJSONLineWorkflow(line))
	}
	return strings.Join(result, "\n")
}

// highlightJSONLineWorkflow highlights a single line of JSON content.
func highlightJSONLineWorkflow(line string) string {
	// Check for key: value pattern
	if colonIdx := strings.Index(line, ":"); colonIdx > 0 {
		prefix := line[:colonIdx]
		suffix := line[colonIdx+1:]

		trimmed := strings.TrimSpace(prefix)
		if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
			return fmt.Sprintf("[%s]%s[-]:[%s]%s[-]", theme.TagAccent(), escapeForTView(prefix), theme.TagFg(), highlightValuesWorkflow(suffix))
		}
	}

	return highlightValuesWorkflow(line)
}

// escapeForTView removes '[' so tview cannot parse payload bytes as style tags.
// tview.Escape only handles already-closed tags like [red], not raw '['.
func escapeForTView(s string) string {
	return strings.ReplaceAll(s, "[", "［")
}

// highlightValuesWorkflow highlights JSON values (booleans, null).
func highlightValuesWorkflow(s string) string {
	result := escapeForTView(s)
	result = strings.ReplaceAll(result, "true", fmt.Sprintf("[%s]true[-]", temporal.StatusCompleted.ColorTag()))
	result = strings.ReplaceAll(result, "false", fmt.Sprintf("[%s]false[-]", temporal.StatusFailed.ColorTag()))
	result = strings.ReplaceAll(result, "null", fmt.Sprintf("[%s]null[-]", theme.TagFgDim()))
	return result
}

func getEventNameDetail(ev *temporal.EnhancedHistoryEvent) string {
	if ev.ActivityType != "" {
		return ev.ActivityType
	}
	if ev.TimerID != "" {
		return "Timer: " + ev.TimerID
	}
	if ev.ChildWorkflowType != "" {
		return ev.ChildWorkflowType
	}
	return ""
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
