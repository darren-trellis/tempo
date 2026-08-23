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
