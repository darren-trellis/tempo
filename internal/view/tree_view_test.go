package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/rivo/tview"
)

func TestEventTreeFormatOmitsDuration(t *testing.T) {
	etv := NewEventTreeView()
	end := time.Now()
	got := etv.formatNodeText(&temporal.EventTreeNode{
		Name:     "Activity: ChargeCard",
		Status:   "Completed",
		Duration: 1500 * time.Millisecond,
		EndTime:  &end,
		Attempts: 1,
	})
	if strings.Contains(got, "1.5s") || strings.Contains(got, "1500ms") {
		t.Fatalf("duration should be its own column, got %q", got)
	}
	if !strings.Contains(got, "ChargeCard") || !strings.Contains(got, "[Completed]") {
		t.Fatalf("label should keep name and status, got %q", got)
	}
}

func TestEventTreeFormatKeepsAttempts(t *testing.T) {
	etv := NewEventTreeView()
	got := etv.formatNodeText(&temporal.EventTreeNode{
		Name:     "Activity: ChargeCard",
		Status:   "Failed",
		Attempts: 3,
	})
	if !strings.Contains(got, "3 attempts") {
		t.Fatalf("got %q", got)
	}
}

func TestEventTreeDurationColumnIsRightAligned(t *testing.T) {
	if got := rightAlignIn("1.5s", 6); got != "  1.5s" {
		t.Fatalf("right align: %q", got)
	}
	end := time.Now()
	nodes := []*temporal.EventTreeNode{
		{Name: "Short", Status: "Completed", Duration: 20 * time.Millisecond, EndTime: &end},
		{Name: "Long", Status: "Completed", Duration: 1500 * time.Millisecond, EndTime: &end},
	}
	if w := eventTreeDurationWidth(nodes); w != len("1.5s") {
		t.Fatalf("column width=%d", w)
	}
	if got := eventTreeDurationText(nodes[0]); got != "20ms" {
		t.Fatalf("short duration: %q", got)
	}
}

func TestFlattenVisibleTreeNodesFollowsExpansion(t *testing.T) {
	root := tview.NewTreeNode("Events").SetExpanded(true)
	child := tview.NewTreeNode("Child").SetExpanded(false)
	child.AddChild(tview.NewTreeNode("Hidden"))
	root.AddChild(child)
	got := flattenVisibleTreeNodes(root)
	if len(got) != 2 || got[0] != root || got[1] != child {
		t.Fatalf("visible=%d", len(got))
	}
}
