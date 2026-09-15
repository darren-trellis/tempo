package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// TLSConfig holds TLS connection settings.
type TLSConfig struct {
	Cert       string `yaml:"cert,omitempty"`
	Key        string `yaml:"key,omitempty"`
	CA         string `yaml:"ca,omitempty"`
	ServerName string `yaml:"server_name,omitempty"`
	SkipVerify bool   `yaml:"skip_verify,omitempty"`
}

// CommandOutputType defines how command output should be displayed.
type CommandOutputType string

const (
	OutputLog       CommandOutputType = "log"
	OutputJSON      CommandOutputType = "json"
	OutputWorkflows CommandOutputType = "workflows"
	OutputWorkflow  CommandOutputType = "workflow"
)

// CommandConfig defines a user-configured command.
type CommandConfig struct {
	Description string            `yaml:"description,omitempty"`
	Cmd         string            `yaml:"cmd"`
	Output      CommandOutputType `yaml:"output,omitempty"`
	Confirm     bool              `yaml:"confirm,omitempty"`
}

// ConnectionConfig holds Temporal connection settings.
type ConnectionConfig struct {
	Address       string                   `yaml:"address"`
	Namespace     string                   `yaml:"namespace"`
	TLS           TLSConfig                `yaml:"tls,omitempty"`
	APIKey        string                   `yaml:"api_key,omitempty"`        // For Temporal Cloud API key authentication
	GRPCMeta      map[string]string        `yaml:"grpc_meta,omitempty"`      // Custom gRPC metadata headers (KEY=VALUE pairs)
	CodecEndpoint string                   `yaml:"codec_endpoint,omitempty"` // Temporal codec server base URL
	UIURL         string                   `yaml:"ui_url,omitempty"`         // Temporal Web UI base URL
	Commands      map[string]CommandConfig `yaml:"commands,omitempty"`
}

// ExpandEnv expands environment variables in sensitive fields.
// Supports ${VAR}, $VAR, and ${VAR:-default} syntax.
func (c ConnectionConfig) ExpandEnv() ConnectionConfig {
	expanded := ConnectionConfig{
		Address:       c.Address,
		Namespace:     c.Namespace,
		TLS:           c.TLS,
		APIKey:        expandEnvVar(c.APIKey),
		CodecEndpoint: expandEnvVar(c.CodecEndpoint),
		UIURL:         expandEnvVar(c.UIURL),
		Commands:      c.Commands,
	}
	if len(c.GRPCMeta) > 0 {
		expanded.GRPCMeta = make(map[string]string, len(c.GRPCMeta))
		for k, v := range c.GRPCMeta {
			expanded.GRPCMeta[k] = expandEnvVar(v)
		}
	}
	return expanded
}

// expandEnvVar expands environment variable references in a string.
// Uses os.ExpandEnv which supports $VAR and ${VAR} syntax.
func expandEnvVar(s string) string {
	return os.ExpandEnv(s)
}

func IsTemporalCloudAddress(addr string) bool {
	host, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(addr)), ":")
	return strings.HasSuffix(host, ".temporal.io") || strings.HasSuffix(host, ".tmprl.cloud")
}

func EnvRefName(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}"):
		inner := strings.TrimSuffix(strings.TrimPrefix(s, "${"), "}")
		if i := strings.IndexAny(inner, ":"); i >= 0 {
			inner = inner[:i]
		}
		return strings.TrimSpace(strings.TrimRight(inner, "-"))
	case strings.HasPrefix(s, "$"):
		return strings.TrimSpace(strings.TrimPrefix(s, "$"))
	default:
		return ""
	}
}

func (c ConnectionConfig) CloudAPIKeyError() error {
	if !IsTemporalCloudAddress(c.Address) {
		return nil
	}
	if expandEnvVar(c.APIKey) != "" {
		return nil
	}
	if name := EnvRefName(c.APIKey); name != "" {
		return fmt.Errorf("Temporal Cloud requires an API key; %s is not set", name)
	}
	return fmt.Errorf("Temporal Cloud requires an API key")
}

