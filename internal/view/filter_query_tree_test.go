package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// describeFilterTree writes a tree as AND(...)/OR(...) around leaf summaries,
// with raw leaves in braces.
func describeFilterTree(n *filterNode) string {
	if n == nil {
		return "<nil>"
	}
	if n.isLeaf() {
		if isRawFilterClause(n.clause) {
			return "{" + n.clause.Value + "}"
		}
		return strings.Replace(filterClauseSummary(n.clause), " Equals ", " = ", 1)
	}
	parts := make([]string, len(n.children))
	for i, child := range n.children {
		parts[i] = describeFilterTree(child)
	}
	return n.op + "(" + strings.Join(parts, ", ") + ")"
}

func TestParseFilterTreeGroups(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{`WorkflowType = 'A'`, "WorkflowType = A"},
		{`WorkflowType = 'A' AND ExecutionStatus = 'Running'`, "AND(WorkflowType = A, ExecutionStatus = Running)"},
		{`WorkflowType = 'A' OR WorkflowType = 'B' AND TaskQueue = 'q'`, "OR(WorkflowType = A, AND(WorkflowType = B, TaskQueue = q))"},
		{`(WorkflowType = 'A' AND TaskQueue = 'q') OR ExecutionStatus = 'Failed'`, "OR(AND(WorkflowType = A, TaskQueue = q), ExecutionStatus = Failed)"},
		{`WorkflowType = 'A' AND (ExecutionStatus = 'Running' OR (ExecutionStatus = 'Failed' AND TaskQueue = 'q'))`,
			"AND(WorkflowType = A, OR(ExecutionStatus = Running, AND(ExecutionStatus = Failed, TaskQueue = q)))"},
		{`WorkflowType = 'A' AND (TaskQueue = 'q' AND RunId = 'r')`, "AND(WorkflowType = A, TaskQueue = q, RunId = r)"},
		{`((WorkflowType = 'A'))`, "WorkflowType = A"},
		{`StartTime BETWEEN '2026-01-01' AND '2026-02-01' OR WorkflowType = 'A'`, "OR(StartTime Between 2026-01-01 and 2026-02-01, WorkflowType = A)"},
		{`ExecutionStatus IN ('Running', 'Failed') and WorkflowType = 'A'`, "AND(ExecutionStatus In Running, Failed, WorkflowType = A)"},
		{`CustomerId = 'r&d (and) or more' AND AndroidId = 'x'`, "AND(CustomerId = r&d (and) or more, AndroidId = x)"},
		{`WorkflowType = 'A' AND (TaskQueue = 'q'`, "{WorkflowType = 'A' AND (TaskQueue = 'q'}"},
		{`WorkflowType = 'A' OR`, "{WorkflowType = 'A' OR}"},
		{`CustomerId = 'unterminated`, "{CustomerId = 'unterminated}"},
	}
	for _, tc := range cases {
		if got := describeFilterTree(parseFilterTree(tc.query)); got != tc.want {
			t.Errorf("parse(%q)\n  got  %s\n  want %s", tc.query, got, tc.want)
		}
	}
	if parseFilterTree("  ") != nil {
		t.Error("an empty query should have no tree")
	}
}

func TestFilterTreeQueryKeepsUntouchedText(t *testing.T) {
	query := `CustomIntField = 5 AND (ExecutionStatus = "Running" OR ExecutionStatus = 'Failed')`
	root := parseFilterTree(query)
	if got := root.query(nil); got != query {
		t.Fatalf("unchanged tree should compile to its source:\n  got  %s\n  want %s", got, query)
	}

	edited := root.replaced([]int{1, 0}, parseFilterTree(`ExecutionStatus = 'Completed'`))
	want := `CustomIntField = 5 AND (ExecutionStatus = 'Completed' OR ExecutionStatus = 'Failed')`
	if got := edited.query(nil); got != want {
		t.Fatalf("edit\n  got  %s\n  want %s", got, want)
	}
	if got := root.query(nil); got != query {
		t.Fatalf("replaced should not modify the original tree, got %s", got)
	}

	collapsed := root.replaced([]int{1, 1}, nil)
	if got := collapsed.query(nil); got != `CustomIntField = 5 AND ExecutionStatus = "Running"` {
		t.Fatalf("a group left with one clause should collapse, got %s", got)
	}

	nested := root.replaced([]int{0}, parseFilterTree(`WorkflowType = 'A' OR WorkflowType = 'B'`))
	want = `(WorkflowType = 'A' OR WorkflowType = 'B') AND (ExecutionStatus = "Running" OR ExecutionStatus = 'Failed')`
	if got := nested.query(nil); got != want {
		t.Fatalf("an OR dropped into an AND needs parentheses\n  got  %s\n  want %s", got, want)
	}

	merged := root.replaced([]int{1, 1}, parseFilterTree(`ExecutionStatus = 'Failed' OR ExecutionStatus = 'TimedOut'`))
	if got := describeFilterTree(merged); got != "AND(CustomIntField = 5, OR(ExecutionStatus = Running, ExecutionStatus = Failed, ExecutionStatus = TimedOut))" {
		t.Fatalf("an OR dropped into an OR should merge, got %s", got)
	}

	if parseFilterTree(`WorkflowType = 'A'`).replaced([]int{}, nil) != nil {
		t.Fatal("removing the only clause should leave nothing")
	}
}

