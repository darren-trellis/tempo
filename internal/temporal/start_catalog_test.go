package temporal

import (
	"errors"
	"reflect"
	"testing"
)

func TestStartCatalogNeedsWorkflowScan(t *testing.T) {
	if startCatalogNeedsWorkflowScan(nil, nil) {
		t.Fatal("successful group-by should not page every workflow")
	}
	if !startCatalogNeedsWorkflowScan(errors.New("no group by"), nil) {
		t.Fatal("a failed type grouping should fall back to listing workflows")
	}
	if !startCatalogNeedsWorkflowScan(nil, errors.New("no group by")) {
		t.Fatal("a failed queue grouping should fall back to listing workflows")
	}
}

func TestCollectStartCatalogWithoutWorkflowPages(t *testing.T) {
	types, queues := collectStartCatalog(
		[]string{"OrderWorkflow"},
		[]string{"orders"},
		nil,
		[]Worker{{TaskQueue: "worker-queue"}},
		[]Schedule{{WorkflowType: "Cron", TaskQueue: "cron-queue"}},
	)
	if !reflect.DeepEqual(types, []string{"Cron", "OrderWorkflow"}) {
		t.Fatalf("types=%v", types)
	}
	if !reflect.DeepEqual(queues, []string{"cron-queue", "orders", "worker-queue"}) {
		t.Fatalf("queues=%v", queues)
	}
}

func TestCollectStartCatalogMergesAllSources(t *testing.T) {
	types, queues := collectStartCatalog(
		[]string{"GroupType"},
		[]string{"group-queue"},
		[]Workflow{{Type: "PageType", TaskQueue: "page-queue"}},
		[]Worker{{TaskQueue: "worker-queue"}},
		[]Schedule{{WorkflowType: "SchedType", TaskQueue: "sched-queue"}},
	)
	wantTypes := []string{"GroupType", "PageType", "SchedType"}
	wantQueues := []string{"group-queue", "page-queue", "sched-queue", "worker-queue"}
	if !reflect.DeepEqual(types, wantTypes) {
		t.Fatalf("types=%v want %v", types, wantTypes)
	}
	if !reflect.DeepEqual(queues, wantQueues) {
		t.Fatalf("queues=%v want %v", queues, wantQueues)
	}
}
