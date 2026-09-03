package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

func TestExpandEnvUIURL(t *testing.T) {
	t.Setenv("TEMPORAL_UI_URL", "https://ui.example.com")
	cfg := ConnectionConfig{
		Address: "localhost:7233",
		UIURL:   "${TEMPORAL_UI_URL}",
	}
	if got := cfg.ExpandEnv().UIURL; got != "https://ui.example.com" {
		t.Fatalf("UIURL = %q", got)
	}
}

func TestResolveUIURL(t *testing.T) {
	t.Setenv("TEMPORAL_UI_URL", "https://ui.from.env")
	cases := []struct {
		name string
		cfg  ConnectionConfig
		want string
	}{
		{name: "explicit", cfg: ConnectionConfig{Address: "prod.example.com:7233", UIURL: "https://temporal.example.com"}, want: "https://temporal.example.com"},
		{name: "explicit trailing slash", cfg: ConnectionConfig{UIURL: "http://localhost:8080/"}, want: "http://localhost:8080"},
		{name: "env", cfg: ConnectionConfig{UIURL: "${TEMPORAL_UI_URL}"}, want: "https://ui.from.env"},
		{name: "localhost", cfg: ConnectionConfig{Address: "localhost:7233"}, want: "http://localhost:8080"},
		{name: "loopback", cfg: ConnectionConfig{Address: "127.0.0.1:7233"}, want: "http://localhost:8080"},
		{name: "cloud", cfg: ConnectionConfig{Address: "my-ns.abc12.tmprl.cloud:7233"}, want: "https://cloud.temporal.io"},
		{name: "cloud regional", cfg: ConnectionConfig{Address: "us-west-2.aws.api.temporal.io:7233"}, want: "https://cloud.temporal.io"},
		{name: "unknown", cfg: ConnectionConfig{Address: "temporal.staging.example.com:7233"}, want: ""},
	}
	for _, tc := range cases {
		if got := tc.cfg.ResolveUIURL(); got != tc.want {
			t.Fatalf("%s: ResolveUIURL()=%q want %q", tc.name, got, tc.want)
		}
	}
}

