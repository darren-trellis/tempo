package view

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/theme/themes"
	"github.com/galaxy-io/tempo/internal/config"
)

type tempoSetting struct {
	name   string
	help   string
	get    func(*config.Config) string
	apply  func(*App, string) error
	values func() []string
}

func onOffValues() []string { return []string{"on", "off"} }

func formatOnOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func parseOnOffValue(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("expected on or off")
	}
}

func parseBoundedInt(s string, min, max int) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("expected an integer")
	}
	if n < min || n > max {
		return 0, fmt.Errorf("expected %d-%d", min, max)
	}
	return n, nil
}

func tempoSettings() []tempoSetting {
	return []tempoSetting{
		{
			name: "theme",
			help: "Color theme",
			get: func(c *config.Config) string {
				if c != nil && strings.TrimSpace(c.Theme) != "" {
					return c.Theme
				}
				return config.DefaultTheme
			},
			apply: func(a *App, value string) error {
				if themes.Get(value) == nil {
					return fmt.Errorf("unknown theme %q", value)
				}
				a.applyThemeLive(value)
				return nil
			},
			values: config.ThemeNames,
		},
		boolSetting("autosave", "Write in-app changes to disk", func(c *config.Config) bool {
			return c.ShouldAutosave()
		}, func(c *config.Config, on bool) { c.Autosave = &on }),
		boolSetting("autoreload", "Reload config file changes live", func(c *config.Config) bool {
			return c.ShouldAutoreload()
		}, func(c *config.Config, on bool) { c.Autoreload = &on }),
		boolSetting("check_updates", "Check for updates", func(c *config.Config) bool {
			return c.ShouldCheckUpdates()
		}, func(c *config.Config, on bool) { c.CheckUpdates = &on }),
		{
			name: "help_style",
			help: "Help display",
			get: func(c *config.Config) string {
				if c == nil {
					return "modal"
				}
				return c.GetHelpStyle()
			},
			apply: func(a *App, value string) error {
				switch value {
				case "modal", "sheet":
					a.config.HelpStyle = value
					return nil
				default:
					return fmt.Errorf("help_style must be modal or sheet")
				}
			},
			values: func() []string { return []string{"modal", "sheet"} },
		},
		boolSetting("show_scrollbars", "Draw scrollbars", func(c *config.Config) bool {
			return c.ShouldShowScrollbars()
		}, func(c *config.Config, on bool) { c.ShowScrollbars = &on }),
		boolSetting("filter_wrap", "Wrap saved-filter chips onto another row", func(c *config.Config) bool {
			return c.ShouldWrapFilters()
		}, func(c *config.Config, on bool) { c.FilterWrap = &on }),
		{
			name: "modal_shadow",
			help: "Modal box shadow",
			get: func(c *config.Config) string {
				return c.ResolvedModalShadow()
			},
			apply: func(a *App, value string) error {
				switch strings.ToLower(strings.TrimSpace(value)) {
				case config.ModalShadowDirectional, config.ModalShadowNone, config.ModalShadowUniform:
					a.config.ModalShadow = strings.ToLower(strings.TrimSpace(value))
					return nil
				default:
					return fmt.Errorf("modal_shadow must be directional, none, or uniform")
				}
			},
			values: func() []string {
				return []string{config.ModalShadowDirectional, config.ModalShadowNone, config.ModalShadowUniform}
			},
		},
		boolSetting("color_code_activities", "Color activity rows by status", func(c *config.Config) bool {
			return c.ShouldColorCodeActivities()
		}, func(c *config.Config, on bool) { c.ColorCodeActivities = &on }),
		boolSetting("color_code_workflows", "Color workflow rows by status", func(c *config.Config) bool {
			return c.ShouldColorCodeWorkflows()
		}, func(c *config.Config, on bool) { c.ColorCodeWorkflows = &on }),
		{
			name: "workflow_time_format",
			help: "Workflow started/ended display",
			get:  func(c *config.Config) string { return c.ResolvedWorkflowTimeFormat() },
			apply: func(a *App, value string) error {
				return setTimeFormat(&a.config.WorkflowTimeFormat, value)
			},
			values: func() []string { return []string{config.TimeFormatRelative, config.TimeFormatAbsolute} },
		},
		{
			name: "activity_time_format",
			help: "Activity started/ended display",
			get:  func(c *config.Config) string { return c.ResolvedActivityTimeFormat() },
			apply: func(a *App, value string) error {
				return setTimeFormat(&a.config.ActivityTimeFormat, value)
			},
			values: func() []string { return []string{config.TimeFormatRelative, config.TimeFormatAbsolute} },
		},
		{
			name: "reset_point",
			help: "Default workflow reset point",
			get: func(c *config.Config) string {
				if c == nil {
					return config.ResetPointFirst
				}
				return c.ResetPointDefault()
			},
			apply: func(a *App, value string) error {
				switch strings.ToLower(value) {
				case config.ResetPointFirst, config.ResetPointLast:
					a.config.ResetPoint = strings.ToLower(value)
					return nil
				default:
					return fmt.Errorf("reset_point must be first or last")
				}
			},
			values: func() []string { return []string{config.ResetPointFirst, config.ResetPointLast} },
		},
		{
			name: "reset_reason",
			help: "Default workflow reset reason",
			get: func(c *config.Config) string {
				if c == nil {
					return config.DefaultResetReason
				}
				return c.ResetReasonDefault()
			},
			apply: func(a *App, value string) error {
				a.config.ResetReason = strings.TrimSpace(value)
				return nil
			},
		},
		intSetting("preview_cache_size", "Preview history cache size", func(c *config.Config) int {
			return c.PreviewCacheLimit()
		}, func(c *config.Config, n int) { c.PreviewCacheSize = &n }, 0, config.MaxPreviewCacheSize),
		intSetting("mouse_scroll_step", "Mouse wheel step", func(c *config.Config) int {
			return c.MouseScrollStepSize()
		}, func(c *config.Config, n int) { c.MouseScrollStep = &n }, config.DefaultMouseScrollStep, config.MaxMouseScrollStep),
		intSetting("workflow_page_size", "Workflows per page", func(c *config.Config) int {
			return c.WorkflowPageLimit()
		}, func(c *config.Config, n int) { c.WorkflowPageSize = &n }, config.MinWorkflowPageSize, config.MaxWorkflowPageSize),
		durationSetting("refresh_rate", "Auto-refresh interval", func(c *config.Config) *config.Setting {
			if c == nil {
				return nil
			}
			return c.RefreshInterval
		}, func(c *config.Config, s *config.Setting) { c.RefreshInterval = s }, config.DefaultRefreshRate.String()),
		durationSetting("preview_load_delay", "Preview load delay", func(c *config.Config) *config.Setting {
			if c == nil {
				return nil
			}
			return c.PreviewLoadWait
		}, func(c *config.Config, s *config.Setting) { c.PreviewLoadWait = s }, config.DefaultPreviewLoadDelay.String()),
		durationSetting("worker_poll_quiet_after", "Worker poll stale after", func(c *config.Config) *config.Setting {
			if c == nil {
				return nil
			}
			return c.WorkerPollQuiet
		}, func(c *config.Config, s *config.Setting) { c.WorkerPollQuiet = s }, ""),
		durationSetting("worker_heartbeat_quiet_after", "Worker heartbeat stale after", func(c *config.Config) *config.Setting {
			if c == nil {
				return nil
			}
			return c.WorkerHeartbeatQuiet
		}, func(c *config.Config, s *config.Setting) { c.WorkerHeartbeatQuiet = s }, ""),
	}
}

