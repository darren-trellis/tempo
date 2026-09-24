package view

import (
	"strings"
	"testing"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func (wl *WorkflowList) workflowDepth(index int) int {
	if index < 0 || index >= len(wl.workflowDepths) {
		return 0
	}
	return wl.workflowDepths[index]
}

func TestNestWorkflowsIndentsChildren(t *testing.T) {
	now := time.Now()
	parent := "root-wf"
	child := "child-wf"
	workflows := []temporal.Workflow{
		{ID: parent, RunID: "r1", StartTime: now},
		{ID: "other", RunID: "r2", StartTime: now.Add(-time.Minute)},
		{ID: child, RunID: "r3", StartTime: now.Add(-30 * time.Second), ParentID: &parent},
		{ID: "grandchild", RunID: "r4", StartTime: now.Add(-10 * time.Second), ParentID: &child},
	}

	got, depths, prefixes, _ := nestWorkflows(workflows, nil)
	if len(got) != 4 || len(depths) != 4 || len(prefixes) != 4 {
		t.Fatalf("len workflows=%d depths=%d prefixes=%d", len(got), len(depths), len(prefixes))
	}
	if got[0].ID != parent || depths[0] != 0 {
		t.Fatalf("root should stay first, got %s depth %d", got[0].ID, depths[0])
	}
	if got[1].ID != child || depths[1] != 1 || prefixes[1] != "└── " {
		t.Fatalf("child should sit under parent, got %s depth %d prefix %q", got[1].ID, depths[1], prefixes[1])
	}
	if got[2].ID != "grandchild" || depths[2] != 2 || prefixes[2] != "    └── " {
		t.Fatalf("grandchild should nest, got %s depth %d prefix %q", got[2].ID, depths[2], prefixes[2])
	}
	if got[3].ID != "other" || depths[3] != 0 {
		t.Fatalf("unrelated root should follow, got %s depth %d", got[3].ID, depths[3])
	}
}

func TestNestWorkflowsOrdersChildrenByStartTime(t *testing.T) {
	parent := "root-wf"
	now := time.Now()
	workflows := []temporal.Workflow{
		{ID: parent, StartTime: now.Add(-time.Hour)},
		{ID: "second", StartTime: now.Add(-10 * time.Minute), ParentID: &parent},
		{ID: "first", StartTime: now.Add(-40 * time.Minute), ParentID: &parent},
		{ID: "third", StartTime: now.Add(-time.Minute), ParentID: &parent},
	}

	got, depths, prefixes, _ := nestWorkflows(workflows, nil)
	if len(got) != 4 {
		t.Fatalf("len: %d", len(got))
	}
	if prefixes[1] != workflowTreePrefix([]bool{false}) || prefixes[2] != workflowTreePrefix([]bool{false}) || prefixes[3] != workflowTreePrefix([]bool{true}) {
		t.Fatalf("sibling prefixes: %q %q %q", prefixes[1], prefixes[2], prefixes[3])
	}
	want := []string{parent, "first", "second", "third"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("row %d: %q, want %q", i, got[i].ID, id)
		}
	}
	if depths[0] != 0 || depths[1] != 1 || depths[2] != 1 || depths[3] != 1 {
		t.Fatalf("depths: %v", depths)
	}
}

func TestNestWorkflowsMissingParentStaysRoot(t *testing.T) {
	missing := "not-in-list"
	workflows := []temporal.Workflow{
		{ID: "orphan", ParentID: &missing},
		{ID: "solo"},
	}
	got, depths, prefixes, _ := nestWorkflows(workflows, nil)
	if len(got) != 2 || depths[0] != 0 || depths[1] != 0 || prefixes[0] != "" || prefixes[1] != "" {
		t.Fatalf("missing parents should stay roots, depths=%v", depths)
	}
}

func TestWorkflowTreePrefix(t *testing.T) {
	if workflowTreePrefix(nil) != "" {
		t.Fatal("roots should not be indented")
	}
	last := workflowTreePrefix([]bool{true})
	if last != "└── " {
		t.Fatalf("last child: %q", last)
	}
	branch := workflowTreePrefix([]bool{false})
	if branch != "├── " {
		t.Fatalf("sibling: %q", branch)
	}
	nested := workflowTreePrefix([]bool{false, true})
	if nested != "│   └── " {
		t.Fatalf("nested last under sibling: %q", nested)
	}
	nestedLast := workflowTreePrefix([]bool{true, true})
	if nestedLast != "    └── " {
		t.Fatalf("nested last under last: %q", nestedLast)
	}
}