// ToTemporalConfig converts config.ConnectionConfig to temporal-compatible format.
// Returns address, namespace, TLS fields, and API key as separate values.
func (c ConnectionConfig) ToTemporalConfig() (address, namespace, tlsCert, tlsKey, tlsCA, tlsServerName string, tlsSkipVerify bool, apiKey string) {
	return c.Address, c.Namespace, c.TLS.Cert, c.TLS.Key, c.TLS.CA, c.TLS.ServerName, c.TLS.SkipVerify, c.APIKey
}

// FromTemporalConfig creates a ConnectionConfig from temporal-style flat fields.
func FromTemporalConfig(address, namespace, tlsCert, tlsKey, tlsCA, tlsServerName string, tlsSkipVerify bool, apiKey string) ConnectionConfig {
	return ConnectionConfig{
		Address:   address,
		Namespace: namespace,
		TLS: TLSConfig{
			Cert:       tlsCert,
			Key:        tlsKey,
			CA:         tlsCA,
			ServerName: tlsServerName,
			SkipVerify: tlsSkipVerify,
		},
		APIKey: apiKey,
	}
}

// SavedFilter represents a saved visibility query.
type SavedFilter struct {
	Name      string `yaml:"name"`
	Query     string `yaml:"query"`
	IsDefault bool   `yaml:"is_default,omitempty"`
}

// ExternalProfilePrefix is the prefix used for profiles imported from the Temporal CLI.
const ExternalProfilePrefix = "import:"

// Config represents the application configuration.
type Config struct {
	Theme            string                      `yaml:"theme"`
	ActiveProfile    string                      `yaml:"active_profile,omitempty"`
	Profiles         map[string]ConnectionConfig `yaml:"profiles,omitempty"`
	ExternalProfiles map[string]ConnectionConfig `yaml:"-"`
	SavedFilters     []SavedFilter               `yaml:"saved_filters,omitempty"`
	CheckUpdates     *bool                       `yaml:"check_updates,omitempty"`
	Autoreload       *bool                       `yaml:"autoreload,omitempty"`
	HelpStyle        string                      `yaml:"help_style,omitempty"` // "modal" (default) or "sheet"
	Commands         map[string]CommandConfig    `yaml:"commands,omitempty"`
	WorkflowColumns  []WorkflowColumnConfig      `yaml:"workflow_columns,omitempty"`
	PreviewCacheSize *int                        `yaml:"preview_cache_size,omitempty"`
	MouseScrollStep  *int                        `yaml:"mouse_scroll_step,omitempty"`
	ShowScrollbars   *bool                       `yaml:"show_scrollbars,omitempty"`
	WorkflowPageSize *int                        `yaml:"workflow_page_size,omitempty"`
	// How long a worker may go unseen before the workers tab calls it stale,
	// written as a duration such as "45s" or "2m", or as a plain number of
	// seconds.
	WorkerPollQuiet      *Setting `yaml:"worker_poll_quiet_after,omitempty"`
	WorkerHeartbeatQuiet *Setting `yaml:"worker_heartbeat_quiet_after,omitempty"`
	// How often auto-refresh reloads the current list, written as a duration
	// such as "1s" or "500ms", or as a plain number of seconds.
	RefreshInterval *Setting `yaml:"refresh_rate,omitempty"`
	// How long preview waits after a new workflow is highlighted before
	// fetching history, written as a duration such as "200ms" or "0".
	PreviewLoadWait *Setting `yaml:"preview_load_delay,omitempty"`
}

// Setting is a duration that tolerates how people actually write one: "45s",
// "2m30s", or a bare number of seconds. A single mistyped value must never make
// the whole config unparseable, so anything else is kept verbatim and reported
// as unusable rather than failing the load.
type Setting struct {
	text string
}

func (s *Setting) UnmarshalYAML(value *yaml.Node) error {
	var text string
	if err := value.Decode(&text); err != nil {
		var seconds float64
		if err := value.Decode(&seconds); err != nil {
			// Keep the raw scalar; the accessor falls back to its default.
			s.text = value.Value
			return nil
		}
		s.text = strconv.FormatFloat(seconds, 'f', -1, 64) + "s"
		return nil
	}
	s.text = text
	return nil
}

func (s Setting) MarshalYAML() (interface{}, error) {
	return s.text, nil
}