func TestWorkflowUIURL(t *testing.T) {
	got := WorkflowUIURL("https://cloud.temporal.io/", "orders.abc12", "order/123", "run-1")
	want := "https://cloud.temporal.io/namespaces/orders.abc12/workflows/order%2F123/run-1/history"
	if got != want {
		t.Fatalf("WorkflowUIURL()=%q want %q", got, want)
	}
	if WorkflowUIURL("", "default", "wf", "run") != "" {
		t.Fatal("empty base should yield no URL")
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

func TestMouseScrollStepSize(t *testing.T) {
	if DefaultConfig().MouseScrollStepSize() != DefaultMouseScrollStep {
		t.Fatalf("default mouse scroll step = %d", DefaultConfig().MouseScrollStepSize())
	}
	zero := 0
	if (&Config{MouseScrollStep: &zero}).MouseScrollStepSize() != DefaultMouseScrollStep {
		t.Fatal("mouse_scroll_step below 1 should use the default")
	}
	step := 4
	if (&Config{MouseScrollStep: &step}).MouseScrollStepSize() != 4 {
		t.Fatal("mouse_scroll_step should use the configured value")
	}
	huge := 1000
	if (&Config{MouseScrollStep: &huge}).MouseScrollStepSize() != MaxMouseScrollStep {
		t.Fatalf("mouse_scroll_step should clamp to %d", MaxMouseScrollStep)
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

func TestRefreshRate(t *testing.T) {
	if got := DefaultConfig().RefreshRate(); got != DefaultRefreshRate {
		t.Fatalf("default refresh rate = %s", got)
	}
	var nilCfg *Config
	if got := nilCfg.RefreshRate(); got != DefaultRefreshRate {
		t.Fatalf("nil config refresh rate = %s", got)
	}

	rate := Setting{text: "2s"}
	cfg := &Config{RefreshInterval: &rate}
	if got := cfg.RefreshRate(); got != 2*time.Second {
		t.Fatalf("configured refresh rate = %s", got)
	}

	for _, bad := range []string{"", "   ", "soon", "0s", "-1s"} {
		value := Setting{text: bad}
		if got := (&Config{RefreshInterval: &value}).RefreshRate(); got != DefaultRefreshRate {
			t.Fatalf("%q should fall back, got %s", bad, got)
		}
	}

	tiny, huge := Setting{text: "1ms"}, Setting{text: "1h"}
	if got := (&Config{RefreshInterval: &tiny}).RefreshRate(); got != MinRefreshRate {
		t.Fatalf("tiny rate should clamp, got %s", got)
	}
	if got := (&Config{RefreshInterval: &huge}).RefreshRate(); got != MaxRefreshRate {
		t.Fatalf("huge rate should clamp, got %s", got)
	}

	cfg, err := parseConfig([]byte("refresh_rate: 500ms\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.RefreshRate(); got != 500*time.Millisecond {
		t.Fatalf("yaml refresh_rate = %s", got)
	}

	cfg, err = parseConfig([]byte("refresh_rate: 2\n"))
	if err != nil {
		t.Fatalf("parse seconds: %v", err)
	}
	if got := cfg.RefreshRate(); got != 2*time.Second {
		t.Fatalf("bare seconds refresh_rate = %s", got)
	}
}

func TestWorkerQuietWindows(t *testing.T) {
	if got := DefaultConfig().WorkerPollQuietAfter(); got != DefaultWorkerPollQuietAfter {
		t.Fatalf("default poll window = %s", got)
	}
	if got := DefaultConfig().WorkerHeartbeatQuietAfter(); got != DefaultWorkerHeartbeatQuietAfter {
		t.Fatalf("default heartbeat window = %s", got)
	}
	var nilCfg *Config
	if got := nilCfg.WorkerPollQuietAfter(); got != DefaultWorkerPollQuietAfter {
		t.Fatalf("nil config poll window = %s", got)
	}

	poll, beat := Setting{text: "30s"}, Setting{text: "2m30s"}
	cfg := &Config{WorkerPollQuiet: &poll, WorkerHeartbeatQuiet: &beat}
	if got := cfg.WorkerPollQuietAfter(); got != 30*time.Second {
		t.Fatalf("configured poll window = %s", got)
	}
	if got := cfg.WorkerHeartbeatQuietAfter(); got != 150*time.Second {
		t.Fatalf("configured heartbeat window = %s", got)
	}

	// Unusable values fall back rather than making everything look stale.
	for _, bad := range []string{"", "   ", "soon", "0s", "-5s"} {
		value := Setting{text: bad}
		if got := (&Config{WorkerPollQuiet: &value}).WorkerPollQuietAfter(); got != DefaultWorkerPollQuietAfter {
			t.Fatalf("%q should fall back, got %s", bad, got)
		}
	}

	tiny, huge := Setting{text: "1ms"}, Setting{text: "48h"}
	if got := (&Config{WorkerPollQuiet: &tiny}).WorkerPollQuietAfter(); got != MinWorkerQuietAfter {
		t.Fatalf("tiny window should clamp, got %s", got)
	}
	if got := (&Config{WorkerPollQuiet: &huge}).WorkerPollQuietAfter(); got != MaxWorkerQuietAfter {
		t.Fatalf("huge window should clamp, got %s", got)
	}
}

// A mistyped setting must never cost someone their whole config.
func TestSettingToleratesHowPeopleWriteDurations(t *testing.T) {
	cases := map[string]time.Duration{
		"worker_poll_quiet_after: 45s":   45 * time.Second,
		"worker_poll_quiet_after: 2m30s": 150 * time.Second,
		"worker_poll_quiet_after: 45":    45 * time.Second,
		"worker_poll_quiet_after: 45.5":  45500 * time.Millisecond,
		`worker_poll_quiet_after: "60s"`: time.Minute,
	}
	for doc, want := range cases {
		cfg, err := parseConfig([]byte(doc + "\n"))
		if err != nil {
			t.Fatalf("%s: %v", doc, err)
		}
		if got := cfg.WorkerPollQuietAfter(); got != want {
			t.Fatalf("%s: got %s want %s", doc, got, want)
		}
	}

	// Nonsense is ignored, and crucially the rest of the file still loads.
	cfg, err := parseConfig([]byte("theme: kanagawa\nworker_poll_quiet_after: whenever\n"))
	if err != nil {
		t.Fatalf("a bad duration should not fail the load: %v", err)
	}
	if cfg.Theme != "kanagawa" {
		t.Fatalf("the rest of the config should survive, theme=%q", cfg.Theme)
	}
	if got := cfg.WorkerPollQuietAfter(); got != DefaultWorkerPollQuietAfter {
		t.Fatalf("nonsense should fall back, got %s", got)
	}

	// A written-out value round-trips.
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "worker_poll_quiet_after: whenever") {
		t.Fatalf("the value should be preserved verbatim:\n%s", out)
	}
}

func TestWorkerQuietWindowsRoundTripYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("worker_poll_quiet_after: 45s\nworker_heartbeat_quiet_after: 4m\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkerPollQuietAfter(); got != 45*time.Second {
		t.Fatalf("poll window from yaml = %s", got)
	}
	if got := cfg.WorkerHeartbeatQuietAfter(); got != 4*time.Minute {
		t.Fatalf("heartbeat window from yaml = %s", got)
	}
}
