package temporal

import (
	"reflect"
	"testing"
	"time"

	v1 "go.temporal.io/api/deployment/v1"
	"go.temporal.io/api/enums/v1"
	workerpb "go.temporal.io/api/worker/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestHostFromIdentity(t *testing.T) {
	cases := map[string]string{
		"":                 "Unknown",
		"worker-host":      "worker-host",
		"worker-host@4122": "worker-host",
		"deploy@worker-1":  "worker-1",
	}
	for in, want := range cases {
		if got := HostFromIdentity(in); got != want {
			t.Fatalf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestWorkerFromHeartbeat(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	beat := start.Add(2 * time.Minute)
	w, ok := WorkerFromHeartbeat(&workerpb.WorkerHeartbeat{
		WorkerInstanceKey: "inst-1",
		WorkerIdentity:    "orders@host-a",
		HostInfo: &workerpb.WorkerHostInfo{
			HostName:            "host-a",
			ProcessId:           "4122",
			CurrentHostCpuUsage: 0.12,
			CurrentHostMemUsage: 0.34,
		},
		TaskQueue: "orders",
		DeploymentVersion: &v1.WorkerDeploymentVersion{
			BuildId:        "build-9",
			DeploymentName: "checkout",
		},
		SdkName:       "temporal-go",
		SdkVersion:    "1.38.0",
		Status:        enums.WORKER_STATUS_RUNNING,
		StartTime:     timestamppb.New(start),
		HeartbeatTime: timestamppb.New(beat),
		WorkflowTaskSlotsInfo: &workerpb.WorkerSlotsInfo{
			CurrentUsedSlots:      2,
			CurrentAvailableSlots: 100,
		},
		ActivityPollerInfo: &workerpb.WorkerPollerInfo{CurrentPollers: 3, IsAutoscaling: true},
	})
	if !ok {
		t.Fatal("expected worker")
	}
	if w.InstanceKey != "inst-1" || w.Host != "host-a" || w.ProcessID != "4122" {
		t.Fatalf("identity: %+v", w)
	}
	if w.Status != WorkerStatusRunning || w.BuildID != "build-9" || w.Deployment != "checkout" {
		t.Fatalf("meta: %+v", w)
	}
	if w.CPU != 0.12 || w.Memory != 0.34 || !w.HasHostInfo {
		t.Fatalf("resources: %+v", w)
	}
	if w.WorkflowSlots.Used != 2 || w.ActivityPollers.Current != 3 || !w.ActivityPollers.Autoscaling {
		t.Fatalf("slots/pollers: %+v", w)
	}
	if !reflect.DeepEqual(w.Types, []string{TaskQueueTypeActivity}) {
		t.Fatalf("types: %v", w.Types)
	}
}

func TestWorkersFromPollers(t *testing.T) {
	now := time.Now()
	got := WorkersFromPollers("orders", []Poller{
		{Identity: "w1@host-a", TaskQueueType: TaskQueueTypeWorkflow, LastAccessTime: now.Add(-time.Second)},
		{Identity: "w1@host-a", TaskQueueType: TaskQueueTypeActivity, LastAccessTime: now},
		{Identity: "w2@host-b", TaskQueueType: TaskQueueTypeActivity, LastAccessTime: now},
	})
	if len(got) != 2 {
		t.Fatalf("workers: %d", len(got))
	}
	if got[0].Host != "host-a" || got[0].TaskQueue != "orders" {
		t.Fatalf("first: %+v", got[0])
	}
	if !reflect.DeepEqual(got[0].Types, []string{TaskQueueTypeWorkflow, TaskQueueTypeActivity}) {
		t.Fatalf("types: %v", got[0].Types)
	}
	if !got[0].LastHeartbeat.Equal(now) {
		t.Fatalf("heartbeat: %v", got[0].LastHeartbeat)
	}
}

func TestWorkersFromPollersMarkPolling(t *testing.T) {
	workers := WorkersFromPollers("orders", []Poller{
		{Identity: "51067@laptop", TaskQueueType: "Workflow"},
		{Identity: "51067@laptop", TaskQueueType: "Activity"},
	})
	if len(workers) != 1 {
		t.Fatalf("one identity polling one queue is one instance, got %d", len(workers))
	}
	if workers[0].Status != WorkerStatusPolling {
		t.Fatalf("status: %q", workers[0].Status)
	}
	if len(workers[0].Types) != 2 {
		t.Fatalf("both poller types should fold into the instance: %v", workers[0].Types)
	}
}

func TestMergeWorkerSourcesUnionsBothViews(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	heartbeats := []Worker{
		{Identity: "49027@laptop", TaskQueue: "orders", Status: WorkerStatusRunning, LastHeartbeat: now.Add(-10 * time.Second)},
		{Identity: "49027@laptop", TaskQueue: "payments", Status: WorkerStatusRunning, LastHeartbeat: now.Add(-10 * time.Second)},
	}
	polled := []Worker{
		{Identity: "49027@laptop", TaskQueue: "orders", Status: WorkerStatusPolling, LastHeartbeat: now.Add(-2 * time.Second)},
		{Identity: "51067@laptop", TaskQueue: "orders", Status: WorkerStatusPolling, LastHeartbeat: now.Add(-1 * time.Second)},
	}

	merged := MergeWorkerSources(heartbeats, polled, now)
	if len(merged) != 3 {
		t.Fatalf("expected the two heartbeats plus the poller-only instance, got %d: %+v", len(merged), merged)
	}
	byKey := map[string]Worker{}
	for _, w := range merged {
		byKey[WorkerKey(w)] = w
	}
	if got := byKey["51067@laptop|orders"]; got.Status != WorkerStatusPolling {
		t.Fatalf("a poller with no heartbeat should show as polling: %+v", got)
	}
	if got := byKey["49027@laptop|orders"]; got.Status != WorkerStatusRunning {
		t.Fatalf("a heartbeat that is still polling stays running: %+v", got)
	}
	// The duplicate is the heartbeat row, not the derived one: it keeps its detail.
	if got := byKey["49027@laptop|orders"]; got.LastHeartbeat != now.Add(-10*time.Second) {
		t.Fatalf("heartbeat detail should win over the derived row: %+v", got)
	}
}

func TestMergeWorkerSourcesFlagsStoppedWorkers(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	// A worker killed a few minutes ago: the server still lists both its heartbeat
	// and its poll registry entry, but neither has moved since.
	heartbeats := []Worker{
		{Identity: "49027@laptop", TaskQueue: "orders", Status: WorkerStatusRunning, LastHeartbeat: now.Add(-4 * time.Minute)},
	}
	polled := []Worker{
		{Identity: "49027@laptop", TaskQueue: "orders", Status: WorkerStatusPolling, LastHeartbeat: now.Add(-4 * time.Minute)},
		{Identity: "51067@laptop", TaskQueue: "orders", Status: WorkerStatusPolling, LastHeartbeat: now.Add(-3 * time.Minute)},
	}

	merged := MergeWorkerSources(heartbeats, polled, now)
	if merged[0].Status != WorkerStatusStale {
		t.Fatalf("a lingering registry entry must not vouch for a dead heartbeat: %+v", merged[0])
	}
	if merged[1].Status != WorkerStatusStale {
		t.Fatalf("a poll registry entry that stopped moving is stale too: %+v", merged[1])
	}

	// Still polling, so still running, however old the heartbeat is.
	live := MergeWorkerSources(
		[]Worker{{Identity: "49027@laptop", TaskQueue: "orders", Status: WorkerStatusRunning, LastHeartbeat: now.Add(-70 * time.Second)}},
		[]Worker{{Identity: "49027@laptop", TaskQueue: "orders", Status: WorkerStatusPolling, LastHeartbeat: now.Add(-20 * time.Second)}},
		now,
	)
	if live[0].Status != WorkerStatusRunning {
		t.Fatalf("a worker whose polls are fresh is running: %+v", live[0])
	}

	// A poll one long-poll cycle old is normal, not death.
	polling := MergeWorkerSources(nil,
		[]Worker{{Identity: "51067@laptop", TaskQueue: "orders", Status: WorkerStatusPolling, LastHeartbeat: now.Add(-time.Minute)}},
		now,
	)
	if polling[0].Status != WorkerStatusPolling {
		t.Fatalf("a poll within the long-poll cycle is still polling: %+v", polling[0])
	}
}

func TestMergeWorkerSourcesStaleWhenQuiet(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	quiet := []Worker{
		{Identity: "a@laptop", TaskQueue: "idle", Status: WorkerStatusRunning, LastHeartbeat: now.Add(-WorkerHeartbeatQuietAfter - time.Second)},
	}
	if got := MergeWorkerSources(quiet, nil, now); got[0].Status != WorkerStatusStale {
		t.Fatalf("a heartbeat quiet for longer than the window is stale: %+v", got[0])
	}

	fresh := []Worker{
		{Identity: "a@laptop", TaskQueue: "idle", Status: WorkerStatusRunning, LastHeartbeat: now.Add(-time.Second)},
	}
	if got := MergeWorkerSources(fresh, nil, now); got[0].Status != WorkerStatusRunning {
		t.Fatalf("a recent heartbeat on an idle queue stays running: %+v", got[0])
	}

	// A worker that told us it is going away is reported as it asked.
	for _, status := range []string{WorkerStatusShuttingDown, WorkerStatusShutdown} {
		leaving := []Worker{
			{Identity: "a@laptop", TaskQueue: "idle", Status: status, LastHeartbeat: now.Add(-time.Hour)},
		}
		if got := MergeWorkerSources(leaving, nil, now); got[0].Status != status {
			t.Fatalf("%s should not be relabelled: %+v", status, got[0])
		}
	}

	// No heartbeat time at all is not evidence of anything.
	unknown := []Worker{{Identity: "a@laptop", TaskQueue: "idle", Status: WorkerStatusRunning}}
	if got := MergeWorkerSources(unknown, nil, now); got[0].Status != WorkerStatusRunning {
		t.Fatalf("a heartbeat with no timestamp should not be guessed at: %+v", got[0])
	}
}
