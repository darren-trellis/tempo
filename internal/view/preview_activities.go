package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
)

type previewKind int

const (
	previewDetails previewKind = iota
	previewActivities
	previewEvents
	previewHierarchy
)

var previewTabOrder = []previewKind{previewDetails, previewActivities, previewEvents, previewHierarchy}

type activityDetailKind int

const (
	activityDetailDetails activityDetailKind = iota
	activityDetailInput
	activityDetailOutput
)

var activityDetailTabOrder = []activityDetailKind{activityDetailDetails, activityDetailInput, activityDetailOutput}

type previewActivity struct {
	ScheduledID int64
	ActivityID  string
	Type        string
	Status      string
	StartTime   time.Time
	EndTime     *time.Time
	Attempt     int32
	TaskQueue   string
	Identity    string
	Input       string
	Result      string
	Failure     string
}

func (k previewKind) title() string {
	switch k {
	case previewEvents:
		return "Events"
	case previewDetails:
		return "Details"
	case previewHierarchy:
		return "Hierarchy"
	default:
		return "Activities"
	}
}

func (k previewKind) icon() string {
	switch k {
	case previewEvents:
		return theme.IconEvent
	case previewDetails:
		return theme.IconWorkflow
	case previewHierarchy:
		return theme.IconNamespace
	default:
		return theme.IconActivity
	}
}

func previewActivitiesFromEvents(events []temporal.EnhancedHistoryEvent) []previewActivity {
	bySched := make(map[int64]*previewActivity)
	var order []int64

	for i := range events {
		ev := events[i]
		switch {
		case strings.Contains(ev.Type, "ActivityTaskScheduled"):
			a := &previewActivity{
				ScheduledID: ev.ID,
				ActivityID:  ev.ActivityID,
				Type:        ev.ActivityType,
				Status:      "Scheduled",
				StartTime:   ev.Time,
				TaskQueue:   ev.TaskQueue,
				Input:       ev.Input,
			}
			if a.Input == "" {
				a.Input = detailField(ev.Details, "Input")
			}
			if a.Type == "" {
				a.Type = getEventNameDetail(&ev)
			}
			bySched[ev.ID] = a
			order = append(order, ev.ID)
		case strings.Contains(ev.Type, "ActivityTaskStarted"):
			if a := bySched[ev.ScheduledEventID]; a != nil {
				a.Status = "Running"
				if ev.Attempt > a.Attempt {
					a.Attempt = ev.Attempt
				}
				if ev.Identity != "" {
					a.Identity = ev.Identity
				}
			}
		case strings.Contains(ev.Type, "ActivityTaskCompleted"):
			if a := bySched[ev.ScheduledEventID]; a != nil {
				a.Status = "Completed"
				if ev.Result != "" {
					a.Result = ev.Result
				} else if result := detailField(ev.Details, "Result"); result != "" {
					a.Result = result
				}
				end := ev.Time
				a.EndTime = &end
			}
		case strings.Contains(ev.Type, "ActivityTaskFailed"):
			if a := bySched[ev.ScheduledEventID]; a != nil {
				a.Status = "Failed"
				if ev.Failure != "" {
					a.Failure = ev.Failure
				}
				end := ev.Time
				a.EndTime = &end
			}
		case strings.Contains(ev.Type, "ActivityTaskTimedOut"):
			if a := bySched[ev.ScheduledEventID]; a != nil {
				a.Status = "TimedOut"
				if ev.Failure != "" {
					a.Failure = ev.Failure
				}
				end := ev.Time
				a.EndTime = &end
			}
		case strings.Contains(ev.Type, "ActivityTaskCanceled"):
			if a := bySched[ev.ScheduledEventID]; a != nil {
				a.Status = "Canceled"
				end := ev.Time
				a.EndTime = &end
			}
		}
	}

	out := make([]previewActivity, 0, len(order))
	for _, id := range order {
		out = append(out, *bySched[id])
	}
	return out
}

func detailField(details, key string) string {
	prefix := key + ":"
	for _, part := range splitPreservingJSONWorkflow(details) {
		part = strings.TrimSpace(part)
		if len(part) < len(prefix) {
			continue
		}
		if strings.EqualFold(part[:len(prefix)], prefix) {
			return strings.TrimSpace(part[len(prefix):])
		}
	}
	return ""
}

func (a previewActivity) duration() string {
	if a.EndTime != nil {
		return a.EndTime.Sub(a.StartTime).Round(time.Millisecond).String()
	}
	if a.Status == "Running" {
		return time.Since(a.StartTime).Round(time.Second).String()
	}
	return ""
}

func (k activityDetailKind) title() string {
	switch k {
	case activityDetailInput:
		return "Input"
	case activityDetailOutput:
		return "Output"
	default:
		return "Details"
	}
}