// Duration parses the setting, returning ok=false when it is not a usable
// duration.
func (s *Setting) Duration() (time.Duration, bool) {
	if s == nil {
		return 0, false
	}
	value := strings.TrimSpace(s.text)
	if value == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d, true
	}
	// A bare number means seconds.
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		return time.Duration(seconds * float64(time.Second)), true
	}
	return 0, false
}

// IsExternalProfile returns true if the given profile name is an external
// profile imported from the Temporal CLI.
func (c *Config) IsExternalProfile(name string) bool {
	if c.ExternalProfiles == nil {
		return false
	}
	_, ok := c.ExternalProfiles[name]
	return ok
}

// GetHelpStyle returns the configured help display style.
// Returns "sheet" if explicitly set, otherwise "modal" (default).
func (c *Config) GetHelpStyle() string {
	if c.HelpStyle == "sheet" {
		return "sheet"
	}
	return "modal"
}

// ShouldCheckUpdates returns whether update checking is enabled.
// Defaults to true if not explicitly set.
func (c *Config) ShouldCheckUpdates() bool {
	if c.CheckUpdates == nil {
		return true
	}
	return *c.CheckUpdates
}

// ShouldAutoreload returns whether config file changes should be applied live.
// Defaults to true if not explicitly set.
func (c *Config) ShouldAutoreload() bool {
	if c == nil || c.Autoreload == nil {
		return true
	}
	return *c.Autoreload
}

const (
	DefaultPreviewCacheSize = 32
	MaxPreviewCacheSize     = 256
	DefaultMouseScrollStep  = 1
	MaxMouseScrollStep      = 40
	DefaultWorkflowPageSize = 100
	MinWorkflowPageSize     = 10
	MaxWorkflowPageSize     = 1000
)

// PreviewCacheLimit is how many workflow histories preview mode keeps in memory.
// Defaults to 32. Set preview_cache_size to 0 to disable caching.
func (c *Config) PreviewCacheLimit() int {
	if c == nil || c.PreviewCacheSize == nil {
		return DefaultPreviewCacheSize
	}
	n := *c.PreviewCacheSize
	if n < 0 {
		return 0
	}
	if n > MaxPreviewCacheSize {
		return MaxPreviewCacheSize
	}
	return n
}

// ShouldShowScrollbars returns whether tables and text views draw scrollbars.
// Defaults to true if not explicitly set.
func (c *Config) ShouldShowScrollbars() bool {
	if c == nil || c.ShowScrollbars == nil {
		return true
	}
	return *c.ShowScrollbars
}

func (c *Config) MouseScrollStepSize() int {
	if c == nil || c.MouseScrollStep == nil {
		return DefaultMouseScrollStep
	}
	n := *c.MouseScrollStep
	if n < DefaultMouseScrollStep {
		return DefaultMouseScrollStep
	}
	if n > MaxMouseScrollStep {
		return MaxMouseScrollStep
	}
	return n
}

// WorkflowPageLimit is how many workflows each list page fetches.
// Defaults to 100. Set workflow_page_size in config.yaml.
func (c *Config) WorkflowPageLimit() int {
	if c == nil || c.WorkflowPageSize == nil {
		return DefaultWorkflowPageSize
	}
	n := *c.WorkflowPageSize
	if n < MinWorkflowPageSize {
		return MinWorkflowPageSize
	}
	if n > MaxWorkflowPageSize {
		return MaxWorkflowPageSize
	}
	return n
}

const (
	DefaultRefreshRate = time.Second
	MinRefreshRate     = 250 * time.Millisecond
	MaxRefreshRate     = 5 * time.Minute
)

const (
	DefaultPreviewLoadDelay = 200 * time.Millisecond
	MaxPreviewLoadDelay     = 2 * time.Second
)

// PreviewLoadDelay is how long preview waits after a new workflow is
// highlighted before fetching history. Set preview_load_delay to a duration
// such as "200ms" or "0" to load immediately.
func (c *Config) PreviewLoadDelay() time.Duration {
	if c == nil {
		return DefaultPreviewLoadDelay
	}
	d, ok := c.PreviewLoadWait.Duration()
	if !ok {
		return DefaultPreviewLoadDelay
	}
	if d < 0 {
		return 0
	}
	if d > MaxPreviewLoadDelay {
		return MaxPreviewLoadDelay
	}
	return d
}

