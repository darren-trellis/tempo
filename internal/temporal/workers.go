package temporal

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/galaxy-io/tempo/internal/config"
	workerpb "go.temporal.io/api/worker/v1"
	"go.temporal.io/api/workflowservice/v1"
)

func WorkerFromHeartbeat(hb *workerpb.WorkerHeartbeat) (Worker, bool) {
	if hb == nil {
		return Worker{}, false
	}
	identity := hb.GetWorkerIdentity()
	if identity == "" {
		identity = hb.GetWorkerInstanceKey()
	}
	var startTime, lastHeartbeat time.Time
	if ts := hb.GetStartTime(); ts != nil {
		startTime = ts.AsTime()
	}
	if ts := hb.GetHeartbeatTime(); ts != nil {
		lastHeartbeat = ts.AsTime()
	} else {
		lastHeartbeat = startTime
	}
	hostInfo := hb.GetHostInfo()
	host := ""
	processID := ""
	var cpu, mem float32
	hasHostInfo := hostInfo != nil
	if hasHostInfo {
		host = hostInfo.GetHostName()
		processID = hostInfo.GetProcessId()
		cpu = hostInfo.GetCurrentHostCpuUsage()
		mem = hostInfo.GetCurrentHostMemUsage()
	}
	if host == "" {
		host = HostFromIdentity(identity)
	}
	var buildID, deployment string
	if ver := hb.GetDeploymentVersion(); ver != nil {
		buildID = ver.GetBuildId()
		deployment = ver.GetDeploymentName()
	}
	return Worker{
		InstanceKey:     hb.GetWorkerInstanceKey(),
		Identity:        identity,
		Host:            host,
		ProcessID:       processID,
		TaskQueue:       hb.GetTaskQueue(),
		Types:           workerTypesFromHeartbeat(hb),
		Status:          MapWorkerStatus(hb.GetStatus()),
		StartTime:       startTime,
		LastHeartbeat:   lastHeartbeat,
		BuildID:         buildID,
		Deployment:      deployment,
		SDKName:         hb.GetSdkName(),
		SDKVersion:      hb.GetSdkVersion(),
		HasHostInfo:     hasHostInfo,
		CPU:             cpu,
		Memory:          mem,
		WorkflowSlots:   slotsFromProto(hb.GetWorkflowTaskSlotsInfo()),
		ActivitySlots:   slotsFromProto(hb.GetActivityTaskSlotsInfo()),
		LocalSlots:      slotsFromProto(hb.GetLocalActivitySlotsInfo()),
		NexusSlots:      slotsFromProto(hb.GetNexusTaskSlotsInfo()),
		WorkflowPollers: pollersFromProto(hb.GetWorkflowPollerInfo()),
		StickyPollers:   pollersFromProto(hb.GetWorkflowStickyPollerInfo()),
		ActivityPollers: pollersFromProto(hb.GetActivityPollerInfo()),
		NexusPollers:    pollersFromProto(hb.GetNexusPollerInfo()),
		StickyCacheHit:  hb.GetTotalStickyCacheHit(),
		StickyCacheMiss: hb.GetTotalStickyCacheMiss(),
		StickyCacheSize: hb.GetCurrentStickyCacheSize(),
	}, true
}

// The server keeps describing a worker for minutes after it dies: a poll registry
// entry lingers until the matching engine evicts it, and a heartbeat record
// outlives the process that sent it. So liveness is judged by how recently the
// instance was actually seen, never by whether it is still listed.
//
// WorkerQuietWindows is how long each source may go unchanged first. A zero field
// falls back to the configured default.
type WorkerQuietWindows struct {
	Poll      time.Duration
	Heartbeat time.Duration
}

func (w WorkerQuietWindows) poll() time.Duration {
	if w.Poll > 0 {
		return w.Poll
	}
	return config.DefaultWorkerPollQuietAfter
}

func (w WorkerQuietWindows) heartbeat() time.Duration {
	if w.Heartbeat > 0 {
		return w.Heartbeat
	}
	return config.DefaultWorkerHeartbeatQuietAfter
}

