package temporal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// GetWorkflowHistory returns the event history for a workflow execution.
func (c *Client) GetWorkflowHistory(ctx context.Context, namespace, workflowID, runID string) ([]HistoryEvent, error) {
	var events []HistoryEvent
	_, err := c.forEachHistoryEvent(ctx, namespace, workflowID, runID, nil, func(event *historypb.HistoryEvent) {
		events = append(events, HistoryEvent{
			ID:      event.GetEventId(),
			Type:    formatEventType(event.GetEventType().String()),
			Time:    event.GetEventTime().AsTime(),
			Details: extractEventDetails(event),
		})
	})
	return events, err
}

// GetEnhancedWorkflowHistory returns event history with relational data for tree/timeline views.
// Events are immutable once written, so a run loaded before only has its new
// events sent through the codec; the rest are reused as they were decoded.
func (c *Client) GetEnhancedWorkflowHistory(ctx context.Context, namespace, workflowID, runID string) ([]EnhancedHistoryEvent, error) {
	key := historyCacheKey(namespace, workflowID, runID)
	known := c.histories.get(key)
	reuse := func(event *historypb.HistoryEvent) (EnhancedHistoryEvent, bool) {
		i := int(event.GetEventId()) - 1
		if i >= 0 && i < len(known) && known[i].ID == event.GetEventId() {
			return known[i], true
		}
		return EnhancedHistoryEvent{}, false
	}
	var events []EnhancedHistoryEvent
	decoded, err := c.forEachHistoryEvent(ctx, namespace, workflowID, runID,
		func(event *historypb.HistoryEvent) bool {
			_, ok := reuse(event)
			return ok
		},
		func(event *historypb.HistoryEvent) {
			if ev, ok := reuse(event); ok {
				events = append(events, ev)
				return
			}
			events = append(events, extractEnhancedEvent(event))
		})
	// Events read while the codec was failing are still encoded; keeping
	// them would keep them encoded on every later load.
	if err == nil && decoded {
		c.histories.put(key, events)
	}
	return events, err
}

// historyDecodeConcurrency is how many history pages are decoded at once,
// while later pages are still being fetched.
const historyDecodeConcurrency = 3

// forEachHistoryEvent pages through a run's history, decoding payloads through
// the namespace's codec before handing each event over in order. Pages are
// decoded concurrently while the next ones are fetched. Events skip reports
// true for are already known and are not decoded. decoded reports whether
// every payload sent to the codec came back decoded.
func (c *Client) forEachHistoryEvent(ctx context.Context, namespace, workflowID, runID string, skip func(*historypb.HistoryEvent) bool, visit func(*historypb.HistoryEvent)) (decoded bool, err error) {
	cl, err := c.conn()
	if err != nil {
		return false, err
	}
	codec := c.payloadCodec(namespace)
	var (
		t         historyLoadTiming
		tMu       sync.Mutex
		wg        sync.WaitGroup
		pages     [][]*historypb.HistoryEvent
		failed    atomic.Bool
		sem       = make(chan struct{}, historyDecodeConcurrency)
		pageToken []byte
	)
	start := time.Now()
	defer func() {
		t.total = time.Since(start)
		t.log(workflowID)
	}()
	for {
		fetchStart := time.Now()
		resp, err := cl.WorkflowService().GetWorkflowExecutionHistory(ctx, &workflowservice.GetWorkflowExecutionHistoryRequest{
			Namespace: namespace,
			Execution: &commonpb.WorkflowExecution{
				WorkflowId: workflowID,
				RunId:      runID,
			},
			NextPageToken: pageToken,
		})
		if err != nil {
			return false, fmt.Errorf("failed to get workflow history: %w", err)
		}
		page := resp.GetHistory().GetEvents()
		tMu.Lock()
		t.fetch += time.Since(fetchStart)
		t.pages++
		t.events += len(page)
		tMu.Unlock()
		pages = append(pages, page)

		var fresh []*historypb.HistoryEvent
		for _, event := range page {
			if skip == nil || !skip(event) {
				fresh = append(fresh, event)
			}
		}
		tMu.Lock()
		t.reused += len(page) - len(fresh)
		tMu.Unlock()
		if len(fresh) > 0 {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				decodeStart := time.Now()
				payloads, bytes, ok := decodePayloadsInMessages(codec, asProtoMessages(fresh)...)
				if !ok {
					failed.Store(true)
				}
				tMu.Lock()
				t.decode += time.Since(decodeStart)
				t.payloads += payloads
				t.payloadBytes += bytes
				tMu.Unlock()
			}()
		}

		pageToken = resp.GetNextPageToken()
		if len(pageToken) == 0 {
			break
		}
	}
	waitStart := time.Now()
	wg.Wait()
	t.decodeWait = time.Since(waitStart)

	visitStart := time.Now()
	for _, page := range pages {
		for _, event := range page {
			visit(event)
		}
	}
	t.visit = time.Since(visitStart)
	return !failed.Load(), nil
}

