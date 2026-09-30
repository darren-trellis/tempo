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
	workflowInfoChildren  = "Children"
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

// workflowChildStatuses reads the latest status of each child workflow off
// the parent's history, in the order they were started. A child that was
// asked for but has not started yet is Scheduled.
func workflowChildStatuses(events []temporal.EnhancedHistoryEvent) []string {
	var order []int64
	status := make(map[int64]string)
	for _, ev := range events {
		if ev.Type == "StartChildWorkflowExecutionInitiated" {
			order = append(order, ev.ID)
			status[ev.ID] = temporal.StatusScheduled.Name()
			continue
		}
		if _, known := status[ev.InitiatedEventID]; !known {
			continue
		}
		switch ev.Type {
		case "ChildWorkflowExecutionStarted":
			status[ev.InitiatedEventID] = temporal.StatusRunning.Name()
		case "ChildWorkflowExecutionCompleted":
			status[ev.InitiatedEventID] = temporal.StatusCompleted.Name()
		case "ChildWorkflowExecutionFailed", "StartChildWorkflowExecutionFailed":
			status[ev.InitiatedEventID] = temporal.StatusFailed.Name()
		case "ChildWorkflowExecutionCanceled":
			status[ev.InitiatedEventID] = temporal.StatusCanceled.Name()
		case "ChildWorkflowExecutionTimedOut":
			status[ev.InitiatedEventID] = temporal.StatusTimedOut.Name()
		case "ChildWorkflowExecutionTerminated":
			status[ev.InitiatedEventID] = temporal.StatusTerminated.Name()
		}
	}
	statuses := make([]string, len(order))
	for i, id := range order {
		statuses[i] = status[id]
	}
	return statuses
}

// workflowChildrenRow summarizes a workflow's children as a count with a
// breakdown by status, such as "3 · 2 Completed, 1 Running". It is colored
// by the worst status among them. ok is false when there are none.
func workflowChildrenRow(events []temporal.EnhancedHistoryEvent) (workflowInfoRow, bool) {
	statuses := workflowChildStatuses(events)
	if len(statuses) == 0 {
		return workflowInfoRow{}, false
	}
	counts := make(map[string]int)
	var names []string
	for _, s := range statuses {
		if counts[s] == 0 {
			names = append(names, s)
		}
		counts[s]++
	}
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%d %s", counts[name], name)
	}
	count := fmt.Sprintf("%d", len(statuses))
	handle := temporal.StatusCompleted
	for _, worst := range []*theme.Status{temporal.StatusFailed, temporal.StatusTerminated, temporal.StatusTimedOut, temporal.StatusCanceled, temporal.StatusRunning, temporal.StatusScheduled} {
		if counts[worst.Name()] > 0 {
			handle = worst
			break
		}
	}
	return workflowInfoRow{
		Key:      workflowInfoChildren,
		Label:    "Children",
		Value:    count,
		Display:  count + " · " + strings.Join(parts, ", "),
		Color:    handle.Color(),
		ColorTag: handle.ColorTag(),
	}, true
}

// withChildrenRow puts the children row after the parent, or after the ID
// when there is no parent, so a workflow's relatives read together.
func withChildrenRow(rows []workflowInfoRow, events []temporal.EnhancedHistoryEvent) []workflowInfoRow {
	children, ok := workflowChildrenRow(events)
	if !ok {
		return rows
	}
	at := workflowInfoRowIndex(rows, workflowInfoParent)
	if at < 0 {
		at = workflowInfoRowIndex(rows, workflowInfoID)
	}
	at++
	out := make([]workflowInfoRow, 0, len(rows)+1)
	out = append(out, rows[:at]...)
	out = append(out, children)
	return append(out, rows[at:]...)
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
