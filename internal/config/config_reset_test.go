package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseKeepsOnlyWhatTheFileSays is the config-reset bug: parsing merged the
// file onto DefaultConfig, so the built-in "default" profile came back every
// time someone removed it, and got written out again on the next save.
func TestParseKeepsOnlyWhatTheFileSays(t *testing.T) {
	cfg, err := parseConfig([]byte(`
theme: kanagawa
active_profile: prod
profiles:
    prod:
        address: temporal.prod:7233
        namespace: orders
`))
	if err != nil {
		t.Fatal(err)
	}
	if _, resurrected := cfg.Profiles["default"]; resurrected {
		t.Fatalf("a profile the file does not list should stay gone: %+v", cfg.Profiles)
	}
	if len(cfg.Profiles) != 1 || cfg.ActiveProfile != "prod" {
		t.Fatalf("profiles=%+v active=%q", cfg.Profiles, cfg.ActiveProfile)
	}
	if cfg.Theme != "kanagawa" {
		t.Fatalf("theme=%q", cfg.Theme)
	}

	// Settings the file does not mention keep their defaults without being
	// written back as explicit values.
	if cfg.WorkflowColumns != nil {
		t.Fatal("an unset column layout should stay unset")
	}
	out, err := cfg.marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "default:") {
		t.Fatalf("saving should not reintroduce the default profile:\n%s", out)
	}
}

// An empty or brand new file still needs something to connect to.
func TestParseEmptyFileFallsBackToDefaults(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("an empty config should get the default profile: %+v", cfg.Profiles)
	}
	if cfg.Theme != DefaultTheme || cfg.ActiveProfile != "default" {
		t.Fatalf("theme=%q active=%q", cfg.Theme, cfg.ActiveProfile)
	}
}

func TestLoadRejectsUnparseableFileInsteadOfReturningDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "tempo"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tempo", "config.yaml")
	if err := os.WriteFile(path, []byte("theme: [this is not a config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err == nil {
		t.Fatal("a broken file should be reported, not silently replaced by defaults")
	}
	if cfg != nil {
		t.Fatal("no config should be returned for a broken file")
	}
	// The bad file is still there to be fixed.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file should be left alone: %v", err)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := DefaultConfig()
	cfg.Theme = "kanagawa"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tempo", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "theme: kanagawa") {
		t.Fatalf("config not written:\n%s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Fatalf("permissions = %v", perm)
	}

	// Nothing is left behind for the watcher to trip over.
	entries, err := os.ReadDir(filepath.Join(dir, "tempo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".config-") {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}

	// A reparse of what we wrote gives back the same settings.
	reparsed, err := parseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if reparsed.Theme != "kanagawa" {
		t.Fatalf("round trip lost the theme: %q", reparsed.Theme)
	}
}