// historyLoadTiming is where a history load spent its time, written to the
// log so a slow preview can be pinned on the server, the codec server, or
// tempo's own formatting.
type historyLoadTiming struct {
	pages, events, reused, payloads, payloadBytes int
	// decode adds up every page's codec call, which overlap; decodeWait is
	// how long the load still waited on them after the last page arrived.
	fetch, decode, decodeWait, visit, total time.Duration
}

func (t historyLoadTiming) log(workflowID string) {
	if sdkLogger == nil {
		return
	}
	sdkLogger.Info("history load",
		"workflow", workflowID,
		"pages", t.pages,
		"events", t.events,
		"reused", t.reused,
		"payloads", t.payloads,
		"payloadKB", t.payloadBytes/1024,
		"fetch", t.fetch.Round(time.Millisecond),
		"decode", t.decode.Round(time.Millisecond),
		"decodeWait", t.decodeWait.Round(time.Millisecond),
		"format", t.visit.Round(time.Millisecond),
		"total", t.total.Round(time.Millisecond),
	)
}

// extractEnhancedEvent extracts structured data from a history event for tree/timeline views.
func extractEnhancedEvent(event *historypb.HistoryEvent) EnhancedHistoryEvent {
	he := EnhancedHistoryEvent{
		ID:      event.GetEventId(),
		Type:    formatEventType(event.GetEventType().String()),
		Time:    event.GetEventTime().AsTime(),
		Details: extractEventDetails(event),
	}

	switch event.GetEventType() {
	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
		a := event.GetWorkflowExecutionStartedEventAttributes()
		he.TaskQueue = a.GetTaskQueue().GetName()
		he.Identity = a.GetIdentity()
		he.Attempt = a.GetAttempt()
		he.Input = formatPayloads(a.GetInput())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:
		he.Result = formatPayloads(event.GetWorkflowExecutionCompletedEventAttributes().GetResult())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:
		a := event.GetWorkflowExecutionFailedEventAttributes()
		if a.GetFailure() != nil {
			a = proto.Clone(a).(*historypb.WorkflowExecutionFailedEventAttributes)
			decodeEncodedFailures(a.GetFailure())
			populateFailureDetails(&he, a.GetFailure())
			he.FailureJSON = eventAttributesJSON(a)
		}

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:
		he.Result = formatPayloads(event.GetWorkflowExecutionCanceledEventAttributes().GetDetails())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED:
		if a := event.GetWorkflowExecutionTerminatedEventAttributes(); a != nil {
			he.Failure = a.GetReason()
			he.FailureJSON = eventAttributesJSON(a)
		}

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:
		he.Failure = "Workflow timed out"
		if a := event.GetWorkflowExecutionTimedOutEventAttributes(); a != nil {
			he.FailureJSON = eventAttributesJSON(a)
		}

	case enums.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED:
		he.TaskQueue = event.GetWorkflowTaskScheduledEventAttributes().GetTaskQueue().GetName()

	case enums.EVENT_TYPE_WORKFLOW_TASK_STARTED:
		a := event.GetWorkflowTaskStartedEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.Identity = a.GetIdentity()

	case enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED:
		a := event.GetWorkflowTaskCompletedEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.StartedEventID = a.GetStartedEventId()
		he.Identity = a.GetIdentity()

	case enums.EVENT_TYPE_WORKFLOW_TASK_TIMED_OUT:
		a := event.GetWorkflowTaskTimedOutEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.StartedEventID = a.GetStartedEventId()

	case enums.EVENT_TYPE_WORKFLOW_TASK_FAILED:
		a := event.GetWorkflowTaskFailedEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		populateFailureDetails(&he, a.GetFailure())

	case enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
		a := event.GetActivityTaskScheduledEventAttributes()
		he.ActivityID = a.GetActivityId()
		he.ActivityType = a.GetActivityType().GetName()
		he.TaskQueue = a.GetTaskQueue().GetName()
		he.Input = formatPayloads(a.GetInput())

	case enums.EVENT_TYPE_ACTIVITY_TASK_STARTED:
		a := event.GetActivityTaskStartedEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.Attempt = a.GetAttempt()
		he.Identity = a.GetIdentity()
		populateFailureDetails(&he, a.GetLastFailure())

	case enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
		a := event.GetActivityTaskCompletedEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.StartedEventID = a.GetStartedEventId()
		he.Identity = a.GetIdentity()
		he.Result = formatPayloads(a.GetResult())

	case enums.EVENT_TYPE_ACTIVITY_TASK_FAILED:
		a := event.GetActivityTaskFailedEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.StartedEventID = a.GetStartedEventId()
		populateFailureDetails(&he, a.GetFailure())

	case enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT:
		a := event.GetActivityTaskTimedOutEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.StartedEventID = a.GetStartedEventId()
		populateFailureDetails(&he, a.GetFailure())

	case enums.EVENT_TYPE_ACTIVITY_TASK_CANCEL_REQUESTED:
		he.ScheduledEventID = event.GetActivityTaskCancelRequestedEventAttributes().GetScheduledEventId()

	case enums.EVENT_TYPE_ACTIVITY_TASK_CANCELED:
		a := event.GetActivityTaskCanceledEventAttributes()
		he.ScheduledEventID = a.GetScheduledEventId()
		he.StartedEventID = a.GetStartedEventId()

	case enums.EVENT_TYPE_TIMER_STARTED:
		he.TimerID = event.GetTimerStartedEventAttributes().GetTimerId()

	case enums.EVENT_TYPE_TIMER_FIRED:
		a := event.GetTimerFiredEventAttributes()
		he.TimerID = a.GetTimerId()
		he.StartedEventID = a.GetStartedEventId()

	case enums.EVENT_TYPE_TIMER_CANCELED:
		a := event.GetTimerCanceledEventAttributes()
		he.TimerID = a.GetTimerId()
		he.StartedEventID = a.GetStartedEventId()

	case enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED:
		a := event.GetStartChildWorkflowExecutionInitiatedEventAttributes()
		he.ChildWorkflowID = a.GetWorkflowId()
		he.ChildWorkflowType = a.GetWorkflowType().GetName()
		he.TaskQueue = a.GetTaskQueue().GetName()

	case enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_FAILED:
		a := event.GetStartChildWorkflowExecutionFailedEventAttributes()
		he.InitiatedEventID = a.GetInitiatedEventId()
		he.ChildWorkflowID = a.GetWorkflowId()
		he.ChildWorkflowType = a.GetWorkflowType().GetName()

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_STARTED:
		a := event.GetChildWorkflowExecutionStartedEventAttributes()
		he.setChild(a.GetInitiatedEventId(), a.GetWorkflowExecution())
		he.ChildWorkflowType = a.GetWorkflowType().GetName()

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_COMPLETED:
		a := event.GetChildWorkflowExecutionCompletedEventAttributes()
		he.setChild(a.GetInitiatedEventId(), a.GetWorkflowExecution())
		he.Result = formatPayloads(a.GetResult())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_FAILED:
		a := event.GetChildWorkflowExecutionFailedEventAttributes()
		he.setChild(a.GetInitiatedEventId(), a.GetWorkflowExecution())
		populateFailureDetails(&he, a.GetFailure())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_CANCELED:
		a := event.GetChildWorkflowExecutionCanceledEventAttributes()
		he.setChild(a.GetInitiatedEventId(), a.GetWorkflowExecution())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TIMED_OUT:
		a := event.GetChildWorkflowExecutionTimedOutEventAttributes()
		he.setChild(a.GetInitiatedEventId(), a.GetWorkflowExecution())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TERMINATED:
		a := event.GetChildWorkflowExecutionTerminatedEventAttributes()
		he.setChild(a.GetInitiatedEventId(), a.GetWorkflowExecution())

	case enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED:
		he.ChildWorkflowID = event.GetSignalExternalWorkflowExecutionInitiatedEventAttributes().GetWorkflowExecution().GetWorkflowId()

	case enums.EVENT_TYPE_EXTERNAL_WORKFLOW_EXECUTION_SIGNALED:
		a := event.GetExternalWorkflowExecutionSignaledEventAttributes()
		he.InitiatedEventID = a.GetInitiatedEventId()
		he.ChildWorkflowID = a.GetWorkflowExecution().GetWorkflowId()
	}

	return he
}

