package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestHierarchyLayoutDoesNotHangOnIDCycles(t *testing.T) {
	current := &temporal.Workflow{ID: "order-1", RunID: "run-a", Type: "Order", Status: "Running"}
	wg := NewWorkflowGraphView(&App{}, "default", current)
	wg.relationships = &temporal.WorkflowRelationships{
		Current: current,
		Parent:  &temporal.Workflow{ID: "order-1", RunID: "run-parent", Type: "Order", Status: "Completed"},
		Children: []*temporal.WorkflowNode{
			{
				Workflow: temporal.Workflow{ID: "order-1", RunID: "run-b", Type: "Order", Status: "Running"},
				EdgeType: "continue",
				Children: []*temporal.WorkflowNode{
					{Workflow: *current, EdgeType: "child"},
				},
			},
			{Workflow: *current, EdgeType: "child"},
		},
	}

	done := make(chan struct{})
	go func() {
		wg.buildTreeData()
		wg.buildGraphData()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("hierarchy layout hung on continue-as-new / self-child IDs")
	}
	if wg.treeData == nil || wg.treeData.GetNode(graphExecID(current.ID, current.RunID)) == nil {
		t.Fatal("current workflow should still be in the tree")
	}
	if wg.graphData == nil || wg.graphData.GetNode(graphExecID(current.ID, current.RunID)) == nil {
		t.Fatal("current workflow should still be in the graph")
	}
}
