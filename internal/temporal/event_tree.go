package temporal

import (
	"fmt"
	"strings"
	"time"
)

// EventGroupType represents the type of event grouping in the tree view.
type EventGroupType int

const (
	GroupWorkflow EventGroupType = iota
	GroupWorkflowTask
	GroupActivity
	GroupTimer
	GroupChildWorkflow
	GroupSignal
	GroupMarker
	GroupOther
)

// String returns a human-readable name for the group type.
func (g EventGroupType) String() string {
	switch g {
	case GroupWorkflow:
		return "Workflow"
	case GroupWorkflowTask:
		return "WorkflowTask"
	case GroupActivity:
		return "Activity"
	case GroupTimer:
		return "Timer"
	case GroupChildWorkflow:
		return "ChildWorkflow"
	case GroupSignal:
		return "Signal"
	case GroupMarker:
		return "Marker"
	default:
		return "Other"
	}
}

// EventTreeNode represents a node in the event tree.
type EventTreeNode struct {
	Name      string                  // Display name (e.g., "Activity: ValidateOrder")
	Type      EventGroupType          // Group type
	Status    string                  // Running, Completed, Failed, Canceled, TimedOut, Pending
	StartTime time.Time               // When this group started
	EndTime   *time.Time              // When this group ended (nil if still running)
	Duration  time.Duration           // Computed duration
	Events    []*EnhancedHistoryEvent // Raw events in this node
	Children  []*EventTreeNode        // Child nodes (for attempts/nested)
	Collapsed bool                    // UI state for expand/collapse
	Attempts  int                     // Number of retry attempts
}

// IsLeaf returns true if this node has no children.
func (n *EventTreeNode) IsLeaf() bool {
	return len(n.Children) == 0
}

// HasChildren returns true if this node has children.
func (n *EventTreeNode) HasChildren() bool {
	return len(n.Children) > 0
}