func (he *EnhancedHistoryEvent) setChild(initiatedEventID int64, execution *commonpb.WorkflowExecution) {
	he.InitiatedEventID = initiatedEventID
	he.ChildWorkflowID = execution.GetWorkflowId()
	he.ChildRunID = execution.GetRunId()
}

// eventDetails builds the "Key: value, Key: value" summary views parse back
// apart, so keys and separators are part of its contract.
type eventDetails []string

func (d *eventDetails) add(key string, value any) {
	*d = append(*d, fmt.Sprintf("%s: %v", key, value))
}

func (d *eventDetails) text(key, value string) {
	if value != "" {
		d.add(key, value)
	}
}

func (d *eventDetails) duration(key string, value *durationpb.Duration) {
	if value != nil {
		d.add(key, value.AsDuration())
	}
}

// extractEventDetails extracts a verbose summary string from a history event.
func extractEventDetails(event *historypb.HistoryEvent) string {
	var d eventDetails

	switch event.GetEventType() {
	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
		a := event.GetWorkflowExecutionStartedEventAttributes()
		d.text("WorkflowType", a.GetWorkflowType().GetName())
		d.text("TaskQueue", a.GetTaskQueue().GetName())
		d.text("Input", formatPayloads(a.GetInput()))
		d.duration("ExecutionTimeout", a.GetWorkflowExecutionTimeout())
		d.duration("RunTimeout", a.GetWorkflowRunTimeout())
		d.duration("TaskTimeout", a.GetWorkflowTaskTimeout())
		d.text("Identity", a.GetIdentity())
		if a.GetAttempt() > 1 {
			d.add("Attempt", a.GetAttempt())
		}

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:
		d.text("Result", formatPayloads(event.GetWorkflowExecutionCompletedEventAttributes().GetResult()))

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:
		a := event.GetWorkflowExecutionFailedEventAttributes()
		d.text("Failure", a.GetFailure().GetMessage())
		if trace := a.GetFailure().GetStackTrace(); trace != "" {
			if len(trace) > 200 {
				trace = trace[:200] + "..."
			}
			d.add("StackTrace", trace)
		}
		d.add("RetryState", a.GetRetryState().String())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:
		d.add("RetryState", event.GetWorkflowExecutionTimedOutEventAttributes().GetRetryState().String())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:
		d.text("Details", formatPayloads(event.GetWorkflowExecutionCanceledEventAttributes().GetDetails()))

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED:
		a := event.GetWorkflowExecutionTerminatedEventAttributes()
		d.text("Reason", a.GetReason())
		d.text("Identity", a.GetIdentity())

	case enums.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED:
		a := event.GetWorkflowTaskScheduledEventAttributes()
		d.text("TaskQueue", a.GetTaskQueue().GetName())
		d.duration("StartToCloseTimeout", a.GetStartToCloseTimeout())

	case enums.EVENT_TYPE_WORKFLOW_TASK_STARTED:
		a := event.GetWorkflowTaskStartedEventAttributes()
		d.text("Identity", a.GetIdentity())
		d.add("ScheduledEventId", a.GetScheduledEventId())

	case enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED:
		a := event.GetWorkflowTaskCompletedEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("StartedEventId", a.GetStartedEventId())
		d.text("Identity", a.GetIdentity())

	case enums.EVENT_TYPE_WORKFLOW_TASK_TIMED_OUT:
		a := event.GetWorkflowTaskTimedOutEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("StartedEventId", a.GetStartedEventId())
		d.add("TimeoutType", a.GetTimeoutType().String())

	case enums.EVENT_TYPE_WORKFLOW_TASK_FAILED:
		a := event.GetWorkflowTaskFailedEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("Cause", a.GetCause().String())
		d.text("Failure", a.GetFailure().GetMessage())

	case enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
		a := event.GetActivityTaskScheduledEventAttributes()
		d.text("ActivityType", a.GetActivityType().GetName())
		d.text("ActivityId", a.GetActivityId())
		d.text("TaskQueue", a.GetTaskQueue().GetName())
		d.text("Input", formatPayloads(a.GetInput()))
		d.duration("ScheduleToCloseTimeout", a.GetScheduleToCloseTimeout())
		d.duration("ScheduleToStartTimeout", a.GetScheduleToStartTimeout())
		d.duration("StartToCloseTimeout", a.GetStartToCloseTimeout())
		if rp := a.GetRetryPolicy(); rp != nil {
			d.add("RetryPolicy", fmt.Sprintf("MaxAttempts=%d", rp.GetMaximumAttempts()))
		}

	case enums.EVENT_TYPE_ACTIVITY_TASK_STARTED:
		a := event.GetActivityTaskStartedEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("Attempt", a.GetAttempt())
		d.text("Identity", a.GetIdentity())

	case enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
		a := event.GetActivityTaskCompletedEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("StartedEventId", a.GetStartedEventId())
		d.text("Result", formatPayloads(a.GetResult()))
		d.text("Identity", a.GetIdentity())

	case enums.EVENT_TYPE_ACTIVITY_TASK_FAILED:
		a := event.GetActivityTaskFailedEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("StartedEventId", a.GetStartedEventId())
		d.text("Failure", a.GetFailure().GetMessage())
		d.add("RetryState", a.GetRetryState().String())

	case enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT:
		a := event.GetActivityTaskTimedOutEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("StartedEventId", a.GetStartedEventId())
		d.text("TimeoutType", a.GetFailure().GetMessage())
		d.add("RetryState", a.GetRetryState().String())

	case enums.EVENT_TYPE_ACTIVITY_TASK_CANCEL_REQUESTED:
		d.add("ScheduledEventId", event.GetActivityTaskCancelRequestedEventAttributes().GetScheduledEventId())

	case enums.EVENT_TYPE_ACTIVITY_TASK_CANCELED:
		a := event.GetActivityTaskCanceledEventAttributes()
		d.add("ScheduledEventId", a.GetScheduledEventId())
		d.add("StartedEventId", a.GetStartedEventId())
		d.text("Details", formatPayloads(a.GetDetails()))

	case enums.EVENT_TYPE_TIMER_STARTED:
		a := event.GetTimerStartedEventAttributes()
		d.text("TimerId", a.GetTimerId())
		d.duration("StartToFireTimeout", a.GetStartToFireTimeout())

	case enums.EVENT_TYPE_TIMER_FIRED:
		a := event.GetTimerFiredEventAttributes()
		d.text("TimerId", a.GetTimerId())
		d.add("StartedEventId", a.GetStartedEventId())

	case enums.EVENT_TYPE_TIMER_CANCELED:
		a := event.GetTimerCanceledEventAttributes()
		d.text("TimerId", a.GetTimerId())
		d.add("StartedEventId", a.GetStartedEventId())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED:
		a := event.GetWorkflowExecutionSignaledEventAttributes()
		d.text("SignalName", a.GetSignalName())
		d.text("Input", formatPayloads(a.GetInput()))
		d.text("Identity", a.GetIdentity())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_ACCEPTED:
		d.text("UpdateId", event.GetWorkflowExecutionUpdateAcceptedEventAttributes().GetAcceptedRequest().GetMeta().GetUpdateId())

	case enums.EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_COMPLETED:
		d.text("UpdateId", event.GetWorkflowExecutionUpdateCompletedEventAttributes().GetMeta().GetUpdateId())

	case enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED:
		a := event.GetStartChildWorkflowExecutionInitiatedEventAttributes()
		d.text("WorkflowType", a.GetWorkflowType().GetName())
		d.text("WorkflowId", a.GetWorkflowId())
		d.text("TaskQueue", a.GetTaskQueue().GetName())
		d.text("Input", formatPayloads(a.GetInput()))

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_STARTED:
		a := event.GetChildWorkflowExecutionStartedEventAttributes()
		d.text("WorkflowType", a.GetWorkflowType().GetName())
		d.text("WorkflowId", a.GetWorkflowExecution().GetWorkflowId())
		d.text("RunId", a.GetWorkflowExecution().GetRunId())
		d.add("InitiatedEventId", a.GetInitiatedEventId())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_COMPLETED:
		a := event.GetChildWorkflowExecutionCompletedEventAttributes()
		d.text("WorkflowId", a.GetWorkflowExecution().GetWorkflowId())
		d.text("Result", formatPayloads(a.GetResult()))
		d.add("InitiatedEventId", a.GetInitiatedEventId())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_FAILED:
		a := event.GetChildWorkflowExecutionFailedEventAttributes()
		d.text("WorkflowId", a.GetWorkflowExecution().GetWorkflowId())
		d.text("Failure", a.GetFailure().GetMessage())
		d.add("InitiatedEventId", a.GetInitiatedEventId())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_CANCELED:
		a := event.GetChildWorkflowExecutionCanceledEventAttributes()
		d.childWorkflow(a.GetWorkflowExecution(), a.GetInitiatedEventId())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TIMED_OUT:
		a := event.GetChildWorkflowExecutionTimedOutEventAttributes()
		d.childWorkflow(a.GetWorkflowExecution(), a.GetInitiatedEventId())

	case enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_TERMINATED:
		a := event.GetChildWorkflowExecutionTerminatedEventAttributes()
		d.childWorkflow(a.GetWorkflowExecution(), a.GetInitiatedEventId())

	case enums.EVENT_TYPE_MARKER_RECORDED:
		d.text("MarkerName", event.GetMarkerRecordedEventAttributes().GetMarkerName())

	case enums.EVENT_TYPE_EXTERNAL_WORKFLOW_EXECUTION_SIGNALED:
		a := event.GetExternalWorkflowExecutionSignaledEventAttributes()
		d.childWorkflow(a.GetWorkflowExecution(), a.GetInitiatedEventId())

	case enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED:
		a := event.GetSignalExternalWorkflowExecutionInitiatedEventAttributes()
		d.text("WorkflowId", a.GetWorkflowExecution().GetWorkflowId())
		d.text("SignalName", a.GetSignalName())
		d.text("Input", formatPayloads(a.GetInput()))

	default:
		d.add("EventType", event.GetEventType().String())
	}

	return strings.Join(d, ", ")
}

