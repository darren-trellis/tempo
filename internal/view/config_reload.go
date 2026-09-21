package view

import (
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/theme/themes"
	"github.com/galaxy-io/tempo/internal/config"
)

const (
	configWatchInterval = 500 * time.Millisecond
	configWatchDebounce = 250 * time.Millisecond
)

func (a *App) startConfigWatch() {
	if a.stopConfigWatch == nil {
		a.stopConfigWatch = make(chan struct{})
	}
	go a.watchConfigFile()
}

func (a *App) watchConfigFile() {
	ticker := time.NewTicker(configWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopConfigWatch:
			return
		case <-ticker.C:
			a.pollConfigFile()
		}
	}
}

func (a *App) pollConfigFile() {
	data, changed, err := config.ReadChangedConfigFile()
	if err != nil || !changed {
		return
	}

	time.Sleep(configWatchDebounce)
	data, changed, err = config.ReadChangedConfigFile()
	if err != nil || !changed {
		return
	}

	cfg, err := config.ParseConfigFile(data)
	config.AcknowledgeConfigFile(data)
	if err != nil {
		a.ShowToastError("Config reload failed: " + err.Error())
		return
	}

	a.applyReloadedConfig(cfg)
}

func (a *App) applyReloadedConfig(cfg *config.Config) {
	a.applyLoadedConfig(cfg, false)
}

func (a *App) applyLoadedConfig(cfg *config.Config, force bool) {
	if cfg == nil {
		return
	}
	migrateSavedFilters(cfg)

	old := a.config
	if !force && old != nil && !old.ShouldAutoreload() && !cfg.ShouldAutoreload() {
		return
	}

	oldTheme := ""
	oldProfile := a.activeProfile
	oldPageSize := 0
	var oldConn config.ConnectionConfig
	var oldCols []config.WorkflowColumnConfig
	var oldActivityCols []config.WorkflowColumnConfig
	oldWorkflowTime := ""
	oldActivityTime := ""
	oldPrimaryTab := ""
	oldSecondaryTab := ""
	oldTertiaryTab := ""
	if old != nil {
		oldTheme = old.Theme
		oldCols = old.WorkflowColumnLayout()
		oldActivityCols = old.ActivityColumnLayout()
		oldWorkflowTime = old.ResolvedWorkflowTimeFormat()
		oldActivityTime = old.ResolvedActivityTimeFormat()
		oldPageSize = old.WorkflowPageLimit()
		oldPrimaryTab = old.ResolvedPrimaryTab()
		oldSecondaryTab = old.ResolvedSecondaryTab()
		oldTertiaryTab = old.ResolvedTertiaryTab()
		if conn, ok := old.GetProfile(oldProfile); ok {
			oldConn = conn
		}
	}

	newProfile := cfg.ActiveProfile
	if newProfile == "" {
		newProfile = oldProfile
	}
	newConn, hasConn := cfg.GetProfile(newProfile)
	needReconnect := hasConn && (newProfile != oldProfile || !config.ConnectionSettingsEqual(oldConn, newConn))
	needTheme := cfg.Theme != "" && cfg.Theme != oldTheme
	needColumns := !workflowColumnsEqual(cfg.WorkflowColumnLayout(), oldCols)
	needActivityColumns := !workflowColumnsEqual(cfg.ActivityColumnLayout(), oldActivityCols)
	needWorkflowTime := cfg.ResolvedWorkflowTimeFormat() != oldWorkflowTime
	needActivityTime := cfg.ResolvedActivityTimeFormat() != oldActivityTime

	a.config = cfg

	a.app.QueueUpdateDraw(func() {
		if needTheme {
			if selected := themes.Get(cfg.Theme); selected != nil {
				theme.SetProvider(selected)
			}
		}
		if current := a.app.Pages().Current(); current != nil {
			if wl, ok := current.(*WorkflowList); ok {
				wl.previewCache.setLimit(cfg.PreviewCacheLimit())
				if wl.taskQueues != nil && wl.taskQueues.cache != nil {
					wl.taskQueues.cache.setLimit(cfg.PreviewCacheLimit())
				}
				if needColumns || needWorkflowTime {
					wl.populateTable()
				}
				if needActivityColumns || needActivityTime {
					wl.renderActivityColumns()
				}
				if cfg.WorkflowPageLimit() != oldPageSize {
					wl.refresh()
				}
				if cfg.ResolvedPrimaryTab() != oldPrimaryTab ||
					cfg.ResolvedSecondaryTab() != oldSecondaryTab ||
					cfg.ResolvedTertiaryTab() != oldTertiaryTab {
					wl.applyConfiguredPaneTabs()
				}
				wl.syncAutoRefresh()
				if wl.taskQueuesActive() && wl.taskQueues != nil {
					wl.taskQueues.syncAutoRefresh()
				}
			}
			if nl, ok := current.(*NamespaceList); ok {
				nl.syncAutoRefresh()
			}
		}
		if !needReconnect {
			a.ToastSuccess("Reloaded config")
		}
	})

	if needReconnect {
		a.ShowToastSuccess("Reloaded config")
		a.applyProfile(newProfile, false)
	}
}

func workflowColumnsEqual(a, b []config.WorkflowColumnConfig) bool {
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
