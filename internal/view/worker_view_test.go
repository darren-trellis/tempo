package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestWorkerViewMockTree(t *testing.T) {
	wv := NewWorkerView(&App{})
	wv.loadMockData()
	if len(wv.groups) != 2 {
		t.Fatalf("hosts: %d", len(wv.groups))
	}
	if len(wv.rows) != 5 || !wv.rows[0].IsHost || wv.rows[1].IsHost {
		t.Fatalf("tree: %+v", wv.rows)
	}
	if !strings.Contains(formatWorkerHostPreview(wv.groups[0]), "host-001") {
		t.Fatal("host preview should include the hostname")
	}
}

func TestGroupWorkersByHost(t *testing.T) {
	workers := []temporal.Worker{
		{Host: "host-b", Identity: "b1", TaskQueue: "q2"},
		{Host: "host-a", Identity: "a2", TaskQueue: "q1"},
		{Host: "host-a", Identity: "a1", TaskQueue: "q1"},
		{Identity: "solo@host-c", TaskQueue: "q3"},
	}
	groups := groupWorkersByHost(workers)
	if len(groups) != 3 {
		t.Fatalf("groups: %d", len(groups))
	}
	if groups[0].Host != "host-a" || len(groups[0].Workers) != 2 {
		t.Fatalf("host-a: %+v", groups[0])
	}
	if groups[0].Workers[0].Identity != "a1" || groups[0].Workers[1].Identity != "a2" {
		t.Fatalf("host-a order: %+v", groups[0].Workers)
	}
	if groups[1].Host != "host-b" || groups[2].Host != "host-c" {
		t.Fatalf("hosts: %q %q", groups[1].Host, groups[2].Host)
	}
}

func TestFlattenWorkerRowsCollapsed(t *testing.T) {
	groups := groupWorkersByHost([]temporal.Worker{
		{Host: "host-a", Identity: "a1", TaskQueue: "q1"},
		{Host: "host-a", Identity: "a2", TaskQueue: "q1"},
		{Host: "host-b", Identity: "b1", TaskQueue: "q2"},
	})
	rows := flattenWorkerRows(groups, nil)
	if len(rows) != 5 {
		t.Fatalf("expanded: %d", len(rows))
	}
	if !rows[0].IsHost || rows[1].IsHost || rows[1].Worker.Identity != "a1" {
		t.Fatalf("expanded rows: %+v", rows)
	}

	rows = flattenWorkerRows(groups, map[string]bool{"host-a": true})
	if len(rows) != 3 {
		t.Fatalf("collapsed: %d", len(rows))
	}
	if !rows[0].IsHost || rows[0].Host != "host-a" || !rows[1].IsHost {
		t.Fatalf("collapsed rows: %+v", rows)
	}
}

func TestWorkerMatches(t *testing.T) {
	w := temporal.Worker{
		Host: "host-a", Identity: "worker-1", TaskQueue: "orders",
		Status: temporal.WorkerStatusRunning, BuildID: "build-9", ProcessID: "4122",
	}
	if !workerMatches(w, "host-a") || !workerMatches(w, "build-9") || !workerMatches(w, "4122") {
		t.Fatal("expected match")
	}
	if workerMatches(w, "payments") {
		t.Fatal("unexpected match")
	}
}

func TestFormatWorkerInstancePreview(t *testing.T) {
	text := formatWorkerInstancePreview(temporal.Worker{
		InstanceKey: "inst-1", Identity: "worker-1", Host: "host-a", ProcessID: "4122",
		TaskQueue: "orders", Status: temporal.WorkerStatusRunning,
		BuildID: "build-9", StartTime: time.Now().Add(-time.Hour), LastHeartbeat: time.Now(),
		HasHostInfo: true, CPU: 0.2, Memory: 0.5,
		WorkflowSlots: temporal.WorkerSlots{Used: 1, Available: 10, Processed: 4},
	})
	for _, want := range []string{"Instance", "worker-1", "host-a", "4122", "orders", "build-9", "Running", "20%", "50%"} {
		if !strings.Contains(text, want) {
			t.Fatalf("preview missing %q:\n%s", want, text)
		}
	}
}

func TestFormatStickyCache(t *testing.T) {
	if got := formatStickyCache(temporal.Worker{}); got != "-" {
		t.Fatalf("empty: %q", got)
	}
	if got := formatStickyCache(temporal.Worker{StickyCacheHit: 80, StickyCacheMiss: 20, StickyCacheSize: 12}); got != "size 12  hit 80%" {
		t.Fatalf("cache: %q", got)
	}
}