// RefreshRate is how often auto-refresh reloads the current list. Set
// refresh_rate to a duration such as "1s" or "500ms".
func (c *Config) RefreshRate() time.Duration {
	if c == nil {
		return DefaultRefreshRate
	}
	d, ok := c.RefreshInterval.Duration()
	if !ok || d <= 0 {
		return DefaultRefreshRate
	}
	if d < MinRefreshRate {
		return MinRefreshRate
	}
	if d > MaxRefreshRate {
		return MaxRefreshRate
	}
	return d
}

const (
	// DefaultWorkerPollQuietAfter is how long a task queue poll registry entry may
	// go unchanged before its worker reads as stale. A live worker refreshes the
	// entry every long poll, so this only has to clear one poll cycle.
	DefaultWorkerPollQuietAfter = 90 * time.Second
	// DefaultWorkerHeartbeatQuietAfter is the same allowance for worker
	// heartbeats, which arrive about once a minute.
	DefaultWorkerHeartbeatQuietAfter = 3 * time.Minute
	MinWorkerQuietAfter              = 5 * time.Second
	MaxWorkerQuietAfter              = time.Hour
)

// WorkerPollQuietAfter is how long a task queue poll registry entry may go
// unchanged before the workers tab treats the instance as gone. Set
// worker_poll_quiet_after to a duration such as "45s".
func (c *Config) WorkerPollQuietAfter() time.Duration {
	if c == nil {
		return DefaultWorkerPollQuietAfter
	}
	return workerQuietWindow(c.WorkerPollQuiet, DefaultWorkerPollQuietAfter)
}

// WorkerHeartbeatQuietAfter is the same allowance for a worker heartbeat, used
// when nothing is polling under that identity. Set worker_heartbeat_quiet_after.
func (c *Config) WorkerHeartbeatQuietAfter() time.Duration {
	if c == nil {
		return DefaultWorkerHeartbeatQuietAfter
	}
	return workerQuietWindow(c.WorkerHeartbeatQuiet, DefaultWorkerHeartbeatQuietAfter)
}

// workerQuietWindow reads a configured duration, falling back to the default
// when it is missing or unusable, and clamps it to a sane range.
func workerQuietWindow(raw *Setting, fallback time.Duration) time.Duration {
	d, ok := raw.Duration()
	if !ok || d <= 0 {
		return fallback
	}
	if d < MinWorkerQuietAfter {
		return MinWorkerQuietAfter
	}
	if d > MaxWorkerQuietAfter {
		return MaxWorkerQuietAfter
	}
	return d
}

// DefaultConfig returns a config with default values.
func DefaultConfig() *Config {
	return &Config{
		Theme:         DefaultTheme,
		ActiveProfile: "default",
		Profiles: map[string]ConnectionConfig{
			"default": {
				Address:   "localhost:7233",
				Namespace: "default",
			},
		},
	}
}

// Load reads the config file from disk.
// Returns default config if file doesn't exist.
func Load() (*Config, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.loadExternalProfiles()
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg, err := parseConfig(data)
	if err != nil {
		return nil, err
	}
	noteFileHash(data)
	return cfg, nil
}

// loadExternalProfiles discovers Temporal CLI profiles and stores them
// with the "import:" prefix to distinguish from native profiles.
func (c *Config) loadExternalProfiles() {
	cliProfiles := LoadTemporalCLIProfiles()
	if len(cliProfiles) == 0 {
		return
	}
	c.ExternalProfiles = make(map[string]ConnectionConfig, len(cliProfiles))
	for name, cfg := range cliProfiles {
		c.ExternalProfiles[ExternalProfilePrefix+name] = cfg
	}
}

// ensureDefaults ensures the config has valid profiles and active profile.
func (c *Config) ensureDefaults() {
	if c.Theme == "" {
		c.Theme = DefaultTheme
	}
	if len(c.Profiles) == 0 {
		c.Profiles = map[string]ConnectionConfig{
			"default": {
				Address:   "localhost:7233",
				Namespace: "default",
			},
		}
		c.ActiveProfile = "default"
	}

	// Ensure ActiveProfile is set and valid
	if c.ActiveProfile == "" {
		for name := range c.Profiles {
			c.ActiveProfile = name
			break
		}
	} else if _, ok := c.Profiles[c.ActiveProfile]; !ok {
		// Active profile doesn't exist, use first available
		for name := range c.Profiles {
			c.ActiveProfile = name
			break
		}
	}
}

