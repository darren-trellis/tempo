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

func TestCloudAPIKeyError(t *testing.T) {
	t.Setenv("TEMPORAL_TS_API_KEY_BETA", "")
	cfg := ConnectionConfig{
		Address: "us-west-2.aws.api.temporal.io:7233",
		APIKey:  "${TEMPORAL_TS_API_KEY_BETA}",
	}
	err := cfg.CloudAPIKeyError()
	if err == nil || !strings.Contains(err.Error(), "TEMPORAL_TS_API_KEY_BETA") {
		t.Fatalf("want unset env named, got %v", err)
	}

	t.Setenv("TEMPORAL_TS_API_KEY_BETA", "k")
	if err := cfg.CloudAPIKeyError(); err != nil {
		t.Fatalf("set key should pass: %v", err)
	}

	local := ConnectionConfig{Address: "localhost:7233"}
	if err := local.CloudAPIKeyError(); err != nil {
		t.Fatalf("local should not require a cloud key: %v", err)
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
	want := "https://cloud.temporal.io/namespaces/orders.abc12/workflows/order%2F123/run-1/timeline"
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

func TestResolveWorkflowColumnsKeepsCustomSearchAttributes(t *testing.T) {
	got := ResolveWorkflowColumns([]WorkflowColumnConfig{
		{ID: "workflow_id", Width: 36},
		{ID: "SA:CustomerId", Width: 12},
		{ID: "sa:CustomerId", Width: 8},
		{ID: "nope", Width: 8},
	})
	if len(got) != 2 {
		t.Fatalf("columns: %+v", got)
	}
	if got[1].ID != SearchAttributeColumnID("CustomerId") || got[1].Width != 12 {
		t.Fatalf("search attribute column: %+v", got[1])
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

func TestShouldShowScrollbars(t *testing.T) {
	if !DefaultConfig().ShouldShowScrollbars() {
		t.Fatal("show_scrollbars should default to on")
	}
	off := false
	if (&Config{ShowScrollbars: &off}).ShouldShowScrollbars() {
		t.Fatal("show_scrollbars: false should hide scrollbars")
	}
	on := true
	if !(&Config{ShowScrollbars: &on}).ShouldShowScrollbars() {
		t.Fatal("show_scrollbars: true should show scrollbars")
	}
	if !((*Config)(nil)).ShouldShowScrollbars() {
		t.Fatal("nil config should default to on")
	}
	parsed, err := ParseConfigFile([]byte("show_scrollbars: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ShouldShowScrollbars() {
		t.Fatal("yaml show_scrollbars: false should hide scrollbars")
	}
}

func TestShouldWrapFilters(t *testing.T) {
	if DefaultConfig().ShouldWrapFilters() {
		t.Fatal("filter_wrap should default to off")
	}
	on := true
	if !(&Config{FilterWrap: &on}).ShouldWrapFilters() {
		t.Fatal("filter_wrap: true should wrap chips")
	}
	off := false
	if (&Config{FilterWrap: &off}).ShouldWrapFilters() {
		t.Fatal("filter_wrap: false should overflow with +N")
	}
	if ((*Config)(nil)).ShouldWrapFilters() {
		t.Fatal("nil config should default to off")
	}
	parsed, err := ParseConfigFile([]byte("filter_wrap: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.ShouldWrapFilters() {
		t.Fatal("yaml filter_wrap: true should wrap chips")
	}
}

func TestResolvedSavedFiltersPosition(t *testing.T) {
	if DefaultConfig().ResolvedSavedFiltersPosition() != SavedFiltersPositionTop {
		t.Fatal("saved_filters_position should default to top")
	}
	if (*Config)(nil).ResolvedSavedFiltersPosition() != SavedFiltersPositionTop {
		t.Fatal("nil config should default to top")
	}
	if (&Config{SavedFiltersPosition: "side"}).ResolvedSavedFiltersPosition() != SavedFiltersPositionSide {
		t.Fatal("saved_filters_position: side should use the sidebar")
	}
	if (&Config{SavedFiltersPosition: "TOP"}).ResolvedSavedFiltersPosition() != SavedFiltersPositionTop {
		t.Fatal("saved_filters_position: TOP should use the top bar")
	}
	parsed, err := ParseConfigFile([]byte("saved_filters_position: side\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ResolvedSavedFiltersPosition() != SavedFiltersPositionSide {
		t.Fatal("yaml saved_filters_position: side should use the sidebar")
	}
}

func TestResolvedPaneTabs(t *testing.T) {
	if DefaultConfig().ResolvedPrimaryTab() != PrimaryTabWorkflows {
		t.Fatal("primary_tab should default to workflows")
	}
	if DefaultConfig().ResolvedSecondaryTab() != SecondaryTabActivities {
		t.Fatal("secondary_tab should default to activities")
	}
	if DefaultConfig().ResolvedTertiaryTab() != TertiaryTabDetails {
		t.Fatal("tertiary_tab should default to details")
	}
	if (*Config)(nil).ResolvedPrimaryTab() != PrimaryTabWorkflows {
		t.Fatal("nil config should default primary to workflows")
	}
	if (&Config{PrimaryTab: "task_queues"}).ResolvedPrimaryTab() != PrimaryTabQueues {
		t.Fatal("task_queues should resolve to queues")
	}
	if (&Config{PrimaryTab: "SCHEDULES"}).ResolvedPrimaryTab() != PrimaryTabSchedules {
		t.Fatal("SCHEDULES should resolve to schedules")
	}
	if (&Config{SecondaryTab: "hierarchy"}).ResolvedSecondaryTab() != SecondaryTabHierarchy {
		t.Fatal("secondary_tab: hierarchy should stick")
	}
	if (&Config{TertiaryTab: "output"}).ResolvedTertiaryTab() != TertiaryTabOutput {
		t.Fatal("tertiary_tab: output should stick")
	}
	if (&Config{PrimaryTab: "nope"}).ResolvedPrimaryTab() != PrimaryTabWorkflows {
		t.Fatal("unknown primary_tab should fall back to workflows")
	}
	parsed, err := ParseConfigFile([]byte("primary_tab: workers\nsecondary_tab: events\ntertiary_tab: input\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ResolvedPrimaryTab() != PrimaryTabWorkers ||
		parsed.ResolvedSecondaryTab() != SecondaryTabEvents ||
		parsed.ResolvedTertiaryTab() != TertiaryTabInput {
		t.Fatalf("yaml tabs: %s %s %s", parsed.ResolvedPrimaryTab(), parsed.ResolvedSecondaryTab(), parsed.ResolvedTertiaryTab())
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

func TestWorkflowPageLimit(t *testing.T) {
	if DefaultConfig().WorkflowPageLimit() != DefaultWorkflowPageSize {
		t.Fatalf("default page size = %d", DefaultConfig().WorkflowPageLimit())
	}
	zero := 0
	if (&Config{WorkflowPageSize: &zero}).WorkflowPageLimit() != MinWorkflowPageSize {
		t.Fatalf("page size below %d should clamp", MinWorkflowPageSize)
	}
	size := 50
	if (&Config{WorkflowPageSize: &size}).WorkflowPageLimit() != 50 {
		t.Fatal("workflow_page_size should use the configured value")
	}
	huge := 5000
	if (&Config{WorkflowPageSize: &huge}).WorkflowPageLimit() != MaxWorkflowPageSize {
		t.Fatalf("page size should clamp to %d", MaxWorkflowPageSize)
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

func TestShouldAutosaveDefault(t *testing.T) {
	if DefaultConfig().ShouldAutosave() {
		t.Fatal("autosave should default to off")
	}
	off := false
	cfg := &Config{Autosave: &off}
	if cfg.ShouldAutosave() {
		t.Fatal("autosave: false should disable writes")
	}
	on := true
	cfg.Autosave = &on
	if !cfg.ShouldAutosave() {
		t.Fatal("autosave: true should persist in-app changes")
	}
	if (*Config)(nil).ShouldAutosave() {
		t.Fatal("nil config should not autosave")
	}
}

func TestColorCodeFlagsDefaultOff(t *testing.T) {
	if DefaultConfig().ShouldColorCodeActivities() || DefaultConfig().ShouldColorCodeWorkflows() {
		t.Fatal("row color coding should default to off")
	}
	on := true
	cfg := &Config{ColorCodeActivities: &on, ColorCodeWorkflows: &on}
	if !cfg.ShouldColorCodeActivities() || !cfg.ShouldColorCodeWorkflows() {
		t.Fatal("explicit true should enable color coding")
	}
	off := false
	cfg.ColorCodeActivities = &off
	cfg.ColorCodeWorkflows = &off
	if cfg.ShouldColorCodeActivities() || cfg.ShouldColorCodeWorkflows() {
		t.Fatal("explicit false should keep color coding off")
	}
}

func TestResetDefaults(t *testing.T) {
	if DefaultConfig().ResetPointDefault() != ResetPointFirst {
		t.Fatal("reset point should default to first")
	}
	if DefaultConfig().ResetReasonDefault() != DefaultResetReason {
		t.Fatal("reset reason should default to Reset via tempo")
	}
	cfg := &Config{ResetPoint: "last", ResetReason: "  retry failed activity  "}
	if cfg.ResetPointDefault() != ResetPointLast {
		t.Fatal("reset_point: last should select the last point")
	}
	if cfg.ResetReasonDefault() != "retry failed activity" {
		t.Fatalf("reason=%q", cfg.ResetReasonDefault())
	}
	parsed, err := parseConfig([]byte("color_code_workflows: true\nreset_point: last\nreset_reason: because\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.ShouldColorCodeWorkflows() || parsed.ResetPointDefault() != ResetPointLast || parsed.ResetReasonDefault() != "because" {
		t.Fatalf("parsed color=%v point=%q reason=%q", parsed.ShouldColorCodeWorkflows(), parsed.ResetPointDefault(), parsed.ResetReasonDefault())
	}
}

func TestTimeFormatDefaultsRelative(t *testing.T) {
	if DefaultConfig().ResolvedWorkflowTimeFormat() != TimeFormatRelative {
		t.Fatal("workflow times should default to relative")
	}
	if DefaultConfig().ResolvedActivityTimeFormat() != TimeFormatRelative {
		t.Fatal("activity times should default to relative")
	}
	if (*Config)(nil).ResolvedWorkflowTimeFormat() != TimeFormatRelative {
		t.Fatal("nil config should use relative workflow times")
	}
	cfg := &Config{WorkflowTimeFormat: "absolute", ActivityTimeFormat: "ABSOLUTE"}
	if cfg.ResolvedWorkflowTimeFormat() != TimeFormatAbsolute || cfg.ResolvedActivityTimeFormat() != TimeFormatAbsolute {
		t.Fatal("explicit absolute should win")
	}
	parsed, err := parseConfig([]byte("workflow_time_format: absolute\nactivity_time_format: relative\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ResolvedWorkflowTimeFormat() != TimeFormatAbsolute || parsed.ResolvedActivityTimeFormat() != TimeFormatRelative {
		t.Fatalf("parsed workflow=%q activity=%q", parsed.ResolvedWorkflowTimeFormat(), parsed.ResolvedActivityTimeFormat())
	}
}

func TestDefaultActivityColumnsIncludeEnded(t *testing.T) {
	found := false
	for _, col := range DefaultActivityColumns() {
		if col.ID == ActivityColumnEnded {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default activity columns should include ended")
	}
}

func TestResolveActivityColumnsDefaults(t *testing.T) {
	got := ResolveActivityColumns(nil)
	want := DefaultActivityColumns()
	if !workflowColumnsEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestSetActivityColumnsOmitsDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.SetActivityColumns(DefaultActivityColumns())
	if cfg.ActivityColumns != nil {
		t.Fatalf("expected defaults to be omitted, got %+v", cfg.ActivityColumns)
	}
	cfg.SetActivityColumns([]WorkflowColumnConfig{{ID: ActivityColumnName, Width: 40}})
	if len(cfg.ActivityColumns) != 1 || cfg.ActivityColumns[0].Width != 40 {
		t.Fatalf("got %+v", cfg.ActivityColumns)
	}
}

func TestStatusColumnAllowsWidthOne(t *testing.T) {
	if got := ClampWorkflowColumnWidth(WorkflowColumnStatus, 1); got != 1 {
		t.Fatalf("status width 1 should be kept, got %d", got)
	}
	if got := ClampWorkflowColumnWidth(WorkflowColumnStatus, 0); got != MinWorkflowStatusWidth {
		t.Fatalf("status width 0 should clamp to %d, got %d", MinWorkflowStatusWidth, got)
	}
	if got := ClampWorkflowColumnWidth(WorkflowColumnWorkflowID, 2); got != MinWorkflowColumnWidth {
		t.Fatalf("other columns should still clamp to %d, got %d", MinWorkflowColumnWidth, got)
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

func TestPreviewLoadDelay(t *testing.T) {
	if got := DefaultConfig().PreviewLoadDelay(); got != DefaultPreviewLoadDelay {
		t.Fatalf("default preview load delay = %s", got)
	}
	var nilCfg *Config
	if got := nilCfg.PreviewLoadDelay(); got != DefaultPreviewLoadDelay {
		t.Fatalf("nil config preview load delay = %s", got)
	}

	zero := Setting{text: "0"}
	if got := (&Config{PreviewLoadWait: &zero}).PreviewLoadDelay(); got != 0 {
		t.Fatalf("0 should load immediately, got %s", got)
	}
	ms := Setting{text: "50ms"}
	if got := (&Config{PreviewLoadWait: &ms}).PreviewLoadDelay(); got != 50*time.Millisecond {
		t.Fatalf("configured delay = %s", got)
	}
	for _, bad := range []string{"", "   ", "soon"} {
		value := Setting{text: bad}
		if got := (&Config{PreviewLoadWait: &value}).PreviewLoadDelay(); got != DefaultPreviewLoadDelay {
			t.Fatalf("%q should fall back, got %s", bad, got)
		}
	}
	neg, huge := Setting{text: "-1s"}, Setting{text: "1h"}
	if got := (&Config{PreviewLoadWait: &neg}).PreviewLoadDelay(); got != 0 {
		t.Fatalf("negative delay should be 0, got %s", got)
	}
	if got := (&Config{PreviewLoadWait: &huge}).PreviewLoadDelay(); got != MaxPreviewLoadDelay {
		t.Fatalf("huge delay should clamp, got %s", got)
	}

	cfg, err := parseConfig([]byte("preview_load_delay: 50ms\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.PreviewLoadDelay(); got != 50*time.Millisecond {
		t.Fatalf("yaml preview_load_delay = %s", got)
	}
	cfg, err = parseConfig([]byte("preview_load_delay: 0\n"))
	if err != nil {
		t.Fatalf("parse zero: %v", err)
	}
	if got := cfg.PreviewLoadDelay(); got != 0 {
		t.Fatalf("yaml 0 preview_load_delay = %s", got)
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

func TestEnsureSavedFiltersSeedsOnlyWhenUnset(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SavedFilters != nil {
		t.Fatal("DefaultConfig should leave saved filters unset")
	}
	cfg.EnsureSavedFilters()
	if len(cfg.GetSavedFilters()) == 0 {
		t.Fatal("empty config should get the built-in filters")
	}
	n := len(cfg.SavedFilters)
	cfg.EnsureSavedFilters()
	if len(cfg.SavedFilters) != n {
		t.Fatal("seeding twice should not duplicate filters")
	}

	owned := &Config{SavedFilters: []SavedFilter{}}
	owned.EnsureSavedFilters()
	if owned.SavedFilters == nil || len(owned.SavedFilters) != 0 {
		t.Fatal("an explicit empty list should stay empty")
	}
}

func TestRenameFilterKeepsQueryDefaultAndOrder(t *testing.T) {
	cfg := &Config{SavedFilters: []SavedFilter{
		{Name: "first", Query: "WorkflowType = 'A'"},
		{Name: "recent", Query: "StartTime > '$HOURS_AGO_24'", IsDefault: true},
		{Name: "last", Query: "ExecutionStatus = 'Failed'"},
	}}
	if err := cfg.RenameFilter("recent", "today"); err != nil {
		t.Fatal(err)
	}
	got := cfg.SavedFilters[1]
	if got.Name != "today" || got.Query != "StartTime > '$HOURS_AGO_24'" || !got.IsDefault {
		t.Fatalf("rename should keep query, default, and position, got %+v", cfg.SavedFilters)
	}
	if cfg.SavedFilters[0].Name != "first" || cfg.SavedFilters[2].Name != "last" {
		t.Fatalf("neighbors should stay put, got %+v", cfg.SavedFilters)
	}
}

func TestRenameFilterSameNameIsNoop(t *testing.T) {
	cfg := &Config{SavedFilters: []SavedFilter{{Name: "recent", Query: "q"}}}
	if err := cfg.RenameFilter("recent", "recent"); err != nil {
		t.Fatal(err)
	}
	if cfg.SavedFilters[0].Name != "recent" || cfg.SavedFilters[0].Query != "q" {
		t.Fatalf("same-name rename should leave the filter alone, got %+v", cfg.SavedFilters)
	}
}

func TestRenameFilterAllowsChangingCase(t *testing.T) {
	cfg := &Config{SavedFilters: []SavedFilter{{Name: "Recent", Query: "q", IsDefault: true}}}
	if err := cfg.RenameFilter("Recent", "recent"); err != nil {
		t.Fatal(err)
	}
	if cfg.SavedFilters[0].Name != "recent" || !cfg.SavedFilters[0].IsDefault {
		t.Fatalf("case-only rename should stay on the same filter, got %+v", cfg.SavedFilters)
	}
}

func TestRenameFilterRefusesCollisionAndMissing(t *testing.T) {
	cfg := &Config{SavedFilters: []SavedFilter{{Name: "a"}, {Name: "b"}}}
	if err := cfg.RenameFilter("a", "b"); err == nil {
		t.Fatal("renaming onto an existing name should fail")
	}
	if cfg.SavedFilters[0].Name != "a" || cfg.SavedFilters[1].Name != "b" {
		t.Fatalf("a refused rename should not change anything, got %+v", cfg.SavedFilters)
	}
	if err := cfg.RenameFilter("missing", "c"); err == nil {
		t.Fatal("renaming a missing filter should fail")
	}
	if err := cfg.RenameFilter("a", "  "); err == nil {
		t.Fatal("an empty name should fail")
	}
}

func TestMoveSavedFilterReorders(t *testing.T) {
	cfg := &Config{SavedFilters: []SavedFilter{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	cfg.MoveSavedFilter(2, 0)
	if cfg.SavedFilters[0].Name != "c" || cfg.SavedFilters[1].Name != "a" || cfg.SavedFilters[2].Name != "b" {
		t.Fatalf("got %+v", cfg.SavedFilters)
	}
	cfg.MoveSavedFilter(0, 2)
	if cfg.SavedFilters[0].Name != "a" || cfg.SavedFilters[2].Name != "c" {
		t.Fatalf("move down %+v", cfg.SavedFilters)
	}
}

func TestParseConfigSeedsSavedFiltersWhenOmitted(t *testing.T) {
	cfg, err := ParseConfigFile([]byte("theme: nord\nactive_profile: default\nprofiles:\n  default:\n    address: localhost:7233\n    namespace: default\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.GetSavedFilters()) == 0 {
		t.Fatal("omitted saved_filters should seed defaults")
	}
}
