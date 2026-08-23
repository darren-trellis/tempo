package config

import (
	"testing"
)

func TestExpandEnvCodecEndpoint(t *testing.T) {
	t.Setenv("TEMPORAL_CODEC_URL", "https://codec.example.com")
	cfg := ConnectionConfig{
		Address:       "localhost:7233",
		Namespace:     "default",
		CodecEndpoint: "${TEMPORAL_CODEC_URL}",
	}
	expanded := cfg.ExpandEnv()
	if expanded.CodecEndpoint != "https://codec.example.com" {
		t.Fatalf("CodecEndpoint = %q", expanded.CodecEndpoint)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "https://codec"); got != "https://codec" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveWorkflowColumnsDefaults(t *testing.T) {
	got := ResolveWorkflowColumns(nil)
	want := DefaultWorkflowColumns()
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("col[%d]=%+v want %+v", i, got[i], want[i])
		}
	}
}

func TestResolveWorkflowColumnsOrderAndUnknown(t *testing.T) {
	got := ResolveWorkflowColumns([]WorkflowColumnConfig{
		{ID: "run_id", Width: 10},
		{ID: "nope", Width: 99},
		{ID: "STATUS", Width: 0},
		{ID: "run_id", Width: 20},
		{ID: "workflow_id", Width: 2},
	})
	want := []WorkflowColumnConfig{
		{ID: WorkflowColumnRunID, Width: 10},
		{ID: WorkflowColumnStatus, Width: DefaultWorkflowColumnWidth(WorkflowColumnStatus)},
		{ID: WorkflowColumnWorkflowID, Width: MinWorkflowColumnWidth},
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("col[%d]=%+v want %+v", i, got[i], want[i])
		}
	}
}

func TestSetWorkflowColumnsOmitsDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.SetWorkflowColumns(DefaultWorkflowColumns())
	if cfg.WorkflowColumns != nil {
		t.Fatalf("expected defaults to be omitted, got %+v", cfg.WorkflowColumns)
	}

	cfg.SetWorkflowColumns([]WorkflowColumnConfig{{ID: WorkflowColumnWorkflowID, Width: 40}})
	if len(cfg.WorkflowColumns) != 1 || cfg.WorkflowColumns[0].Width != 40 {
		t.Fatalf("got %+v", cfg.WorkflowColumns)
	}
}
