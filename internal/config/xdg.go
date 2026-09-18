package config

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the configuration directory path following XDG spec.
// Priority: $XDG_CONFIG_HOME/tempo > ~/.config/tempo > ~/.tempo
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tempo")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".tempo"
	}

	// Prefer ~/.config/tempo if .config exists
	configDir := filepath.Join(home, ".config")
	if info, err := os.Stat(configDir); err == nil && info.IsDir() {
		return filepath.Join(configDir, "tempo")
	}

	return filepath.Join(home, ".tempo")
}

// ConfigPath returns the full path to the config file.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// StateDir returns the directory for session state such as command history.
// Priority: $XDG_STATE_HOME/tempo > ~/.local/state/tempo
func StateDir() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "tempo")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".tempo"
	}
	return filepath.Join(home, ".local", "state", "tempo")
}

// HistoryPath returns the command history file path.
func HistoryPath() string {
	return filepath.Join(StateDir(), "history")
}

// SearchHistoryPath returns the / search history file path.
func SearchHistoryPath() string {
	return filepath.Join(StateDir(), "search_history")
}

// ThemesDir returns the directory for custom themes.
func ThemesDir() string {
	return filepath.Join(ConfigDir(), "themes")
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	dir := ConfigDir()
	return os.MkdirAll(dir, 0755)
}

// EnsureThemesDir creates the themes directory if it doesn't exist.
func EnsureThemesDir() error {
	dir := ThemesDir()
	return os.MkdirAll(dir, 0755)
}
