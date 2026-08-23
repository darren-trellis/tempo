package config

import (
	"os"
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

func TestDefaultWorkflowColumnsIncludeParent(t *testing.T) {
	found := false
	for _, col := range DefaultWorkflowColumns() {
		if col.ID == WorkflowColumnParentID {
			found = true
			if col.Width != 36 {
				t.Fatalf("parent width=%d", col.Width)
			}
		}
	}
	if !found {
		t.Fatal("default columns should include parent_id")
	}
	if !knownWorkflowColumn(WorkflowColumnParentID) {
		t.Fatal("parent_id should be a known column")
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

func TestPreviewCacheLimit(t *testing.T) {
	if DefaultConfig().PreviewCacheLimit() != DefaultPreviewCacheSize {
		t.Fatalf("default cache size = %d", DefaultConfig().PreviewCacheLimit())
	}
	zero := 0
	if (&Config{PreviewCacheSize: &zero}).PreviewCacheLimit() != 0 {
		t.Fatal("preview_cache_size: 0 should disable the cache")
	}
	huge := 1000
	if (&Config{PreviewCacheSize: &huge}).PreviewCacheLimit() != MaxPreviewCacheSize {
		t.Fatalf("cache size should clamp to %d", MaxPreviewCacheSize)
	}
}

func TestShouldAutoreloadDefault(t *testing.T) {
	if !DefaultConfig().ShouldAutoreload() {
		t.Fatal("autoreload should default to on")
	}
	off := false
	cfg := &Config{Autoreload: &off}
	if cfg.ShouldAutoreload() {
		t.Fatal("autoreload: false should disable reload")
	}
	on := true
	cfg.Autoreload = &on
	if !cfg.ShouldAutoreload() {
		t.Fatal("autoreload: true should enable reload")
	}
}

func TestReadChangedConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := DefaultConfig()
	cfg.Theme = "nord"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := ReadChangedConfigFile(); err != nil || changed {
		t.Fatalf("changed=%v err=%v after save", changed, err)
	}

	if err := os.WriteFile(ConfigPath(), []byte("theme: dracula\nactive_profile: default\nprofiles:\n  default:\n    address: localhost:7233\n    namespace: default\n"), 0644); err != nil {
		t.Fatal(err)
	}
	data, changed, err := ReadChangedConfigFile()
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	parsed, err := ParseConfigFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Theme != "dracula" {
		t.Fatalf("theme=%q", parsed.Theme)
	}
	AcknowledgeConfigFile(data)
	if _, changed, err = ReadChangedConfigFile(); err != nil || changed {
		t.Fatalf("changed=%v err=%v after acknowledge", changed, err)
	}
}

func TestConnectionSettingsEqual(t *testing.T) {
	a := ConnectionConfig{Address: "localhost:7233", Namespace: "default"}
	b := a
	if !ConnectionSettingsEqual(a, b) {
		t.Fatal("expected equal")
	}
	b.CodecEndpoint = "https://codec"
	if ConnectionSettingsEqual(a, b) {
		t.Fatal("expected codec change to differ")
	}
}