// marshal renders the config as it would be written to disk.
func (c *Config) marshal() ([]byte, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	return data, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	if err := EnsureConfigDir(); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := c.marshal()
	if err != nil {
		return err
	}

	path := ConfigPath()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("writing config: %w", err)
	}
	noteFileHash(data)

	return nil
}

// LoadTheme loads a theme by name or path.
// If name matches a built-in theme, returns that.
// Otherwise, attempts to load from custom themes directory or absolute path.
func LoadTheme(name string) (*ParsedTheme, error) {
	// Check built-in themes first
	if theme, ok := BuiltinThemes[name]; ok {
		parsed, err := theme.Parse()
		if err != nil {
			return nil, err
		}
		parsed.Key = name
		return parsed, nil
	}

	// Check custom themes directory
	customPath := filepath.Join(ThemesDir(), name+".yaml")
	if _, err := os.Stat(customPath); err == nil {
		parsed, err := loadThemeFile(customPath)
		if err != nil {
			return nil, err
		}
		parsed.Key = name
		return parsed, nil
	}

	// Try as absolute/relative path
	if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		if _, err := os.Stat(name); err == nil {
			parsed, err := loadThemeFile(name)
			if err != nil {
				return nil, err
			}
			parsed.Key = name
			return parsed, nil
		}
	}

	return nil, fmt.Errorf("theme not found: %s", name)
}

// Save writes the config to disk (standalone function).
func Save(c *Config) error {
	return c.Save()
}

// GetProfile returns a profile by name, checking both native and external profiles.
func (c *Config) GetProfile(name string) (ConnectionConfig, bool) {
	if c.Profiles != nil {
		if profile, ok := c.Profiles[name]; ok {
			return profile, true
		}
	}
	if c.ExternalProfiles != nil {
		if profile, ok := c.ExternalProfiles[name]; ok {
			return profile, true
		}
	}
	return ConnectionConfig{}, false
}

// GetActiveProfile returns the active profile name and its configuration.
func (c *Config) GetActiveProfile() (string, ConnectionConfig) {
	if c.ActiveProfile == "" {
		return "default", ConnectionConfig{
			Address:   "localhost:7233",
			Namespace: "default",
		}
	}
	// Check native profiles
	if c.Profiles != nil {
		if profile, ok := c.Profiles[c.ActiveProfile]; ok {
			return c.ActiveProfile, profile
		}
	}
	// Check external profiles
	if c.ExternalProfiles != nil {
		if profile, ok := c.ExternalProfiles[c.ActiveProfile]; ok {
			return c.ActiveProfile, profile
		}
	}
	// Active profile doesn't exist, return first available native profile
	for name, cfg := range c.Profiles {
		return name, cfg
	}
	return "default", ConnectionConfig{
		Address:   "localhost:7233",
		Namespace: "default",
	}
}

// SetActiveProfile sets the active profile by name.
// Returns error if profile doesn't exist. Supports both native and external profiles.
func (c *Config) SetActiveProfile(name string) error {
	if _, ok := c.GetProfile(name); !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	c.ActiveProfile = name
	return nil
}

// SaveProfile saves or updates a profile.
// Returns error if trying to save an external (imported) profile.
func (c *Config) SaveProfile(name string, cfg ConnectionConfig) error {
	if c.IsExternalProfile(name) {
		return fmt.Errorf("cannot modify external profile %q", name)
	}
	if c.Profiles == nil {
		c.Profiles = make(map[string]ConnectionConfig)
	}
	c.Profiles[name] = cfg
	return nil
}

// DeleteProfile deletes a profile by name.
// Returns error if trying to delete the active profile, an external profile,
// or if profile doesn't exist.
func (c *Config) DeleteProfile(name string) error {
	if c.IsExternalProfile(name) {
		return fmt.Errorf("cannot delete external profile %q", name)
	}
	if c.Profiles == nil {
		return fmt.Errorf("profile %q not found", name)
	}
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	if c.ActiveProfile == name {
		return fmt.Errorf("cannot delete active profile %q", name)
	}
	delete(c.Profiles, name)
	return nil
}

