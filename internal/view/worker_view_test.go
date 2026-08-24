package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestWorkerViewListsInstancesFlat(t *testing.T) {
	wv := NewWorkerView(&App{})
	wv.loadMockData()
	if len(wv.rows) != len(wv.allWorkers) {
		t.Fatalf("every worker is one row: %d rows for %d workers", len(wv.rows), len(wv.allWorkers))
	}
	if wv.table.RowCount() != len(wv.allWorkers) {
		t.Fatalf("table rows: %d", wv.table.RowCount())
	}
	// The host moved from a grouping row into its own column.
	headers := workerTableHeaders()
	if headers[0] != "INSTANCE" || headers[1] != "HOST" {
		t.Fatalf("headers: %v", headers)
	}
	if got := wv.table.GetCell(1, 1).Text; got != "host-001" {
		t.Fatalf("host column: %q", got)
	}
}

func TestSortWorkerInstances(t *testing.T) {
	sorted := sortWorkerInstances([]temporal.Worker{
		{Host: "host-b", Identity: "b1", TaskQueue: "q2"},
		{Host: "host-a", Identity: "a2", TaskQueue: "q1"},
		{Host: "host-a", Identity: "a1", TaskQueue: "q9"},
		{Host: "host-a", Identity: "a1", TaskQueue: "q1"},
		{Identity: "solo@host-c", TaskQueue: "q3"},
	})
	var got []string
	for _, w := range sorted {
		got = append(got, workerHost(w)+"/"+w.Identity+"/"+w.TaskQueue)
	}
	want := []string{
		"host-a/a1/q1",
		"host-a/a1/q9",
		"host-a/a2/q1",
		"host-b/b1/q2",
		"host-c/solo@host-c/q3",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order:\n got %v\nwant %v", got, want)
		}
	}
}

// infoRowValue returns the value of a detail row by key.
func infoRowValue(rows []workflowInfoRow, key string) string {
	if idx := workflowInfoRowIndex(rows, key); idx >= 0 {
		return rows[idx].displayText()
	}
	return ""
}

func TestWorkerInfoRows(t *testing.T) {
	rows := workerInfoRows(time.Now(), temporal.Worker{
		InstanceKey: "inst-1", Identity: "worker-1", Host: "host-a", ProcessID: "4122",
		TaskQueue: "orders", Status: temporal.WorkerStatusRunning,
		BuildID: "build-9", StartTime: time.Now().Add(-time.Hour), LastHeartbeat: time.Now(),
		HasHostInfo: true, CPU: 0.2, Memory: 0.5,
		WorkflowSlots: temporal.WorkerSlots{Used: 1, Available: 10, Processed: 4},
	})
	want := map[string]string{
		"instance":       "inst-1",
		"identity":       "worker-1",
		"host":           "host-a",
		"pid":            "4122",
		"taskqueue":      "orders",
		"buildid":        "build-9",
		"status":         "Running",
		"cpu":            "20%",
		"memory":         "50%",
		"slots-workflow": "1 / 10  processed 4  failed 0",
		"slots-activity": "-",
	}
	for key, value := range want {
		if got := infoRowValue(rows, key); got != value {
			t.Fatalf("row %q: got %q want %q", key, got, value)
		}
	}
	if got := infoRowValue(rows, "cpu"); strings.ContainsAny(got, "\u2588\u2591") {
		t.Fatalf("the columns and detail table should stay numeric: %q", got)
	}
}

func TestWorkerPollersValue(t *testing.T) {
	if got := workerPollersValue(temporal.WorkerPollers{}); got != "-" {
		t.Fatalf("empty: %q", got)
	}
	if got := workerPollersValue(temporal.WorkerPollers{Current: 2, Autoscaling: true}); got != "2  autoscaling" {
		t.Fatalf("autoscaling: %q", got)
	}
	if got := workerPollersValue(temporal.WorkerPollers{Current: 3}); got != "3  manual" {
		t.Fatalf("manual: %q", got)
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
