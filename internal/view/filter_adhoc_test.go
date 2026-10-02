package view

import (
	"strings"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
)

func adHocWorkflowList(t *testing.T, cfg *config.Config) (*App, *WorkflowList) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	return a, wl
}

func TestAdHocClauseAppendsToTheActiveFilterWithoutSaving(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running", Query: "ExecutionStatus = 'Running'"}}
	_, wl := adHocWorkflowList(t, cfg)
	wl.applySavedFilter(cfg.SavedFilters[0])

	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})

	want := "ExecutionStatus = 'Running' AND WorkflowType = 'Order'"
	if wl.visibilityQuery != want {
		t.Fatalf("query=%q want %q", wl.visibilityQuery, want)
	}
	if wl.activeFilterName != "Running" {
		t.Fatalf("the filter should stay active, got %q", wl.activeFilterName)
	}
	if got := cfg.SavedFiltersFor("local")[0].Query; got != "ExecutionStatus = 'Running'" {
		t.Fatalf("ad-hoc clauses should not be saved, stored %q", got)
	}

	items := filterBarItems(wl)
	if len(items) != 2 || !items[1].active || items[1].label != "Running*" {
		t.Fatalf("active chip should be marked unsaved, got %+v", items)
	}
	if items[1].name != "Running" {
		t.Fatalf("the asterisk should stay out of the chip name, got %q", items[1].name)
	}
}

func TestAdHocClausesStack(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running", Query: "ExecutionStatus = 'Running'"}}
	_, wl := adHocWorkflowList(t, cfg)
	wl.applySavedFilter(cfg.SavedFilters[0])

	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})
	wl.appendAdHocClauses([]config.FilterClause{{Key: "CloseTime", Op: filterOpIsNull}})

	want := "ExecutionStatus = 'Running' AND WorkflowType = 'Order' AND CloseTime IS NULL"
	if wl.visibilityQuery != want {
		t.Fatalf("query=%q want %q", wl.visibilityQuery, want)
	}
}

func TestAdHocClauseOnAllMarksTheAllChip(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running", Query: "ExecutionStatus = 'Running'"}}
	_, wl := adHocWorkflowList(t, cfg)

	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})

	if wl.visibilityQuery != "WorkflowType = 'Order'" {
		t.Fatalf("query=%q", wl.visibilityQuery)
	}
	items := filterBarItems(wl)
	if !items[0].active || items[0].label != "All*" {
		t.Fatalf("All should be marked unsaved, got %+v", items)
	}
	if items[1].active {
		t.Fatalf("no saved filter should be active, got %+v", items)
	}
}

func TestAdHocClausesShowOnTheStatusBar(t *testing.T) {
	a, wl := adHocWorkflowList(t, config.DefaultConfig())
	wl.appendAdHocClauses([]config.FilterClause{
		{Key: "WorkflowType", Op: filterOpEq, Value: "Order"},
		{Key: "CloseTime", Op: filterOpIsNotNull},
	})

	var b strings.Builder
	for _, seg := range a.statusBarSegments() {
		b.WriteString(seg.text)
	}
	got := b.String()
	if !strings.Contains(got, "WorkflowType Equals Order") {
		t.Fatalf("status bar should list the ad-hoc clauses, got %q", got)
	}
	if !strings.Contains(got, "CloseTime "+filterOpLabel(filterOpIsNotNull)) {
		t.Fatalf("status bar should list every ad-hoc clause, got %q", got)
	}
}

func TestSwitchingFiltersCarriesAdHocClauses(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "Running", Query: "ExecutionStatus = 'Running'"},
		{Name: "Failed", Query: "ExecutionStatus = 'Failed'"},
	}
	_, wl := adHocWorkflowList(t, cfg)
	wl.applySavedFilter(cfg.SavedFilters[0])
	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})

	wl.applySavedFilter(cfg.SavedFilters[1])
	if want := "ExecutionStatus = 'Failed' AND WorkflowType = 'Order'"; wl.visibilityQuery != want {
		t.Fatalf("query=%q want %q", wl.visibilityQuery, want)
	}
	items := filterBarItems(wl)
	if items[1].label != "Running" || items[2].label != "Failed*" || !items[2].active {
		t.Fatalf("the new filter should carry the asterisk, got %+v", items)
	}

	wl.applyAllWorkflowsFilter()
	if wl.visibilityQuery != "WorkflowType = 'Order'" || wl.activeFilterName != "" {
		t.Fatalf("All should keep the ad-hoc clauses, name=%q query=%q", wl.activeFilterName, wl.visibilityQuery)
	}
	if items := filterBarItems(wl); items[0].label != "All*" {
		t.Fatalf("All should be marked unsaved, got %+v", items)
	}

	wl.clearTransientWorkflowFilter()
	if wl.adHoc.active() || wl.visibilityQuery != "" {
		t.Fatalf("peeling the clauses off All should leave nothing, query=%q", wl.visibilityQuery)
	}
}