func (k activityDetailKind) icon() string {
	switch k {
	case activityDetailInput:
		return theme.IconArrowRight
	case activityDetailOutput:
		return theme.IconArrowLeft
	default:
		return theme.IconInfo
	}
}

func formatActivityInput(a previewActivity) string {
	return formatIOContent("Input", a.Input)
}

func formatActivityOutput(a previewActivity) string {
	if a.Result != "" {
		return formatIOContent("Output", a.Result)
	}
	if a.Failure != "" {
		return fmt.Sprintf("[%s]%s[-]", theme.TagError(), a.Failure)
	}
	return formatIOContent("Output", "")
}

const (
	activityInfoName      = "Activity"
	activityInfoID        = "ID"
	activityInfoStatus    = "Status"
	activityInfoStarted   = "Started"
	activityInfoEnded     = "Ended"
	activityInfoDuration  = "Duration"
	activityInfoAttempt   = "Attempt"
	activityInfoTaskQueue = "Task Queue"
	activityInfoIdentity  = "Identity"
)

const (
	eventInfoID   = "ID"
	eventInfoType = "Type"
	eventInfoName = "Name"
	eventInfoTime = "Time"
)

func eventInfoRows(ev temporal.EnhancedHistoryEvent) []workflowInfoRow {
	rows := []workflowInfoRow{
		{Key: eventInfoID, Label: "ID", Value: fmt.Sprintf("%d", ev.ID), Color: theme.Fg(), ColorTag: theme.TagFg()},
		{
			Key:      eventInfoType,
			Label:    "Type",
			Value:    ev.Type,
			Display:  eventIcon(ev.Type) + " " + ev.Type,
			Color:    eventColor(ev.Type),
			ColorTag: eventColorTag(ev.Type),
		},
	}
	if name := getEventNameDetail(&ev); name != "" {
		rows = append(rows, workflowInfoRow{Key: eventInfoName, Label: "Name", Value: name, Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	if !ev.Time.IsZero() {
		rows = append(rows, workflowInfoRow{Key: eventInfoTime, Label: "Time", Value: ev.Time.Format("2006-01-02 15:04:05.000"), Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	if ev.Failure != "" {
		rows = append(rows, workflowInfoRow{Key: "Failure", Label: "Failure", Value: ev.Failure, Color: theme.Error(), ColorTag: theme.TagError()})
	}
	if ev.FailureSource != "" {
		rows = append(rows, workflowInfoRow{Key: "Failure Source", Label: "Source", Value: ev.FailureSource, Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	if ev.FailureStackTrace != "" {
		rows = append(rows, workflowInfoRow{Key: "Stack Trace", Label: "Stack Trace", Value: ev.FailureStackTrace, Color: theme.FgDim(), ColorTag: theme.TagFgDim()})
	}
	if ev.FailureCause != "" {
		rows = append(rows, workflowInfoRow{Key: "Failure Cause", Label: "Cause", Value: ev.FailureCause, Color: theme.FgDim(), ColorTag: theme.TagFgDim()})
	}

	seen := map[string]bool{
		"id": true, "event id": true, "type": true, "name": true, "time": true,
		"activitytype": ev.ActivityType != "",
		"failure":      ev.Failure != "", "source": ev.FailureSource != "",
		"stacktrace": ev.FailureStackTrace != "", "stack trace": ev.FailureStackTrace != "",
		"cause": ev.FailureCause != "",
	}
	rows = append(rows, eventDetailAttributeRows(ev.Details, seen)...)
	if ev.Input != "" && !seen["input"] {
		rows = append(rows, workflowInfoRow{Key: "Input", Label: "Input", Value: ev.Input, Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	if ev.Result != "" && !seen["result"] {
		rows = append(rows, workflowInfoRow{Key: "Result", Label: "Result", Value: ev.Result, Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	return rows
}

func eventTreeInfoRows(node *temporal.EventTreeNode) []workflowInfoRow {
	if node == nil {
		return nil
	}
	status := temporal.GetWorkflowStatus(node.Status)
	durationStr := "running..."
	if node.Duration > 0 {
		durationStr = temporal.FormatDuration(node.Duration)
	}
	rows := []workflowInfoRow{
		{Key: eventInfoName, Label: "Name", Value: node.Name, Color: theme.Fg(), ColorTag: theme.TagFg()},
		{
			Key:      "Status",
			Label:    "Status",
			Value:    node.Status,
			Display:  status.Icon() + " " + node.Status,
			Color:    status.Color(),
			ColorTag: status.ColorTag(),
		},
		{Key: "Duration", Label: "Duration", Value: durationStr, Color: theme.Fg(), ColorTag: theme.TagFg()},
	}
	if !node.StartTime.IsZero() {
		rows = append(rows, workflowInfoRow{Key: "Start Time", Label: "Start Time", Value: node.StartTime.Format("2006-01-02 15:04:05.000"), Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	if node.Attempts > 1 {
		rows = append(rows, workflowInfoRow{Key: "Attempts", Label: "Attempts", Value: fmt.Sprintf("%d", node.Attempts), Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	for _, ev := range node.Events {
		if ev == nil {
			continue
		}
		if ev.Result != "" {
			rows = append(rows, workflowInfoRow{Key: "Result", Label: "Result", Value: ev.Result, Color: theme.Fg(), ColorTag: theme.TagFg()})
		}
		if ev.Failure != "" {
			rows = append(rows, workflowInfoRow{Key: "Failure", Label: "Failure", Value: ev.Failure, Color: theme.Error(), ColorTag: theme.TagError()})
		}
		if ev.FailureSource != "" {
			rows = append(rows, workflowInfoRow{Key: "Failure Source", Label: "Source", Value: ev.FailureSource, Color: theme.Fg(), ColorTag: theme.TagFg()})
		}
		if ev.FailureStackTrace != "" {
			rows = append(rows, workflowInfoRow{Key: "Stack Trace", Label: "Stack Trace", Value: ev.FailureStackTrace, Color: theme.FgDim(), ColorTag: theme.TagFgDim()})
		}
		if ev.FailureCause != "" {
			rows = append(rows, workflowInfoRow{Key: "Failure Cause", Label: "Cause", Value: ev.FailureCause, Color: theme.FgDim(), ColorTag: theme.TagFgDim()})
		}
	}
	return rows
}

func eventDetailAttributeRows(details string, seen map[string]bool) []workflowInfoRow {
	if details == "" {
		return nil
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	trimmed := strings.TrimSpace(details)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		seen["details"] = true
		return []workflowInfoRow{{Key: "Details", Label: "Details", Value: details, Color: theme.Fg(), ColorTag: theme.TagFg()}}
	}
	var rows []workflowInfoRow
	for _, part := range splitPreservingJSONWorkflow(details) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colonIdx := findKeyColonIndex(part)
		if colonIdx <= 0 {
			rows = append(rows, workflowInfoRow{Key: part, Label: part, Value: part, Color: theme.Fg(), ColorTag: theme.TagFg()})
			continue
		}
		key := strings.TrimSpace(part[:colonIdx])
		value := strings.TrimSpace(part[colonIdx+1:])
		if seen[strings.ToLower(key)] {
			continue
		}
		seen[strings.ToLower(key)] = true
		rows = append(rows, workflowInfoRow{Key: key, Label: key, Value: value, Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	return rows
}

func activityInfoRows(a previewActivity) []workflowInfoRow {
	status := temporal.GetActivityStatus(a.Status)
	name := a.Type
	if name == "" {
		name = "Activity"
	}
	activityID := a.ActivityID
	if activityID == "" {
		activityID = fmt.Sprintf("%d", a.ScheduledID)
	}
	attempt := "-"
	if a.Attempt > 0 {
		attempt = fmt.Sprintf("%d", a.Attempt)
	}
	end := "—"
	if a.EndTime != nil {
		end = a.EndTime.Format("2006-01-02 15:04:05.000")
	}

	rows := []workflowInfoRow{
		{Key: activityInfoName, Label: "Activity", Value: name, Color: theme.Fg(), ColorTag: theme.TagFg()},
		{Key: activityInfoID, Label: "ID", Value: activityID, Color: theme.Fg(), ColorTag: theme.TagFg()},
		{
			Key:      activityInfoStatus,
			Label:    "Status",
			Value:    a.Status,
			Display:  status.Icon() + " " + a.Status,
			Color:    status.Color(),
			ColorTag: status.ColorTag(),
		},
		{Key: activityInfoStarted, Label: "Started", Value: a.StartTime.Format("2006-01-02 15:04:05.000"), Color: theme.Fg(), ColorTag: theme.TagFg()},
		{Key: activityInfoEnded, Label: "Ended", Value: end, Color: theme.Fg(), ColorTag: theme.TagFg()},
		{Key: activityInfoDuration, Label: "Duration", Value: a.duration(), Color: theme.Fg(), ColorTag: theme.TagFg()},
		{Key: activityInfoAttempt, Label: "Attempt", Value: attempt, Color: theme.Fg(), ColorTag: theme.TagFg()},
	}
	if a.TaskQueue != "" {
		rows = append(rows, workflowInfoRow{Key: activityInfoTaskQueue, Label: "Task Queue", Value: a.TaskQueue, Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	if a.Identity != "" {
		rows = append(rows, workflowInfoRow{Key: activityInfoIdentity, Label: "Identity", Value: a.Identity, Color: theme.Fg(), ColorTag: theme.TagFg()})
	}
	return rows
}