func boolSetting(name, help string, get func(*config.Config) bool, set func(*config.Config, bool)) tempoSetting {
	return tempoSetting{
		name: name,
		help: help,
		get: func(c *config.Config) string {
			return formatOnOff(get(c))
		},
		apply: func(a *App, value string) error {
			on, err := parseOnOffValue(value)
			if err != nil {
				return err
			}
			set(a.config, on)
			return nil
		},
		values: onOffValues,
	}
}

func intSetting(name, help string, get func(*config.Config) int, set func(*config.Config, int), min, max int) tempoSetting {
	return tempoSetting{
		name: name,
		help: help,
		get: func(c *config.Config) string {
			return strconv.Itoa(get(c))
		},
		apply: func(a *App, value string) error {
			n, err := parseBoundedInt(value, min, max)
			if err != nil {
				return err
			}
			set(a.config, n)
			return nil
		},
	}
}

func durationSetting(name, help string, get func(*config.Config) *config.Setting, set func(*config.Config, *config.Setting), fallback string) tempoSetting {
	return tempoSetting{
		name: name,
		help: help,
		get: func(c *config.Config) string {
			if text := strings.TrimSpace(get(c).Text()); text != "" {
				return text
			}
			return fallback
		},
		apply: func(a *App, value string) error {
			s := config.NewSetting(value)
			if _, ok := s.Duration(); !ok && strings.TrimSpace(value) != "" {
				return fmt.Errorf("expected a duration such as 1s or 500ms")
			}
			set(a.config, s)
			return nil
		},
	}
}