func TestColorizeWorkflowTreePrefix(t *testing.T) {
	prefix := "└── "
	got := colorizeWorkflowTreePrefix(prefix+"child-wf", prefix)
	want := "[" + theme.TagFgDim() + "]" + prefix + "[-]child-wf"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if colorizeWorkflowTreePrefix("root", "") != "root" {
		t.Fatal("roots should stay uncolored")
	}
}

func TestWorkflowIDCellDimsTreePrefix(t *testing.T) {
	col := workflowColumn{id: config.WorkflowColumnWorkflowID, width: 40}
	cell := col.cell(time.Now(), temporal.Workflow{ID: "child-wf"}, "└── ", config.TimeFormatRelative)
	if !strings.HasPrefix(cell.Text, "["+theme.TagFgDim()+"]└── [-]") {
		t.Fatalf("expected dim tree prefix, got %q", cell.Text)
	}
	if !strings.Contains(cell.Text, "child-wf") {
		t.Fatalf("expected workflow id, got %q", cell.Text)
	}
}

func TestToggleWorkflowTreeReordersRows(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	if !wl.workflowTreeMode {
		t.Fatal("tree mode should be the default")
	}
	if desc := hintDescription(wl.Hints(), "b"); desc != "List" {
		t.Fatalf("tree hint: %q", desc)
	}
	flat := append([]temporal.Workflow(nil), wl.workflows...)
	if len(wl.workflows) != len(flat) {
		t.Fatalf("tree should keep all rows, got %d want %d", len(wl.workflows), len(flat))
	}
	foundChild := false
	for i, w := range wl.workflows {
		if w.ID == "payment-xyz789" {
			foundChild = true
			if wl.workflowDepth(i) != 1 || wl.workflowTreePrefixAt(i) != "└── " {
				t.Fatalf("payment should be indented under parent, depth=%d prefix=%q", wl.workflowDepth(i), wl.workflowTreePrefixAt(i))
			}
			if i == 0 || wl.workflows[i-1].ID != "order-processing-abc123" {
				t.Fatal("payment should appear directly under its parent")
			}
		}
		if w.ID == "fulfillment-ghi000" && (wl.workflowDepth(i) != 2 || wl.workflowTreePrefixAt(i) != "    └── ") {
			t.Fatalf("fulfillment should nest under payment, depth=%d prefix=%q", wl.workflowDepth(i), wl.workflowTreePrefixAt(i))
		}
	}
	if !foundChild {
		t.Fatal("expected child workflow in tree")
	}
	if desc := hintDescription(wl.Hints(), "space"); desc != "Fold/Unfold" {
		t.Fatalf("tree fold hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "Ctrl+Space"); desc != "Fold All" {
		t.Fatalf("tree fold-all hint: %q", desc)
	}
}

func TestNestWorkflowsHidesFoldedChildren(t *testing.T) {
	parent := "root-wf"
	child := "child-wf"
	now := time.Now()
	workflows := []temporal.Workflow{
		{ID: parent, RunID: "r1", StartTime: now},
		{ID: child, RunID: "r3", StartTime: now.Add(-30 * time.Second), ParentID: &parent},
		{ID: "grandchild", RunID: "r4", StartTime: now.Add(-10 * time.Second), ParentID: &child},
	}

	got, _, _, hasKids := nestWorkflows(workflows, map[string]bool{workflowIdentityKey(workflows[0]): true})
	if len(got) != 1 || got[0].ID != parent {
		t.Fatalf("folded root should hide descendants, got %v", workflowIDs(got))
	}
	if !hasKids[0] {
		t.Fatal("folded root should still be marked as a parent")
	}

	got, depths, _, hasKids := nestWorkflows(workflows, map[string]bool{workflowIdentityKey(workflows[1]): true})
	if len(got) != 2 || got[0].ID != parent || got[1].ID != child {
		t.Fatalf("folded child should hide grandchild, got %v", workflowIDs(got))
	}
	if depths[1] != 1 || !hasKids[1] || hasKids[0] != true {
		t.Fatalf("child fold: depths=%v hasKids=%v", depths, hasKids)
	}
}

func TestToggleWorkflowTreeFoldHidesChildren(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	if !selectWorkflowByID(wl, "order-processing-abc123") {
		t.Fatal("expected parent workflow")
	}
	if !wl.toggleWorkflowTreeFold() {
		t.Fatal("parent should fold")
	}
	if !visibleWorkflowIDsContain(wl, "order-processing-abc123") {
		t.Fatal("folded parent should stay visible")
	}
	if visibleWorkflowIDsContain(wl, "payment-xyz789") || visibleWorkflowIDsContain(wl, "fulfillment-ghi000") {
		t.Fatalf("folded children should hide, got %v", workflowIDs(wl.workflows))
	}
	if !strings.Contains(wl.workflowRowPrefix(wl.table.SelectedRow()), workflowTreeCollapsed) {
		t.Fatalf("folded parent should show a collapsed mark, got %q", wl.workflowRowPrefix(wl.table.SelectedRow()))
	}
	if desc := hintDescription(wl.Hints(), "Ctrl+Space"); desc != "Unfold All" {
		t.Fatalf("folded hint: %q", desc)
	}

	if !wl.toggleWorkflowTreeFold() {
		t.Fatal("parent should unfold")
	}
	if !visibleWorkflowIDsContain(wl, "payment-xyz789") || !visibleWorkflowIDsContain(wl, "fulfillment-ghi000") {
		t.Fatalf("unfold should restore children, got %v", workflowIDs(wl.workflows))
	}
	if !strings.Contains(wl.workflowRowPrefix(wl.table.SelectedRow()), workflowTreeExpanded) {
		t.Fatalf("unfolded parent should show an empty diamond, got %q", wl.workflowRowPrefix(wl.table.SelectedRow()))
	}
}

func TestToggleWorkflowTreeFoldIgnoresLeaves(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	if !selectWorkflowByID(wl, "shipment-def456") {
		t.Fatal("expected leaf workflow")
	}
	if wl.toggleWorkflowTreeFold() {
		t.Fatal("leaves should not fold")
	}
	if len(wl.workflows) != len(wl.allWorkflows) {
		t.Fatalf("leaf fold should not hide rows, got %d", len(wl.workflows))
	}
}

func TestToggleWorkflowTreeFoldAll(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.loadMockData()
	if !wl.toggleWorkflowTreeFoldAll() {
		t.Fatal("fold all should run in tree mode")
	}
	for _, w := range wl.workflows {
		if w.ID == "payment-xyz789" || w.ID == "fulfillment-ghi000" {
			t.Fatalf("fold all should hide descendants, got %v", workflowIDs(wl.workflows))
		}
	}
	if !wl.anyWorkflowFolded() {
		t.Fatal("fold all should mark parents folded")
	}
	if !wl.toggleWorkflowTreeFoldAll() {
		t.Fatal("unfold all should run")
	}
	if wl.anyWorkflowFolded() {
		t.Fatal("unfold all should clear folds")
	}
	if !visibleWorkflowIDsContain(wl, "payment-xyz789") || !visibleWorkflowIDsContain(wl, "fulfillment-ghi000") {
		t.Fatalf("unfold all should restore children, got %v", workflowIDs(wl.workflows))
	}
}

func TestWorkflowTreeFoldKeys(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.Start()
	wl.loadMockData()
	if !selectWorkflowByID(wl, "order-processing-abc123") {
		t.Fatal("expected parent workflow")
	}
	capture := wl.table.GetInputCapture()
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, ' ', 0)); ev != nil {
		t.Fatal("space should fold the current parent")
	}
	if visibleWorkflowIDsContain(wl, "payment-xyz789") {
		t.Fatal("space should hide children")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModCtrl)); ev != nil {
		t.Fatal("ctrl+space should unfold all")
	}
	if !visibleWorkflowIDsContain(wl, "payment-xyz789") {
		t.Fatal("ctrl+space should restore children")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModMeta)); ev != nil {
		t.Fatal("meta+space should fold all")
	}
	if visibleWorkflowIDsContain(wl, "payment-xyz789") {
		t.Fatal("meta+space should hide descendants")
	}
}

func workflowIDs(workflows []temporal.Workflow) []string {
	ids := make([]string, len(workflows))
	for i, w := range workflows {
		ids[i] = w.ID
	}
	return ids
}

func visibleWorkflowIDsContain(wl *WorkflowList, id string) bool {
	for _, w := range wl.workflows {
		if w.ID == id {
			return true
		}
	}
	return false
}

func selectWorkflowByID(wl *WorkflowList, id string) bool {
	for i, w := range wl.workflows {
		if w.ID == id {
			wl.table.SelectRow(i)
			return true
		}
	}
	return false
}
