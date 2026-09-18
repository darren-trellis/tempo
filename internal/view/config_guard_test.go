package view

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
)

// TestSaveConfigRefusesToOverwriteAnUnreadableFile is the other half of the
// config-reset bug: when the file could not be parsed the app ran on defaults,
// and the next save wrote those defaults over the user's settings.
func TestSaveConfigRefusesToOverwriteAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "tempo"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tempo", "config.yaml")
	broken := "theme: [not a config\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "default")
	on := true
	a.config.Autosave = &on
	a.MarkConfigUnreadable()
	if err := a.SaveConfig(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != broken {
		t.Fatalf("the unreadable file should be left alone, got:\n%s", data)
	}

	// Applying a theme must not write defaults over it either.
	a.applyTheme("kanagawa")
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != broken {
		t.Fatalf("choosing a theme should not rewrite an unreadable config, got:\n%s", data)
	}
	if a.config.Theme != "kanagawa" {
		t.Fatal("the theme should still apply for this session")
	}
}

func TestApplyThemeDoesNotWriteWhenAutosaveIsOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "tempo"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tempo", "config.yaml")
	original := "theme: nord\nactive_profile: default\nprofiles:\n    default:\n        address: localhost:7233\n        namespace: default\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "default")
	a.applyTheme("kanagawa")
	if a.config.Theme != "kanagawa" {
		t.Fatal("the theme should still apply for this session")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("autosave off should leave the file alone, got:\n%s", data)
	}
}

func TestSaveConfigNoopsWhenAutosaveIsOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "default")
	if err := a.SaveConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tempo", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("autosave off should not create a config file, stat err = %v", err)
	}
}

func TestSaveConfigWritesWhenTheFileIsFine(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := config.DefaultConfig()
	on := true
	cfg.Autosave = &on
	cfg.Profiles = map[string]config.ConnectionConfig{
		"prod": {Address: "temporal.prod:7233", Namespace: "orders"},
	}
	cfg.ActiveProfile = "prod"
	a := NewAppWithProvider(nil, "orders", cfg, "prod")

	if err := a.SaveConfig(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "tempo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// What comes back is what the user had, with no default profile bolted on.
	reloaded, err := config.ParseConfigFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, resurrected := reloaded.Profiles["default"]; resurrected {
		t.Fatalf("saving should not add a default profile back: %+v", reloaded.Profiles)
	}
	if reloaded.ActiveProfile != "prod" || len(reloaded.Profiles) != 1 {
		t.Fatalf("profiles=%+v active=%q", reloaded.Profiles, reloaded.ActiveProfile)
	}
}
