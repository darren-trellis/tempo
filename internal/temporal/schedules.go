package temporal

import (
	"context"
	"fmt"
	"strings"

	"go.temporal.io/sdk/client"
)

// ListSchedules returns all schedules in a namespace.
func (c *Client) ListSchedules(ctx context.Context, namespace string, opts ListOptions) ([]Schedule, string, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, "", err
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	resp, err := cl.ScheduleClient().List(ctx, client.ScheduleListOptions{
		PageSize: pageSize,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to list schedules: %w", err)
	}

	var schedules []Schedule
	for resp.HasNext() {
		entry, err := resp.Next()
		if err != nil {
			return nil, "", fmt.Errorf("failed to iterate schedules: %w", err)
		}

		schedule := Schedule{
			ID:           entry.ID,
			Paused:       entry.Paused,
			Notes:        entry.Note,
			WorkflowType: entry.WorkflowType.Name,
			RecentRuns:   convertScheduleRuns(entry.RecentActions),
		}

		// Extract spec info
		if entry.Spec != nil {
			schedule.Spec = formatScheduleSpec(entry.Spec)
		}

		// Recent and future actions
		if len(entry.RecentActions) > 0 {
			lastAction := entry.RecentActions[len(entry.RecentActions)-1]
			t := lastAction.ActualTime
			schedule.LastRunTime = &t
		}
		if len(entry.NextActionTimes) > 0 {
			t := entry.NextActionTimes[0]
			schedule.NextRunTime = &t
		}

		schedules = append(schedules, schedule)
	}

	return schedules, "", nil
}

// GetSchedule returns details for a specific schedule.
func (c *Client) GetSchedule(ctx context.Context, namespace, scheduleID string) (*Schedule, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	desc, err := handle.Describe(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to describe schedule: %w", err)
	}

	schedule := &Schedule{
		ID:     scheduleID,
		Paused: desc.Schedule.State.Paused,
		Notes:  desc.Schedule.State.Note,
	}

	// Extract workflow info from action
	if desc.Schedule.Action != nil {
		if startAction, ok := desc.Schedule.Action.(*client.ScheduleWorkflowAction); ok {
			// Workflow is an interface{} representing the workflow type
			if wfType, ok := startAction.Workflow.(string); ok {
				schedule.WorkflowType = wfType
			}
			schedule.WorkflowID = startAction.ID
			schedule.TaskQueue = startAction.TaskQueue
		}
	}

	// Extract spec info
	if desc.Schedule.Spec != nil {
		schedule.Spec = formatScheduleSpec(desc.Schedule.Spec)
	}

	// Info from description
	schedule.TotalActions = int64(desc.Info.NumActions)
	schedule.RecentRuns = convertScheduleRuns(desc.Info.RecentActions)
	if len(desc.Info.RecentActions) > 0 {
		lastAction := desc.Info.RecentActions[len(desc.Info.RecentActions)-1]
		t := lastAction.ActualTime
		schedule.LastRunTime = &t
	}
	if len(desc.Info.NextActionTimes) > 0 {
		t := desc.Info.NextActionTimes[0]
		schedule.NextRunTime = &t
	}

	return schedule, nil
}

// PauseSchedule pauses a schedule.
func (c *Client) PauseSchedule(ctx context.Context, namespace, scheduleID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Pause(ctx, client.SchedulePauseOptions{
		Note: reason,
	})
}

// UnpauseSchedule unpauses a schedule.
func (c *Client) UnpauseSchedule(ctx context.Context, namespace, scheduleID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Unpause(ctx, client.ScheduleUnpauseOptions{
		Note: reason,
	})
}

// TriggerSchedule immediately triggers a scheduled workflow execution.
func (c *Client) TriggerSchedule(ctx context.Context, namespace, scheduleID string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Trigger(ctx, client.ScheduleTriggerOptions{})
}

// DeleteSchedule permanently deletes a schedule.
func (c *Client) DeleteSchedule(ctx context.Context, namespace, scheduleID string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Delete(ctx)
}

func convertScheduleRuns(actions []client.ScheduleActionResult) []ScheduleRun {
	if len(actions) == 0 {
		return nil
	}

	runs := make([]ScheduleRun, 0, len(actions))
	for _, action := range actions {
		run := ScheduleRun{
			ScheduleTime: action.ScheduleTime,
			ActualTime:   action.ActualTime,
		}
		if action.StartWorkflowResult != nil {
			run.WorkflowID = action.StartWorkflowResult.WorkflowID
			run.RunID = action.StartWorkflowResult.FirstExecutionRunID
		}
		runs = append(runs, run)
	}

	return runs
}

// formatScheduleSpec creates a human-readable schedule specification.
func formatScheduleSpec(spec *client.ScheduleSpec) string {
	if spec == nil {
		return ""
	}

	var parts []string

	// Check for cron expressions
	if len(spec.CronExpressions) > 0 {
		parts = append(parts, spec.CronExpressions[0])
	}

	// Check for intervals
	if len(spec.Intervals) > 0 {
		interval := spec.Intervals[0]
		parts = append(parts, fmt.Sprintf("every %s", interval.Every))
	}

	// Check for calendars
	if len(spec.Calendars) > 0 {
		parts = append(parts, "calendar-based")
	}

	if len(parts) == 0 {
		return "custom"
	}

	return strings.Join(parts, ", ")
}