func (d *eventDetails) childWorkflow(execution *commonpb.WorkflowExecution, initiatedEventID int64) {
	d.text("WorkflowId", execution.GetWorkflowId())
	d.add("InitiatedEventId", initiatedEventID)
}

// formatEventType cleans up the event type string for display
func formatEventType(eventType string) string {
	// Remove EVENT_TYPE_ prefix if present (older protobuf format)
	eventType = strings.TrimPrefix(eventType, "EVENT_TYPE_")

	// If it contains underscores, convert from SCREAMING_SNAKE_CASE to PascalCase
	if strings.Contains(eventType, "_") {
		parts := strings.Split(strings.ToLower(eventType), "_")
		for i, part := range parts {
			if len(part) > 0 {
				parts[i] = strings.ToUpper(part[:1]) + part[1:]
			}
		}
		return strings.Join(parts, "")
	}

	// Otherwise it's already in a readable format (e.g., WorkflowExecutionStarted)
	return eventType
}

// formatPayloads formats payloads for display
func formatPayloads(payloads *commonpb.Payloads) string {
	if payloads == nil {
		return ""
	}

	var results []string
	for _, p := range payloads.GetPayloads() {
		if p == nil {
			continue
		}
		data := p.GetData()
		if len(data) == 0 {
			continue
		}

		// Try to parse as JSON for nicer display
		var jsonVal interface{}
		if err := json.Unmarshal(data, &jsonVal); err == nil {
			// Format as compact JSON
			if b, err := json.Marshal(jsonVal); err == nil {
				results = append(results, string(b))
				continue
			}
		}

		// Fall back to raw string (truncated)
		s := string(data)
		if len(s) > 100 {
			s = s[:100] + "..."
		}
		results = append(results, s)
	}

	return strings.Join(results, ", ")
}
