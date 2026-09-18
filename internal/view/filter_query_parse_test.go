package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
)

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
		`(WorkflowType = 'A' AND CustomerId = 'x')`,
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
