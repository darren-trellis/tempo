package temporal

import (
	"strings"
	"time"
	"unicode"

	workerpb "go.temporal.io/api/worker/v1"
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

// WorkerStaleAfter is how long a heartbeat can go quiet, with nothing polling on
// its behalf, before the instance is treated as gone.
const WorkerStaleAfter = 2 * time.Minute

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
// flags heartbeats that newer poll activity has outrun.
//
// A heartbeat is stale when nothing is polling under its identity on its task
// queue and either that queue has seen newer poll activity from someone else, or
// the heartbeat itself has gone quiet for longer than WorkerStaleAfter.
func MergeWorkerSources(heartbeats, polled []Worker, now time.Time) []Worker {
	polling := make(map[string]struct{}, len(polled))
	queueSeen := make(map[string]time.Time, len(polled))
	for _, w := range polled {
		polling[WorkerKey(w)] = struct{}{}
		if last, ok := queueSeen[w.TaskQueue]; !ok || w.LastHeartbeat.After(last) {
			queueSeen[w.TaskQueue] = w.LastHeartbeat
		}
	}

	out := make([]Worker, 0, len(heartbeats)+len(polled))
	seen := make(map[string]struct{}, len(heartbeats))
	for _, w := range heartbeats {
		key := WorkerKey(w)
		seen[key] = struct{}{}
		if _, live := polling[key]; !live && workerLooksGone(w, queueSeen[w.TaskQueue], now) {
			w.Status = WorkerStatusStale
		}
		out = append(out, w)
	}
	for _, w := range polled {
		if _, ok := seen[WorkerKey(w)]; ok {
			continue
		}
		out = append(out, w)
	}
	return out
}

// workerLooksGone reports whether a heartbeat has been outrun by poll activity on
// its task queue, or has simply gone quiet.
func workerLooksGone(w Worker, queueLastPoll, now time.Time) bool {
	switch w.Status {
	case WorkerStatusShutdown, WorkerStatusShuttingDown:
		return false
	}
	if w.LastHeartbeat.IsZero() {
		return false
	}
	if !queueLastPoll.IsZero() && queueLastPoll.After(w.LastHeartbeat) {
		return true
	}
	return !now.IsZero() && now.Sub(w.LastHeartbeat) > WorkerStaleAfter
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
