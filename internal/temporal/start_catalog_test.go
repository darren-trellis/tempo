package temporal

import (
	"reflect"
	"testing"
)

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