// WorkersFromPollers derives one instance per identity polling a task queue.
// These carry no heartbeat detail, so they are marked as polling only.
func WorkersFromPollers(queue string, pollers []Poller) []Worker {
	type key struct {
		identity string
		queue    string
	}
	byKey := map[key]*Worker{}
	order := make([]key, 0, len(pollers))
	for _, p := range pollers {
		if p.Identity == "" {
			continue
		}
		k := key{identity: p.Identity, queue: queue}
		entry := byKey[k]
		if entry == nil {
			w := Worker{
				Identity:      p.Identity,
				Host:          HostFromIdentity(p.Identity),
				TaskQueue:     queue,
				Status:        WorkerStatusPolling,
				LastHeartbeat: p.LastAccessTime,
			}
			byKey[k] = &w
			order = append(order, k)
			entry = &w
		}
		entry.Types = appendUniqueStrings(entry.Types, p.TaskQueueType)
		if p.LastAccessTime.After(entry.LastHeartbeat) {
			entry.LastHeartbeat = p.LastAccessTime
		}
	}
	out := make([]Worker, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// WorkerKey identifies one instance: an identity polling one task queue.
func WorkerKey(w Worker) string {
	identity := w.Identity
	if identity == "" {
		identity = w.InstanceKey
	}
	return identity + "|" + w.TaskQueue
}

// MergeWorkerSources folds instances seen only in task queue poll registries into
// the heartbeat list, so the worker view is a superset of what is polling, and
// marks the instances that have gone quiet.
func MergeWorkerSources(heartbeats, polled []Worker, now time.Time, windows WorkerQuietWindows) []Worker {
	lastPoll := make(map[string]time.Time, len(polled))
	for _, w := range polled {
		key := WorkerKey(w)
		if last, ok := lastPoll[key]; !ok || w.LastHeartbeat.After(last) {
			lastPoll[key] = w.LastHeartbeat
		}
	}

	out := make([]Worker, 0, len(heartbeats)+len(polled))
	seen := make(map[string]struct{}, len(heartbeats))
	for _, w := range heartbeats {
		key := WorkerKey(w)
		seen[key] = struct{}{}
		if workerGoneQuiet(w, lastPoll[key], now, windows) {
			w.Status = WorkerStatusStale
		}
		out = append(out, w)
	}
	for _, w := range polled {
		if _, ok := seen[WorkerKey(w)]; ok {
			continue
		}
		// A derived row's heartbeat time is its last poll.
		if workerGoneQuiet(w, w.LastHeartbeat, now, windows) {
			w.Status = WorkerStatusStale
		}
		out = append(out, w)
	}
	return out
}

// workerGoneQuiet reports whether an instance has stopped being seen. Polling is
// the stronger signal, since a live worker refreshes its registry entry every
// long poll; only when there is no entry does the heartbeat clock decide.
func workerGoneQuiet(w Worker, lastPoll, now time.Time, windows WorkerQuietWindows) bool {
	switch w.Status {
	case WorkerStatusShutdown, WorkerStatusShuttingDown:
		return false
	}
	if now.IsZero() {
		return false
	}
	if !lastPoll.IsZero() {
		return now.Sub(lastPoll) > windows.poll()
	}
	if w.LastHeartbeat.IsZero() {
		return false
	}
	return now.Sub(w.LastHeartbeat) > windows.heartbeat()
}

func HostFromIdentity(identity string) string {
	if identity == "" {
		return "Unknown"
	}
	i := strings.LastIndex(identity, "@")
	if i <= 0 {
		return identity
	}
	left, right := identity[:i], identity[i+1:]
	if isAllDigits(right) {
		return left
	}
	if right != "" {
		return right
	}
	return identity
}

func slotsFromProto(info *workerpb.WorkerSlotsInfo) WorkerSlots {
	if info == nil {
		return WorkerSlots{}
	}
	return WorkerSlots{
		Used:      info.GetCurrentUsedSlots(),
		Available: info.GetCurrentAvailableSlots(),
		Kind:      info.GetSlotSupplierKind(),
		Processed: info.GetTotalProcessedTasks(),
		Failed:    info.GetTotalFailedTasks(),
	}
}

func pollersFromProto(info *workerpb.WorkerPollerInfo) WorkerPollers {
	if info == nil {
		return WorkerPollers{}
	}
	return WorkerPollers{
		Current:     info.GetCurrentPollers(),
		Autoscaling: info.GetIsAutoscaling(),
	}
}

func appendUniqueStrings(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func (c *Client) ListWorkers(ctx context.Context, namespace string) ([]Worker, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	var workers []Worker
	var token []byte
	for {
		resp, err := cl.WorkflowService().ListWorkers(ctx, &workflowservice.ListWorkersRequest{
			Namespace:     namespace,
			PageSize:      100,
			NextPageToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list workers: %w", err)
		}
		for _, info := range resp.GetWorkersInfo() {
			if w, ok := WorkerFromHeartbeat(info.GetWorkerHeartbeat()); ok {
				workers = append(workers, w)
			}
		}
		token = resp.GetNextPageToken()
		if len(token) == 0 {
			break
		}
	}
	return workers, nil
}