func setTimeFormat(dst *string, value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case config.TimeFormatRelative, config.TimeFormatAbsolute:
		*dst = strings.ToLower(strings.TrimSpace(value))
		return nil
	default:
		return fmt.Errorf("expected relative or absolute")
	}
}

func lookupTempoSetting(name string) (tempoSetting, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range tempoSettings() {
		if s.name == name {
			return s, true
		}
	}
	return tempoSetting{}, false
}

func tempoSettingNames() []string {
	all := tempoSettings()
	names := make([]string, len(all))
	for i, s := range all {
		names[i] = s.name
	}
	return names
}

func (a *App) execConfig(args []string) bool {
	if len(args) == 0 {
		a.showConfigGet("")
		return true
	}
	switch strings.ToLower(args[0]) {
	case "get":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		a.showConfigGet(name)
	case "set":
		a.execConfigSet(args[1:])
	case "load":
		a.execConfigLoad()
	case "save":
		a.execConfigSave()
	case "reset":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		a.execConfigReset(name)
	default:
		a.ToastWarning("usage: config get|set|load|save|reset")
	}
	return true
}

func (a *App) showConfigGet(name string) {
	if a.config == nil {
		a.ToastWarning("No config loaded")
		return
	}
	if name != "" {
		s, ok := lookupTempoSetting(name)
		if !ok {
			a.ToastWarning(unknownSettingError(name))
			return
		}
		a.ToastSuccess(s.name + "=" + s.get(a.config))
		return
	}
	if a.app == nil {
		return
	}
	table := components.NewTable()
	table.SetBorder(false)
	table.SetHeaders("SETTING", "VALUE")
	for _, s := range tempoSettings() {
		table.AddRow(s.name, s.get(a.config))
	}
	table.SelectRow(0)
	modal := newModal(components.ModalConfig{
		Title:    fmt.Sprintf("%s Config", theme.IconInfo),
		Width:    56,
		Height:   22,
		Backdrop: true,
	})
	modal.SetContent(table)
	modal.SetHints([]components.KeyHint{{Key: "esc", Description: "Close"}})
	modal.SetOnCancel(func() {
		a.app.Pages().DismissModal()
		a.refocusCurrent()
	})
	a.PushModal(modal)
	a.app.SetFocus(table)
}

func (a *App) execConfigSet(args []string) {
	if a.config == nil {
		a.ToastWarning("No config loaded")
		return
	}
	if len(args) == 0 {
		a.showConfigNamePicker()
		return
	}
	s, ok := lookupTempoSetting(args[0])
	if !ok {
		a.ToastWarning(unknownSettingError(args[0]))
		return
	}
	if len(args) == 1 {
		a.showConfigValuePicker(s)
		return
	}
	a.commitSetting(s, strings.Join(args[1:], " "))
}

func (a *App) showConfigNamePicker() {
	all := tempoSettings()
	labels := make([]string, len(all))
	helps := make([]string, len(all))
	for i, s := range all {
		labels[i] = s.name
		helps[i] = s.help
	}
	a.showOptionPicker("Set Config", labels, helps, "", nil, func(name string) {
		if s, ok := lookupTempoSetting(name); ok {
			a.showConfigValuePicker(s)
		}
	}, nil)
}

func (a *App) showConfigValuePicker(s tempoSetting) {
	if s.name == "theme" {
		a.showThemeSelector()
		return
	}
	var values []string
	if s.values != nil {
		values = s.values()
	}
	if len(values) == 0 {
		a.ToastWarning("usage: config set " + s.name + " <value>")
		return
	}
	current := s.get(a.config)
	original := current
	helps := make([]string, len(values))
	a.showOptionPicker("Set "+s.name, values, helps, current, func(value string) {
		_ = s.apply(a, value)
		a.applySettingSideEffects(s.name)
	}, func(value string) {
		a.commitSetting(s, value)
	}, func() {
		_ = s.apply(a, original)
		a.applySettingSideEffects(s.name)
	})
}

