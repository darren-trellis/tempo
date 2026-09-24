package view

import (
	"strings"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
)

// parseVisibilityQuery reports the query as form clauses when it is nothing
// but an AND of comparisons the form can edit and recompile without changing
// their meaning.
func parseVisibilityQuery(query string) ([]config.FilterClause, bool) {
	clauses := filterClausesFromQuery(query)
	if len(clauses) == 0 {
		return nil, false
	}
	for _, clause := range clauses {
		if isRawFilterClause(clause) {
			return nil, false
		}
	}
	return clauses, true
}

func TestParseVisibilityQueryProducesClauses(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []config.FilterClause
	}{
		{
			name:  "single equality",
			query: `CustomerId = "acme"`,
			want:  []config.FilterClause{{Key: "CustomerId", Op: filterOpEq, Value: "acme"}},
		},
		{
			name:  "conjunction",
			query: `WorkflowType = 'Sync' AND ExecutionStatus != 'Running'`,
			want: []config.FilterClause{
				{Key: "WorkflowType", Op: filterOpEq, Value: "Sync"},
				{Key: "ExecutionStatus", Op: filterOpNeq, Value: "Running"},
			},
		},
		{
			name:  "starts with and time bounds",
			query: `WorkflowId STARTS_WITH 'order-' and StartTime > 2026-01-01T00:00:00Z`,
			want: []config.FilterClause{
				{Key: "WorkflowId", Op: filterOpStartsWith, Value: "order-"},
				{Key: "StartTime", Op: filterOpAfter, Value: "2026-01-01T00:00:00Z"},
			},
		},
		{
			name:  "escaped quote in value",
			query: `CustomerId = 'o''brien'`,
			want:  []config.FilterClause{{Key: "CustomerId", Op: filterOpEq, Value: "o'brien"}},
		},
		{
			name:  "value containing and",
			query: `CustomerId = 'research and development'`,
			want:  []config.FilterClause{{Key: "CustomerId", Op: filterOpEq, Value: "research and development"}},
		},
		{
			name:  "redundant parentheses",
			query: `(WorkflowType = 'A' AND CustomerId = 'x')`,
			want: []config.FilterClause{
				{Key: "WorkflowType", Op: filterOpEq, Value: "A"},
				{Key: "CustomerId", Op: filterOpEq, Value: "x"},
			},
		},
		{
			name:  "identifier prefixed with and",
			query: `AndroidId = 'pixel'`,
			want:  []config.FilterClause{{Key: "AndroidId", Op: filterOpEq, Value: "pixel"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseVisibilityQuery(tt.query)
			if !ok {
				t.Fatalf("parseVisibilityQuery(%q) reported unsupported", tt.query)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d clauses, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("clause %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseVisibilityQueryKeepsComplexQueriesRaw(t *testing.T) {
	for _, query := range []string{
		`WorkflowType = 'A' OR WorkflowType = 'B'`,
		`(WorkflowType = 'A' OR CustomerId = 'x')`,
		`ExecutionStatus IN ('Running', 'Failed')`,
		`StartTime BETWEEN '2026-01-01' AND '2026-02-01'`,
		`CloseTime >= '2026-01-01T00:00:00Z'`,
		`CustomerId = 'unterminated`,
	} {
		if _, ok := parseVisibilityQuery(query); ok {
			t.Errorf("parseVisibilityQuery(%q) should stay raw", query)
		}
		clauses := filterClausesFromQuery(query)
		if len(clauses) != 1 || clauses[0].Op != filterOpRaw {
			t.Errorf("filterClausesFromQuery(%q) = %+v, want single raw clause", query, clauses)
		}
		if clauses[0].Value != query {
			t.Errorf("raw clause value = %q, want %q", clauses[0].Value, query)
		}
	}
}

func TestFilterClausesFromQueryRoundTrips(t *testing.T) {
	query := `CustomerId = "acme" AND ExecutionStatus = 'Running'`
	clauses := filterClausesFromQuery(query)
	compiled := compileFilterClauses(clauses)
	got := filterClausesFromQuery(compiled)
	if len(got) != len(clauses) {
		t.Fatalf("round trip changed clause count: %d -> %d", len(clauses), len(got))
	}
	for i := range got {
		if got[i] != clauses[i] {
			t.Errorf("clause %d = %+v, want %+v", i, got[i], clauses[i])
		}
	}
}

// Filters are stored as a query and parsed back into builder rows, so every
// built-in filter has to survive that trip unchanged.
func TestDefaultSavedFiltersRoundTrip(t *testing.T) {
	for _, f := range config.DefaultSavedFilters() {
		t.Run(f.Name, func(t *testing.T) {
			if f.Query == "" {
				t.Fatal("built-in filters should be stored as a query")
			}
			if len(f.Clauses) != 0 {
				t.Fatal("built-in filters should not carry clauses")
			}
			clauses := savedFilterClauses(f)
			if len(clauses) == 0 {
				t.Fatal("filter parsed into no clauses")
			}
			if got := compileFilterClauses(clauses); got != f.Query {
				t.Errorf("round trip changed the query:\n  stored:   %s\n  compiled: %s", f.Query, got)
			}
		})
	}
}

// Relative filters only stay relative if the placeholder survives storage.
func TestRelativeDefaultFiltersKeepPlaceholders(t *testing.T) {
	want := map[string]string{
		"Started Last 24 Hours": "$HOURS_AGO_24",
		"Started Today":         "$TODAY",
		"Long Running (>6h)":    "$HOURS_AGO_6",
	}
	for _, f := range config.DefaultSavedFilters() {
		placeholder, ok := want[f.Name]
		if !ok {
			continue
		}
		if !strings.Contains(f.Query, placeholder) {
			t.Errorf("%s = %q, want it to keep %s", f.Name, f.Query, placeholder)
		}
		clauses := savedFilterClauses(f)
		found := false
		for _, c := range clauses {
			if strings.Contains(c.Value, placeholder) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s lost %s when parsed into clauses: %+v", f.Name, placeholder, clauses)
		}
	}
}

func TestMigrateSavedFiltersDropsClauses(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "legacy", Clauses: []config.FilterClause{
			{Key: "ExecutionStatus", Op: filterOpEq, Value: "Running"},
			{Key: "StartTime", Op: filterOpAfter, Value: "$TODAY"},
		}},
		{Name: "already a query", Query: "WorkflowType = 'Order'"},
	}

	if !migrateSavedFilters(cfg) {
		t.Fatal("migration should report a change")
	}
	legacy := cfg.SavedFilters[0]
	if len(legacy.Clauses) != 0 {
		t.Errorf("clauses should be dropped, got %+v", legacy.Clauses)
	}
	if legacy.Query != "ExecutionStatus = 'Running' AND StartTime > $TODAY" {
		t.Errorf("migrated query = %q", legacy.Query)
	}
	if cfg.SavedFilters[1].Query != "WorkflowType = 'Order'" {
		t.Errorf("query-only filter should be untouched, got %q", cfg.SavedFilters[1].Query)
	}
	if migrateSavedFilters(cfg) {
		t.Error("second migration should be a no-op")
	}
}

func TestNullOperatorsCompileAndParse(t *testing.T) {
	cases := []struct {
		clause config.FilterClause
		query  string
	}{
		{config.FilterClause{Key: "CustomerId", Op: filterOpIsNull}, "CustomerId IS NULL"},
		{config.FilterClause{Key: "CustomerId", Op: filterOpIsNotNull}, "CustomerId IS NOT NULL"},
		{config.FilterClause{Key: "CloseTime", Op: filterOpIsNull}, "CloseTime IS NULL"},
	}
	for _, tt := range cases {
		t.Run(tt.query, func(t *testing.T) {
			if got := compileFilterClause(tt.clause); got != tt.query {
				t.Fatalf("compiled %q, want %q", got, tt.query)
			}
			parsed, ok := parseVisibilityQuery(tt.query)
			if !ok || len(parsed) != 1 {
				t.Fatalf("parse(%q) = %+v, ok=%v", tt.query, parsed, ok)
			}
			if parsed[0] != tt.clause {
				t.Fatalf("parsed %+v, want %+v", parsed[0], tt.clause)
			}
		})
	}
}

func TestNullOperatorsMixWithOtherClauses(t *testing.T) {
	query := "ExecutionStatus = 'Running' AND CloseTime IS NULL AND CustomerId IS NOT NULL"
	clauses, ok := parseVisibilityQuery(query)
	if !ok {
		t.Fatalf("parse(%q) reported unsupported", query)
	}
	want := []config.FilterClause{
		{Key: "ExecutionStatus", Op: filterOpEq, Value: "Running"},
		{Key: "CloseTime", Op: filterOpIsNull},
		{Key: "CustomerId", Op: filterOpIsNotNull},
	}
	if len(clauses) != len(want) {
		t.Fatalf("got %d clauses, want %d: %+v", len(clauses), len(want), clauses)
	}
	for i := range want {
		if clauses[i] != want[i] {
			t.Errorf("clause %d = %+v, want %+v", i, clauses[i], want[i])
		}
	}
	if got := compileFilterClauses(clauses); got != query {
		t.Errorf("round trip = %q, want %q", got, query)
	}
}

// "IS NULL" only counts as an operator when it is the whole right-hand side.
func TestIsNullNotMatchedInValues(t *testing.T) {
	clauses, ok := parseVisibilityQuery("CustomerId = 'IS NULL'")
	if !ok || len(clauses) != 1 {
		t.Fatalf("parse = %+v, ok=%v", clauses, ok)
	}
	if clauses[0].Op != filterOpEq || clauses[0].Value != "IS NULL" {
		t.Errorf("clause = %+v, want an equality on the literal", clauses[0])
	}
}

func TestNullOperatorsOfferedOnEveryKey(t *testing.T) {
	has := func(labels []string, want string) bool {
		for _, l := range labels {
			if l == want {
				return true
			}
		}
		return false
	}
	// "" is the key a fresh clause starts with, and "MyCustomAttr" stands in
	// for an attribute the catalog has not loaded yet.
	for _, key := range []string{"", "WorkflowId", "RunId", "WorkflowType", "TaskQueue", "ExecutionStatus", "StartTime", "CloseTime", "MyCustomAttr"} {
		labels := filterOpLabelsForKey(key)
		if !has(labels, "Is Null") || !has(labels, "Is Not Null") {
			t.Errorf("operators for %q = %v, want the null checks", key, labels)
		}
		if got := len(labels); got != len(uniqueStrings(labels)) {
			t.Errorf("operators for %q contain duplicates: %v", key, labels)
		}
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