func TestClearingFiltersDropsAdHocClauses(t *testing.T) {
	_, wl := adHocWorkflowList(t, config.DefaultConfig())
	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})

	wl.clearAllFilters()
	if wl.adHoc.active() || wl.adHocSummary() != "" {
		t.Fatal("clearing filters should drop the ad-hoc clauses")
	}
}

func TestAdHocClausesSurviveARawFilterTest(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running", Query: "ExecutionStatus = 'Running'"}}
	_, wl := adHocWorkflowList(t, cfg)
	wl.applySavedFilter(cfg.SavedFilters[0])
	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})

	wl.testVisibilityQuery("WorkflowId STARTS_WITH 'order-'")
	if wl.adHoc.active() {
		t.Fatal("a test run should stand on its own")
	}

	wl.restoreFilterTest()
	if !wl.adHoc.active() {
		t.Fatal("a failed test should put the ad-hoc clauses back")
	}
	if wl.visibilityQuery != "ExecutionStatus = 'Running' AND WorkflowType = 'Order'" {
		t.Fatalf("query=%q", wl.visibilityQuery)
	}
}

func TestAdHocClauseParenthesisesADisjunctiveFilter(t *testing.T) {
	cfg := config.DefaultConfig()
	f := config.SavedFilter{Name: "Open", Query: "ExecutionStatus = 'Running' OR ExecutionStatus = 'ContinuedAsNew'"}
	cfg.SavedFilters = []config.SavedFilter{f}
	_, wl := adHocWorkflowList(t, cfg)
	wl.applySavedFilter(f)

	wl.appendAdHocClauses([]config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Order"}})

	want := "(ExecutionStatus = 'Running' OR ExecutionStatus = 'ContinuedAsNew') AND WorkflowType = 'Order'"
	if wl.visibilityQuery != want {
		t.Fatalf("query=%q want %q", wl.visibilityQuery, want)
	}
}

func TestQueryHasTopLevelOr(t *testing.T) {
	cases := []struct {
		query string
		want  bool
	}{
		{"ExecutionStatus = 'Running'", false},
		{"A = 1 OR B = 2", true},
		{"A = 1 or B = 2", true},
		{"(A = 1 OR B = 2) AND C = 3", false},
		{"WorkflowId = 'a or b'", false},
		{"WorkflowType = 'Order'", false},
		{"Organization = 'x'", false},
	}
	for _, tc := range cases {
		if got := queryHasTopLevelOr(tc.query); got != tc.want {
			t.Errorf("queryHasTopLevelOr(%q)=%v want %v", tc.query, got, tc.want)
		}
	}
}

func TestFilterKeysOpenTheClauseEditorAndTheManager(t *testing.T) {
	cfg := config.DefaultConfig()
	a, wl := adHocWorkflowList(t, cfg)
	wl.Start()

	capture := wl.table.GetInputCapture()
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone)); ev != nil {
		t.Fatal("f should be handled")
	}
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("f should open a modal, current=%T", a.app.Pages().Current())
	}
	if hintDescription(om.Hints(), "Ctrl+T") != "Test" {
		t.Fatalf("f should open the clause editor, got hints %+v", om.Hints())
	}
	wl.closeModal()

	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModNone)); ev != nil {
		t.Fatal("F should be handled")
	}
	om, ok = a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("F should open a modal, current=%T", a.app.Pages().Current())
	}
	if hintDescription(om.Hints(), "n") != "New Filter" {
		t.Fatalf("F should open the Filters dialog, got hints %+v", om.Hints())
	}
}
