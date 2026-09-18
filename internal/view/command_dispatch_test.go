package view

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/rivo/tview"
)

func TestCommitSettingStaysInMemoryUnlessAutosave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	a := &App{config: cfg}
	s, ok := lookupTempoSetting("color_code_workflows")
	if !ok {
		t.Fatal("missing color_code_workflows")
	}
	a.commitSetting(s, "on")
	if !cfg.ShouldColorCodeWorkflows() {
		t.Fatal("in-memory value should change")
	}
	if _, err := os.Stat(config.ConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("autosave off should not write %s: %v", config.ConfigPath(), err)
	}

	on := true
	cfg.Autosave = &on
	a.commitSetting(s, "off")
	data, err := os.ReadFile(config.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "color_code_workflows") {
		t.Fatalf("autosave should persist, got %s", got)
	}
}

func TestConfigSetWithoutValueOpensPicker(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	a.executeBuiltinCommand([]string{"config", "set", "color_code_workflows"})
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("config set without a value should open a picker")
	}

	a.app.Pages().DismissModal()
	a.executeBuiltinCommand([]string{"config", "set", "theme"})
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("config set theme should open the theme selector")
	}
}

func TestBuiltinCommandsInvokeViewActions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowListWithData(a, "default", []temporal.Workflow{{ID: "wf-1", RunID: "run-1"}})
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	a.executeBuiltinCommand([]string{"tab", "schedules"})
	if !wl.schedulesActive() {
		t.Fatal("tab schedules should switch the primary tab")
	}

	a.executeBuiltinCommand([]string{"tab", "workflows"})
	if !wl.workflowsActive() {
		t.Fatal("tab workflows should restore the workflow list")
	}

	a.executeBuiltinCommand([]string{"preview", "on"})
	if !wl.previewMode {
		t.Fatal("preview on should show preview")
	}
	a.executeBuiltinCommand([]string{"preview", "off"})
	if wl.previewMode {
		t.Fatal("preview off should hide preview")
	}

	a.executeBuiltinCommand([]string{"workflow", "start"})
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("workflow start should open the start modal")
	}
}

func TestBuiltinCommandsBeatUserCommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Commands = map[string]config.CommandConfig{
		"config": {Cmd: "echo stolen", Description: "stolen"},
	}
	a := &App{config: cfg}
	if !a.executeBuiltinCommand([]string{"config", "get", "theme"}) {
		t.Fatal("builtin config should win")
	}
	if a.statusText != "theme="+config.DefaultTheme {
		t.Fatalf("status=%q", a.statusText)
	}
}

func TestPersistConfigWritesExplicitSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	a := &App{config: cfg}
	s, _ := lookupTempoSetting("autosave")
	a.commitSetting(s, "on")
	if err := a.persistConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Clean(config.ConfigPath())); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSetPickerPreviewUsesDataRow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	s, ok := lookupTempoSetting("modal_shadow")
	if !ok {
		t.Fatal("missing modal_shadow")
	}
	a.showConfigValuePicker(s)
	table := modalContentTable(a.app.Pages().Current())
	if table == nil {
		t.Fatal("expected a table in the picker")
	}

	table.SelectRow(1)
	if got := a.config.ResolvedModalShadow(); got != config.ModalShadowNone {
		t.Fatalf("highlighting none previewed %q", got)
	}
	table.SelectRow(2)
	if got := a.config.ResolvedModalShadow(); got != config.ModalShadowUniform {
		t.Fatalf("highlighting uniform previewed %q", got)
	}
	table.SelectRow(0)
	if got := a.config.ResolvedModalShadow(); got != config.ModalShadowDirectional {
		t.Fatalf("highlighting directional previewed %q", got)
	}
}

func modalContentTable(p tview.Primitive) *components.Table {
	g, ok := p.(interface{ GetPanel() *components.Panel })
	if !ok || g.GetPanel() == nil {
		return nil
	}
	table, _ := g.GetPanel().GetContent().(*components.Table)
	return table
}
