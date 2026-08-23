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
