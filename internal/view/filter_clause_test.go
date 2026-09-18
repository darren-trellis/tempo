package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
)

func TestCompileFilterClauses(t *testing.T) {
	got := compileFilterClauses([]config.FilterClause{
		{Key: "ExecutionStatus", Op: filterOpEq, Value: "Running"},
		{Key: "StartTime", Op: filterOpAfter, Value: "$HOUR_AGO"},
	})
	want := "ExecutionStatus = 'Running' AND StartTime > $HOUR_AGO"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got = compileFilterClauses([]config.FilterClause{
		{Key: "WorkflowId", Op: filterOpStartsWith, Value: "order-"},
	})
	if got != "WorkflowId STARTS_WITH 'order-'" {
		t.Fatalf("starts with: %q", got)
	}

	got = compileFilterClauses([]config.FilterClause{
		{Key: "StartTime", Op: filterOpBefore, Value: "2024-01-02T03:04:05Z"},
	})
	if got != "StartTime < '2024-01-02T03:04:05Z'" {
		t.Fatalf("rfc3339: %q", got)
	}
}

func TestCompiledFilterQueryPrefersStoredQuery(t *testing.T) {
	f := config.SavedFilter{Query: "WorkflowType = 'Order'"}
	if got := compiledFilterQuery(f); got != "WorkflowType = 'Order'" {
		t.Fatalf("stored query: %q", got)
	}
	// Clauses only survive in configs written before filters became queries.
	legacy := config.SavedFilter{Clauses: []config.FilterClause{{Key: "WorkflowType", Op: filterOpEq, Value: "Pay"}}}
	if got := compiledFilterQuery(legacy); got != "WorkflowType = 'Pay'" {
		t.Fatalf("legacy clauses: %q", got)
	}
}

func TestParseFilterDateTime(t *testing.T) {
	got, err := parseFilterDateTime("$TODAY")
	if err != nil || got != "$TODAY" {
		t.Fatalf("placeholder: %q %v", got, err)
	}
	got, err = parseFilterDateTime("2024-06-01 13:45")
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || got == "2024-06-01 13:45" {
		t.Fatalf("should convert to RFC3339 UTC, got %q", got)
	}
}
