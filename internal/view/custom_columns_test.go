package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestCustomColumnExpressions(t *testing.T) {
	start := time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)
	parent := "parent-1"
	w := temporal.Workflow{
		ID:        "order-123-eu",
		RunID:     "run-1",
		Type:      "OrderWorkflow",
		Status:    "Completed",
		TaskQueue: "orders",
		Namespace: "default",
		StartTime: start,
		EndTime:   &end,
		ParentID:  &parent,
		Memo:      map[string]string{"owner": "team-a"},
		SearchAttributes: map[string]string{
			"CustomerId": "acme",
			"Amount":     "42",
			"Tags":       "gold, vip",
		},
		SearchAttributeValues: map[string]any{
			"Amount": 42,
			"Tags":   []any{"gold", "vip"},
		},
	}
	now := end.Add(time.Hour)
	cases := []struct {
		expr string
		want string
	}{
		{`WorkflowId`, "order-123-eu"},
		{`split(WorkflowId, "-")[2] | upper()`, "EU"},
		{`WorkflowType + "@" + TaskQueue`, "OrderWorkflow@orders"},
		{`ExecutionStatus == "Completed" ? "done" : "open"`, "done"},
		{`RunId + " " + Namespace + " " + ParentWorkflowId`, "run-1 default parent-1"},
		{`SearchAttributes.CustomerId`, "acme"},
		{`SearchAttributes.Amount * 2`, "84"},
		{`SearchAttributes.Amount / 8`, "5.25"},
		{`"vip" in SearchAttributes.Tags`, "true"},
		{`SearchAttributes.Tags`, "gold, vip"},
		{`SearchAttributes.Missing ?? "-"`, "-"},
		{`SearchAttributes.Missing > 3`, ""},
		{`ExecutionDuration`, "1m30s"},
		{`ExecutionDuration > duration("1m") ? "slow" : "fast"`, "slow"},
		{`CloseTime`, "1h ago"},
		{`Memo.owner`, "team-a"},
		{`"a\nb"`, "a b"},
	}
	for _, tc := range cases {
		program := compileCustomColumn(tc.expr)
		if program.err != nil {
			t.Fatalf("%s: %v", tc.expr, program.err)
		}
		if got := program.value(now, w, config.TimeFormatRelative); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}

	running := temporal.Workflow{Status: "Running", StartTime: start}
	if got := compileCustomColumn(`ExecutionDuration`).value(start.Add(time.Minute), running, config.TimeFormatRelative); got != "1m0s" {
		t.Fatalf("running duration=%q", got)
	}
	if got := compileCustomColumn(`CloseTime`).value(now, running, config.TimeFormatRelative); got != "-" {
		t.Fatalf("open close time=%q", got)
	}
}

func TestCustomColumnInWorkflowTable(t *testing.T) {
	a := &App{config: &config.Config{
		CustomColumns: []config.CustomColumnConfig{
			{ID: "region", Header: "REGION", Expr: `split(WorkflowId, "-")[1]`},
			{ID: "broken", Expr: `WorkflowId +`},
			{ID: "brackets", Expr: `"[" + WorkflowType + "]"`},
		},
	}}
	wl := NewWorkflowList(a, "default")

	byID := map[string]workflowColumn{}
	var ids []string
	for _, col := range wl.columnLayout() {
		byID[col.id] = col
		ids = append(ids, col.id)
	}
	region := config.CustomColumnID("region")
	if col, ok := byID[region]; !ok || col.header != "REGION" || col.width != config.DefaultWorkflowColumnWidth(region) {
		t.Fatalf("custom columns should be shown by default: %v", ids)
	}
	if byID[config.CustomColumnID("broken")].header != "broken" {
		t.Fatalf("header should default to the id: %+v", byID[config.CustomColumnID("broken")])
	}
	if !strings.Contains(a.statusText, "Custom column broken") {
		t.Fatalf("compile errors should be reported, status=%q", a.statusText)
	}

	cells := wl.styledWorkflowCells(time.Now(), temporal.Workflow{ID: "order-eu", Type: "Pay"}, 0)
	text := map[string]string{}
	for i, id := range ids {
		text[id] = strings.TrimSpace(cells[i].Text)
	}
	if text[region] != "eu" {
		t.Fatalf("region=%q", text[region])
	}
	if text[config.CustomColumnID("broken")] != customColumnErrorText {
		t.Fatalf("broken=%q", text[config.CustomColumnID("broken")])
	}
	if text[config.CustomColumnID("brackets")] != "[Pay[]" {
		t.Fatalf("brackets should be escaped for tview, got %q", text[config.CustomColumnID("brackets")])
	}

	a.config.SetWorkflowColumns([]config.WorkflowColumnConfig{{ID: config.WorkflowColumnWorkflowID, Width: 30}})
	hidden := map[string]bool{}
	for _, item := range wl.columnEditorItems() {
		hidden[item.id] = item.hidden
	}
	if !hidden[region] {
		t.Fatalf("custom columns left out of a saved layout should be offered hidden: %+v", hidden)
	}
	for _, item := range wl.defaultWorkflowColumnEditorItems() {
		if item.id == region && item.hidden {
			t.Fatal("resetting columns should show custom columns again")
		}
	}

	a.config.SetWorkflowColumns(a.config.DefaultWorkflowColumnLayout())
	if a.config.WorkflowColumns != nil {
		t.Fatalf("the default layout, custom columns included, should not be written: %+v", a.config.WorkflowColumns)
	}
}