// BuildEventTree constructs a tree from a flat list of enhanced history events.
func BuildEventTree(events []EnhancedHistoryEvent) []*EventTreeNode {
	if len(events) == 0 {
		return nil
	}

	// Create index for O(1) lookups by event ID
	eventMap := make(map[int64]*EnhancedHistoryEvent)
	for i := range events {
		eventMap[events[i].ID] = &events[i]
	}

	var rootNodes []*EventTreeNode

	// Track which events have been processed
	processed := make(map[int64]bool)

	// Track activity groups by ScheduledEventID
	activityGroups := make(map[int64]*EventTreeNode)

	// Track timer groups by StartedEventID
	timerGroups := make(map[int64]*EventTreeNode)

	// Track child workflow groups by InitiatedEventID
	childWfGroups := make(map[int64]*EventTreeNode)

	// Track workflow task groups by ScheduledEventID
	wfTaskGroups := make(map[int64]*EventTreeNode)

	// First pass: identify group roots and build groups
	for i := range events {
		ev := &events[i]

		if processed[ev.ID] {
			continue
		}

		switch {
		// Workflow start event
		case ev.Type == "WorkflowExecutionStarted":
			node := &EventTreeNode{
				Name:      "Workflow Started",
				Type:      GroupWorkflow,
				Status:    "Running",
				StartTime: ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Workflow terminal events
		case strings.HasPrefix(ev.Type, "WorkflowExecution") && ev.Type != "WorkflowExecutionStarted" && ev.Type != "WorkflowExecutionSignaled":
			status := extractWorkflowStatus(ev.Type)
			node := &EventTreeNode{
				Name:      fmt.Sprintf("Workflow %s", status),
				Type:      GroupWorkflow,
				Status:    status,
				StartTime: ev.Time,
				EndTime:   &ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Activity Scheduled - creates a new activity group
		case ev.Type == "ActivityTaskScheduled":
			node := &EventTreeNode{
				Name:      fmt.Sprintf("Activity: %s", ev.ActivityType),
				Type:      GroupActivity,
				Status:    "Scheduled",
				StartTime: ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			activityGroups[ev.ID] = node
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Activity Started - links to Scheduled
		case ev.Type == "ActivityTaskStarted":
			if group, ok := activityGroups[ev.ScheduledEventID]; ok {
				group.Events = append(group.Events, ev)
				group.Status = "Running"
				if ev.Attempt > 1 {
					group.Attempts = int(ev.Attempt)
					// Create attempt child node
					attemptNode := &EventTreeNode{
						Name:      fmt.Sprintf("Attempt %d", ev.Attempt),
						Type:      GroupActivity,
						Status:    "Running",
						StartTime: ev.Time,
						Events:    []*EnhancedHistoryEvent{ev},
					}
					group.Children = append(group.Children, attemptNode)
				}
			}
			processed[ev.ID] = true

		// Activity terminal events
		case ev.Type == "ActivityTaskCompleted" || ev.Type == "ActivityTaskFailed" ||
			ev.Type == "ActivityTaskTimedOut" || ev.Type == "ActivityTaskCanceled":
			if group, ok := activityGroups[ev.ScheduledEventID]; ok {
				group.Events = append(group.Events, ev)
				group.Status = extractActivityStatus(ev.Type)
				group.EndTime = &ev.Time
				group.Duration = ev.Time.Sub(group.StartTime)

				// Update attempt child if exists
				if len(group.Children) > 0 {
					lastAttempt := group.Children[len(group.Children)-1]
					lastAttempt.Events = append(lastAttempt.Events, ev)
					lastAttempt.Status = group.Status
					lastAttempt.EndTime = &ev.Time
					lastAttempt.Duration = ev.Time.Sub(lastAttempt.StartTime)
				}
			}
			processed[ev.ID] = true

		// Timer Started - creates a new timer group
		case ev.Type == "TimerStarted":
			node := &EventTreeNode{
				Name:      fmt.Sprintf("Timer: %s", ev.TimerID),
				Type:      GroupTimer,
				Status:    "Running",
				StartTime: ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			timerGroups[ev.ID] = node
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Timer terminal events
		case ev.Type == "TimerFired" || ev.Type == "TimerCanceled":
			if group, ok := timerGroups[ev.StartedEventID]; ok {
				group.Events = append(group.Events, ev)
				if ev.Type == "TimerFired" {
					group.Status = "Fired"
				} else {
					group.Status = "Canceled"
				}
				group.EndTime = &ev.Time
				group.Duration = ev.Time.Sub(group.StartTime)
			}
			processed[ev.ID] = true

		// Child Workflow Initiated - creates a new child workflow group
		case ev.Type == "StartChildWorkflowExecutionInitiated":
			node := &EventTreeNode{
				Name:      fmt.Sprintf("ChildWorkflow: %s", ev.ChildWorkflowType),
				Type:      GroupChildWorkflow,
				Status:    "Initiated",
				StartTime: ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			childWfGroups[ev.ID] = node
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Child Workflow Started
		case ev.Type == "ChildWorkflowExecutionStarted":
			if group, ok := childWfGroups[ev.InitiatedEventID]; ok {
				group.Events = append(group.Events, ev)
				group.Status = "Running"
			}
			processed[ev.ID] = true

		// Child Workflow terminal events
		case strings.HasPrefix(ev.Type, "ChildWorkflowExecution") && ev.Type != "ChildWorkflowExecutionStarted":
			if group, ok := childWfGroups[ev.InitiatedEventID]; ok {
				group.Events = append(group.Events, ev)
				group.Status = extractChildWorkflowStatus(ev.Type)
				group.EndTime = &ev.Time
				group.Duration = ev.Time.Sub(group.StartTime)
			}
			processed[ev.ID] = true

		// Workflow Task Scheduled
		case ev.Type == "WorkflowTaskScheduled":
			node := &EventTreeNode{
				Name:      "WorkflowTask",
				Type:      GroupWorkflowTask,
				Status:    "Scheduled",
				StartTime: ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			wfTaskGroups[ev.ID] = node
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Workflow Task Started
		case ev.Type == "WorkflowTaskStarted":
			if group, ok := wfTaskGroups[ev.ScheduledEventID]; ok {
				group.Events = append(group.Events, ev)
				group.Status = "Running"
			}
			processed[ev.ID] = true

		// Workflow Task terminal events
		case ev.Type == "WorkflowTaskCompleted" || ev.Type == "WorkflowTaskFailed" || ev.Type == "WorkflowTaskTimedOut":
			if group, ok := wfTaskGroups[ev.ScheduledEventID]; ok {
				group.Events = append(group.Events, ev)
				group.Status = extractWorkflowTaskStatus(ev.Type)
				group.EndTime = &ev.Time
				group.Duration = ev.Time.Sub(group.StartTime)
			}
			processed[ev.ID] = true

		// Signal events
		case ev.Type == "WorkflowExecutionSignaled":
			node := &EventTreeNode{
				Name:      "Signal Received",
				Type:      GroupSignal,
				Status:    "Received",
				StartTime: ev.Time,
				EndTime:   &ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Marker events
		case ev.Type == "MarkerRecorded":
			node := &EventTreeNode{
				Name:      "Marker",
				Type:      GroupMarker,
				Status:    "Recorded",
				StartTime: ev.Time,
				EndTime:   &ev.Time,
				Events:    []*EnhancedHistoryEvent{ev},
			}
			rootNodes = append(rootNodes, node)
			processed[ev.ID] = true

		// Other unhandled events
		default:
			if !processed[ev.ID] {
				node := &EventTreeNode{
					Name:      ev.Type,
					Type:      GroupOther,
					Status:    "Unknown",
					StartTime: ev.Time,
					Events:    []*EnhancedHistoryEvent{ev},
				}
				rootNodes = append(rootNodes, node)
				processed[ev.ID] = true
			}
		}
	}

	return rootNodes
}

// extractWorkflowStatus extracts status from workflow terminal event type.
func extractWorkflowStatus(eventType string) string {
	switch eventType {
	case "WorkflowExecutionCompleted":
		return "Completed"
	case "WorkflowExecutionFailed":
		return "Failed"
	case "WorkflowExecutionTimedOut":
		return "TimedOut"
	case "WorkflowExecutionCanceled":
		return "Canceled"
	case "WorkflowExecutionTerminated":
		return "Terminated"
	case "WorkflowExecutionContinuedAsNew":
		return "ContinuedAsNew"
	default:
		return "Unknown"
	}
}

// extractActivityStatus extracts status from activity terminal event type.
func extractActivityStatus(eventType string) string {
	switch eventType {
	case "ActivityTaskCompleted":
		return "Completed"
	case "ActivityTaskFailed":
		return "Failed"
	case "ActivityTaskTimedOut":
		return "TimedOut"
	case "ActivityTaskCanceled":
		return "Canceled"
	default:
		return "Unknown"
	}
}

// extractChildWorkflowStatus extracts status from child workflow terminal event type.
func extractChildWorkflowStatus(eventType string) string {
	switch eventType {
	case "ChildWorkflowExecutionCompleted":
		return "Completed"
	case "ChildWorkflowExecutionFailed":
		return "Failed"
	case "ChildWorkflowExecutionTimedOut":
		return "TimedOut"
	case "ChildWorkflowExecutionCanceled":
		return "Canceled"
	case "ChildWorkflowExecutionTerminated":
		return "Terminated"
	default:
		return "Unknown"
	}
}

// extractWorkflowTaskStatus extracts status from workflow task terminal event type.
func extractWorkflowTaskStatus(eventType string) string {
	switch eventType {
	case "WorkflowTaskCompleted":
		return "Completed"
	case "WorkflowTaskFailed":
		return "Failed"
	case "WorkflowTaskTimedOut":
		return "TimedOut"
	default:
		return "Unknown"
	}
}

// FormatDuration formats a duration for display.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