// ListProfiles returns a sorted list of profile names, including external profiles.
func (c *Config) ListProfiles() []string {
	names := make([]string, 0, len(c.Profiles)+len(c.ExternalProfiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	for name := range c.ExternalProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ProfileExists checks if a profile with the given name exists (native or external).
func (c *Config) ProfileExists(name string) bool {
	_, ok := c.GetProfile(name)
	return ok
}

// Saved filter management methods

// GetSavedFilters returns all saved filters.
func (c *Config) GetSavedFilters() []SavedFilter {
	return c.SavedFilters
}

// GetSavedFilter returns a saved filter by name.
func (c *Config) GetSavedFilter(name string) (SavedFilter, bool) {
	for _, f := range c.SavedFilters {
		if f.Name == name {
			return f, true
		}
	}
	return SavedFilter{}, false
}

// SaveFilter adds or updates a saved filter.
func (c *Config) SaveFilter(filter SavedFilter) {
	// Check if filter with same name exists
	for i, f := range c.SavedFilters {
		if f.Name == filter.Name {
			c.SavedFilters[i] = filter
			return
		}
	}
	// Add new filter
	c.SavedFilters = append(c.SavedFilters, filter)
}

// DeleteFilter removes a saved filter by name.
func (c *Config) DeleteFilter(name string) error {
	for i, f := range c.SavedFilters {
		if f.Name == name {
			c.SavedFilters = append(c.SavedFilters[:i], c.SavedFilters[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("filter %q not found", name)
}

// GetDefaultFilter returns the default filter if one is set.
func (c *Config) GetDefaultFilter() (SavedFilter, bool) {
	for _, f := range c.SavedFilters {
		if f.IsDefault {
			return f, true
		}
	}
	return SavedFilter{}, false
}

// SetDefaultFilter sets a filter as the default, clearing any previous default.
func (c *Config) SetDefaultFilter(name string) error {
	found := false
	for i := range c.SavedFilters {
		if c.SavedFilters[i].Name == name {
			c.SavedFilters[i].IsDefault = true
			found = true
		} else {
			c.SavedFilters[i].IsDefault = false
		}
	}
	if !found {
		return fmt.Errorf("filter %q not found", name)
	}
	return nil
}

// ClearDefaultFilter clears the default filter.
func (c *Config) ClearDefaultFilter() {
	for i := range c.SavedFilters {
		c.SavedFilters[i].IsDefault = false
	}
}

// loadThemeFile loads a theme from a YAML file.
func loadThemeFile(path string) (*ParsedTheme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading theme file: %w", err)
	}

	var theme Theme
	if err := yaml.Unmarshal(data, &theme); err != nil {
		return nil, fmt.Errorf("parsing theme file: %w", err)
	}

	return theme.Parse()
}

// GetMergedCommands returns commands merged from global and profile-level config.
// Profile commands override global commands with the same name.
func (c *Config) GetMergedCommands(profileName string) map[string]CommandConfig {
	merged := make(map[string]CommandConfig)
	for name, cmd := range c.Commands {
		merged[name] = cmd
	}
	if profile, ok := c.Profiles[profileName]; ok {
		for name, cmd := range profile.Commands {
			merged[name] = cmd
		}
	}
	return merged
}

// ListCommandNames returns a sorted list of available command names for a profile.
func (c *Config) ListCommandNames(profileName string) []string {
	merged := c.GetMergedCommands(profileName)
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateTheme checks if a theme name is valid.
func ValidateTheme(name string) bool {
	// Built-in theme
	if _, ok := BuiltinThemes[name]; ok {
		return true
	}

	// Custom theme file
	customPath := filepath.Join(ThemesDir(), name+".yaml")
	if _, err := os.Stat(customPath); err == nil {
		return true
	}

	// Absolute path
	if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		if _, err := os.Stat(name); err == nil {
			return true
		}
	}

	return false
}

const (
	WorkflowColumnWorkflowID = "workflow_id"
	WorkflowColumnParentID   = "parent_id"
	WorkflowColumnStatus     = "status"
	WorkflowColumnType       = "type"
	WorkflowColumnStarted    = "started"
	WorkflowColumnEnded      = "ended"
	WorkflowColumnDuration   = "duration"
	WorkflowColumnTaskQueue  = "task_queue"
	WorkflowColumnRunID      = "run_id"

	MinWorkflowColumnWidth = 4
	MaxWorkflowColumnWidth = 200
)

// WorkflowColumnConfig is one column in the workflows table.
// Omit a column to hide it. Width is a max character width.
type WorkflowColumnConfig struct {
	ID    string `yaml:"id"`
	Width int    `yaml:"width,omitempty"`
}

func defaultWorkflowColumns() []WorkflowColumnConfig {
	return []WorkflowColumnConfig{
		{ID: WorkflowColumnWorkflowID, Width: 36},
		{ID: WorkflowColumnParentID, Width: 36},
		{ID: WorkflowColumnStatus, Width: 12},
		{ID: WorkflowColumnType, Width: 24},
		{ID: WorkflowColumnStarted, Width: 11},
		{ID: WorkflowColumnEnded, Width: 11},
		{ID: WorkflowColumnDuration, Width: 12},
		{ID: WorkflowColumnTaskQueue, Width: 20},
		{ID: WorkflowColumnRunID, Width: 36},
	}
}

// DefaultWorkflowColumnWidth returns the built-in width for a column id.
func DefaultWorkflowColumnWidth(id string) int {
	for _, col := range defaultWorkflowColumns() {
		if col.ID == id {
			return col.Width
		}
	}
	return 16
}

func knownWorkflowColumn(id string) bool {
	for _, col := range defaultWorkflowColumns() {
		if col.ID == id {
			return true
		}
	}
	return false
}

// DefaultWorkflowColumns returns the built-in workflows table layout.
func DefaultWorkflowColumns() []WorkflowColumnConfig {
	return defaultWorkflowColumns()
}

// KnownWorkflowColumnIDs returns every supported workflows column id.
func KnownWorkflowColumnIDs() []string {
	cols := defaultWorkflowColumns()
	ids := make([]string, len(cols))
	for i, col := range cols {
		ids[i] = col.ID
	}
	return ids
}

// ClampWorkflowColumnWidth keeps a column width within supported bounds.
func ClampWorkflowColumnWidth(width int) int {
	if width < MinWorkflowColumnWidth {
		return MinWorkflowColumnWidth
	}
	if width > MaxWorkflowColumnWidth {
		return MaxWorkflowColumnWidth
	}
	return width
}

// ResolveWorkflowColumns validates order and widths, filling in defaults.
func ResolveWorkflowColumns(cols []WorkflowColumnConfig) []WorkflowColumnConfig {
	if len(cols) == 0 {
		return defaultWorkflowColumns()
	}

	seen := make(map[string]bool, len(cols))
	out := make([]WorkflowColumnConfig, 0, len(cols))
	for _, col := range cols {
		id := strings.ToLower(strings.TrimSpace(col.ID))
		if !knownWorkflowColumn(id) || seen[id] {
			continue
		}
		seen[id] = true
		width := col.Width
		if width <= 0 {
			width = DefaultWorkflowColumnWidth(id)
		}
		out = append(out, WorkflowColumnConfig{
			ID:    id,
			Width: ClampWorkflowColumnWidth(width),
		})
	}
	if len(out) == 0 {
		return defaultWorkflowColumns()
	}
	return out
}

// WorkflowColumnLayout returns the resolved workflows table layout.
func (c *Config) WorkflowColumnLayout() []WorkflowColumnConfig {
	if c == nil {
		return defaultWorkflowColumns()
	}
	return ResolveWorkflowColumns(c.WorkflowColumns)
}

// SetWorkflowColumns stores a resolved layout. Defaults are omitted from yaml.
func (c *Config) SetWorkflowColumns(cols []WorkflowColumnConfig) {
	if c == nil {
		return
	}
	resolved := ResolveWorkflowColumns(cols)
	if workflowColumnsEqual(resolved, defaultWorkflowColumns()) {
		c.WorkflowColumns = nil
		return
	}
	c.WorkflowColumns = resolved
}

func workflowColumnsEqual(a, b []WorkflowColumnConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Width != b[i].Width {
			return false
		}
	}
	return true
}
