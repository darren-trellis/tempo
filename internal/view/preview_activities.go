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
