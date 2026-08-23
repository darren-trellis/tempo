package temporal

import (
	"github.com/atterpac/jig/theme"
	"go.temporal.io/api/enums/v1"
)

// Typed workflow status handles - use these for compile-time safe color/icon access.
var (
	StatusRunning    = theme.DefineStatus("Running", theme.Info, theme.IconRunning)
	StatusCompleted  = theme.DefineStatus("Completed", theme.Success, theme.IconCompleted)
	StatusFailed     = theme.DefineStatus("Failed", theme.Error, theme.IconFailed)
	StatusCanceled   = theme.DefineStatus("Canceled", theme.Warning, theme.IconCanceled)
	StatusTerminated = theme.DefineStatus("Terminated", theme.Error, theme.IconStop)
	StatusTimedOut   = theme.DefineStatus("TimedOut", theme.Warning, theme.IconTimedOut)
	StatusUnknown    = theme.DefineStatus("Unknown", theme.FgDim, theme.IconPending)
	StatusScheduled  = theme.DefineStatus("Scheduled", theme.FgDim, theme.IconPending)
)

// GetActivityStatus returns the typed Status for an activity status string.
func GetActivityStatus(status string) *theme.Status {
	if status == "Scheduled" {
		return StatusScheduled
	}
	return GetWorkflowStatus(status)
}

// MapWorkflowStatus converts a Temporal SDK workflow execution status to a display string.
func MapWorkflowStatus(status enums.WorkflowExecutionStatus) string {
	switch status {
	case enums.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return "Running"
	case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return "Completed"
	case enums.WORKFLOW_EXECUTION_STATUS_FAILED:
		return "Failed"
	case enums.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return "Canceled"
	case enums.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return "Terminated"
	case enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return "TimedOut"
	case enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return "Completed" // Treat ContinuedAsNew as completed for display
	default:
		return "Unknown"
	}
}

// GetWorkflowStatus returns the typed Status for a workflow status string.
func GetWorkflowStatus(status string) *theme.Status {
	switch status {
	case "Running":
		return StatusRunning
	case "Completed":
		return StatusCompleted
	case "Failed":
		return StatusFailed
	case "Canceled":
		return StatusCanceled
	case "Terminated":
		return StatusTerminated
	case "TimedOut":
		return StatusTimedOut
	default:
		return StatusUnknown
	}
}

// Typed namespace state handles.
var (
	NamespaceStateActive     = theme.DefineStatus("Active", theme.Success, theme.IconCheck)
	NamespaceStateDeprecated = theme.DefineStatus("Deprecated", theme.Warning, theme.IconWarning)
	NamespaceStateDeleted    = theme.DefineStatus("Deleted", theme.Error, theme.IconDelete)
	NamespaceStateUnknown    = theme.DefineStatus("Unknown", theme.FgDim, theme.IconPending)
)

// MapNamespaceState converts a Temporal SDK namespace state to a display string.
func MapNamespaceState(state enums.NamespaceState) string {
	switch state {
	case enums.NAMESPACE_STATE_REGISTERED:
		return "Active"
	case enums.NAMESPACE_STATE_DEPRECATED:
		return "Deprecated"
	case enums.NAMESPACE_STATE_DELETED:
		return "Deleted"
	default:
		return "Unknown"
	}
}

// GetNamespaceState returns the typed Status for a namespace state string.
func GetNamespaceState(state string) *theme.Status {
	switch state {
	case "Active":
		return NamespaceStateActive
	case "Deprecated":
		return NamespaceStateDeprecated
	case "Deleted":
		return NamespaceStateDeleted
	default:
		return NamespaceStateUnknown
	}
}

// Task queue type handles.
var (
	TaskQueueTypeWorkflowStatus = theme.DefineStatus("Workflow", theme.Info, theme.IconWorkflow)
	TaskQueueTypeActivityStatus = theme.DefineStatus("Activity", theme.Info, theme.IconActivity)
	TaskQueueTypeUnknownStatus  = theme.DefineStatus("Unknown", theme.FgDim, theme.IconPending)
)

// Task queue type string constants (for data storage).
const (
	TaskQueueTypeWorkflow = "Workflow"
	TaskQueueTypeActivity = "Activity"
)

// MapTaskQueueType converts a Temporal SDK task queue type to a display string.
func MapTaskQueueType(tqType enums.TaskQueueType) string {
	switch tqType {
	case enums.TASK_QUEUE_TYPE_WORKFLOW:
		return TaskQueueTypeWorkflow
	case enums.TASK_QUEUE_TYPE_ACTIVITY:
		return TaskQueueTypeActivity
	default:
		return "Unknown"
	}
}

// GetTaskQueueTypeStatus returns the typed Status for a task queue type string.
func GetTaskQueueTypeStatus(tqType string) *theme.Status {
	switch tqType {
	case TaskQueueTypeWorkflow:
		return TaskQueueTypeWorkflowStatus
	case TaskQueueTypeActivity:
		return TaskQueueTypeActivityStatus
	default:
		return TaskQueueTypeUnknownStatus
	}
}

var (
	WorkerStatusRunningHandle      = theme.DefineStatus("Running", theme.Success, theme.IconRunning)
	WorkerStatusShuttingDownHandle = theme.DefineStatus("Shutting Down", theme.Warning, theme.IconWarning)
	WorkerStatusShutdownHandle     = theme.DefineStatus("Shutdown", theme.FgDim, theme.IconStop)
)

const (
	WorkerStatusRunning      = "Running"
	WorkerStatusShuttingDown = "Shutting Down"
	WorkerStatusShutdown     = "Shutdown"
)

func MapWorkerStatus(status enums.WorkerStatus) string {
	switch status {
	case enums.WORKER_STATUS_RUNNING:
		return WorkerStatusRunning
	case enums.WORKER_STATUS_SHUTTING_DOWN:
		return WorkerStatusShuttingDown
	case enums.WORKER_STATUS_SHUTDOWN:
		return WorkerStatusShutdown
	default:
		return "Unknown"
	}
}

func GetWorkerStatus(status string) *theme.Status {
	switch status {
	case WorkerStatusRunning:
		return WorkerStatusRunningHandle
	case WorkerStatusShuttingDown:
		return WorkerStatusShuttingDownHandle
	case WorkerStatusShutdown:
		return WorkerStatusShutdownHandle
	default:
		return StatusUnknown
	}
}