func TestBuilderClausesKeepGroupsAsRawRows(t *testing.T) {
	query := `WorkflowType = 'A' AND (ExecutionStatus = 'Running' OR ExecutionStatus = 'Failed')`
	clauses := filterClausesFromQuery(query)
	want := []config.FilterClause{
		{Key: "WorkflowType", Op: filterOpEq, Value: "A"},
		{Key: filterOpRaw, Op: filterOpRaw, Value: "(ExecutionStatus = 'Running' OR ExecutionStatus = 'Failed')"},
	}
	if len(clauses) != len(want) || clauses[0] != want[0] || clauses[1] != want[1] {
		t.Fatalf("clauses = %+v", clauses)
	}
	if got := compileFilterClauses(clauses); got != query {
		t.Fatalf("round trip = %s", got)
	}

	typed := compileFilterClauses([]config.FilterClause{
		{Key: "WorkflowType", Op: filterOpEq, Value: "A"},
		{Key: filterOpRaw, Op: filterOpRaw, Value: "TaskQueue = 'a' OR TaskQueue = 'b'"},
	})
	if typed != "WorkflowType = 'A' AND (TaskQueue = 'a' OR TaskQueue = 'b')" {
		t.Fatalf("a raw OR joined with AND must keep its meaning, got %s", typed)
	}
}

func TestFilterManagerShowsClauseTree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "Orders", Query: `(WorkflowType = 'Order' AND CustomerId = 'q') OR ExecutionStatus = 'Failed'`},
		{Name: "Single", Query: `WorkflowType = 'Sync'`},
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	table := filterManagerTable(t, a, wl)

	screen := drawCurrentModal(t, a)
	for _, want := range []string{
		"Orders",
		"├── ∨ all of",
		"│   ├── ∧ WorkflowType Equals Order",
		"│   └── ∧ CustomerId Equals q",
		"└── ∨ ExecutionStatus Equals Failed",
		"Single",
		"└── ∧ WorkflowType Equals Sync",
	} {
		if !strings.Contains(screen, want) {
			t.Fatalf("filters tree should show %q, got:\n%s", want, screen)
		}
	}

	press := func(r rune) {
		t.Helper()
		if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)); ev != nil {
			t.Fatalf("%c should be handled", r)
		}
	}

	table.SelectRow(3)
	press('e')
	editor := a.app.Pages().Current().(*overlayModal)
	form, ok := editor.body.(*components.Tabs).GetActiveTab().Content.(*components.Form)
	if !ok {
		t.Fatalf("e on a clause should open the clause editor, body=%T", editor.body)
	}
	field, ok := form.GetTextField("value")
	if !ok {
		t.Fatal("clause editor should have a value field")
	}
	if field.GetValue() != "q" {
		t.Fatalf("clause editor should start from the clause, value=%q", field.GetValue())
	}
	field.SetValue("fast")
	form.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if got := cfg.SavedFilters[0].Query; got != `(WorkflowType = 'Order' AND CustomerId = 'fast') OR ExecutionStatus = 'Failed'` {
		t.Fatalf("editing a clause should rewrite only that clause, query=%s", got)
	}
	if _, ok := a.app.Pages().Current().(*overlayModal).body.(*charScrollView); !ok {
		t.Fatal("saving a clause should return to the Filters dialog")
	}

	table.SelectRow(2)
	press('d')
	if got := cfg.SavedFilters[0].Query; got != `CustomerId = 'fast' OR ExecutionStatus = 'Failed'` {
		t.Fatalf("deleting a clause should collapse its group, query=%s", got)
	}

	table.SelectRow(5)
	press('d')
	if got := cfg.SavedFilters[1].Query; got != `WorkflowType = 'Sync'` {
		t.Fatalf("the last clause of a filter must not be deleted, query=%s", got)
	}

	table.SelectRow(0)
	press('e')
	if _, ok := a.app.Pages().Current().(*overlayModal).body.(*components.Tabs); !ok {
		t.Fatalf("e on a filter should open the filter editor, body=%T", a.app.Pages().Current().(*overlayModal).body)
	}
}