func (a *App) showOptionPicker(title string, values, helps []string, current string, onPreview, onSelect func(string), onCancel func()) {
	if a == nil || a.app == nil {
		return
	}
	table := components.NewTable()
	table.SetBorder(false)
	if len(helps) > 0 && helps[0] != "" {
		table.SetHeaders("", "OPTION", "HELP")
	} else {
		table.SetHeaders("", "OPTION")
	}
	currentIdx := 0
	for i, value := range values {
		marker := " "
		if value == current {
			marker = "●"
			currentIdx = i
		}
		if len(helps) > i && helps[i] != "" {
			table.AddRow(marker, value, helps[i])
		} else {
			table.AddRow(marker, value)
		}
	}
	if len(values) > 0 {
		table.SelectRow(currentIdx)
	}
	committed := false
	table.SetSelectionChangedFunc(func(row, col int) {
		if onPreview == nil {
			return
		}
		idx := table.SelectedRow()
		if idx < 0 || idx >= len(values) {
			return
		}
		onPreview(values[idx])
	})
	table.SetOnSelect(func(row int) {
		if row < 0 || row >= len(values) {
			return
		}
		committed = true
		a.app.Pages().DismissModal()
		if onSelect != nil {
			onSelect(values[row])
		}
		if !a.app.Pages().CurrentIsModal() {
			a.refocusCurrent()
		}
	})
	modal := newOverlayModal(components.ModalConfig{
		Title:  title,
		Width:  56,
		Height: 18,
	}, a.currentContent())
	modal.SetContent(table)
	modal.SetHints([]components.KeyHint{
		{Key: "enter", Description: "Select"},
		{Key: "esc", Description: "Cancel"},
	})
	restore := func() {
		if committed || onCancel == nil {
			return
		}
		onCancel()
	}
	modal.SetOnDismiss(func() bool {
		restore()
		return true
	})
	modal.SetOnCancel(func() {
		restore()
		a.app.Pages().DismissModal()
		a.refocusCurrent()
	})
	a.PushModal(modal)
	if a.JigApp() != nil {
		a.JigApp().SetFocus(table)
	}
}

func (a *App) commitSetting(s tempoSetting, value string) {
	if err := s.apply(a, value); err != nil {
		a.ToastWarning(err.Error())
		return
	}
	a.applySettingSideEffects(s.name)
	if a.config != nil && a.config.ShouldAutosave() {
		if err := a.persistConfig(); err != nil {
			a.ToastError("Failed to save config: " + err.Error())
			return
		}
	}
	a.ToastSuccess(s.name + "=" + s.get(a.config))
}

func (a *App) applySettingSideEffects(name string) {
	if a == nil {
		return
	}
	wl, hasWL := a.workflowList()
	switch name {
	case "color_code_workflows", "workflow_time_format", "workflow_page_size":
		if hasWL {
			if name == "workflow_page_size" {
				wl.refresh()
				return
			}
			wl.populateTable()
		}
	case "color_code_activities", "activity_time_format":
		if hasWL {
			wl.renderActivityColumns()
		}
	case "preview_cache_size":
		if hasWL && a.config != nil {
			wl.previewCache.setLimit(a.config.PreviewCacheLimit())
			if wl.taskQueues != nil && wl.taskQueues.cache != nil {
				wl.taskQueues.cache.setLimit(a.config.PreviewCacheLimit())
			}
		}
	case "refresh_rate":
		if hasWL {
			wl.syncAutoRefresh()
			if wl.taskQueues != nil {
				wl.taskQueues.syncAutoRefresh()
			}
		}
		if nl, ok := a.namespaceListView(); ok {
			nl.syncAutoRefresh()
		}
	case "filter_wrap":
		if hasWL {
			wl.syncFilterBarHeight()
		}
	}
}

func (a *App) execConfigLoad() {
	cfg, err := config.Load()
	if err != nil {
		a.ToastError("Config load failed: " + err.Error())
		return
	}
	a.configUnreadable = false
	a.applyLoadedConfig(cfg, true)
}

func (a *App) execConfigSave() {
	if err := a.persistConfig(); err != nil {
		a.ToastError("Failed to save config: " + err.Error())
		return
	}
	if a.configUnreadable {
		return
	}
	a.ToastSuccess("Saved " + config.ConfigPath())
}

func (a *App) execConfigReset(name string) {
	if a.config == nil {
		a.ToastWarning("No config loaded")
		return
	}
	if name == "" {
		for _, s := range tempoSettings() {
			_ = s.apply(a, s.get(config.DefaultConfig()))
			a.applySettingSideEffects(s.name)
		}
		if a.config.ShouldAutosave() {
			_ = a.persistConfig()
		}
		a.ToastSuccess("Reset settings to defaults")
		return
	}
	s, ok := lookupTempoSetting(name)
	if !ok {
		a.ToastWarning(unknownSettingError(name))
		return
	}
	a.commitSetting(s, s.get(config.DefaultConfig()))
}

func unknownSettingError(name string) string {
	return fmt.Sprintf("unknown setting %q", name)
}

func (a *App) persistConfig() error {
	if a == nil || a.config == nil {
		return nil
	}
	if a.configUnreadable {
		a.ToastError("Not saving: " + config.ConfigPath() + " could not be read")
		return nil
	}
	return a.config.Save()
}
