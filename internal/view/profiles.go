package view

import (
	"context"
	"strings"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func (a *App) profileTitle() string {
	if a == nil {
		return ""
	}
	if a.chromeProfile != "" {
		return a.chromeProfile
	}
	return a.activeProfile
}

func (a *App) setProfile(name string) {
	if a == nil {
		return
	}
	a.chromeProfile = name
	if wl, ok := a.currentContent().(*WorkflowList); ok {
		wl.applyProfileTitle()
	}
}

// ShowProfileSelector opens the profile selector modal.
func (a *App) ShowProfileSelector() {
	if a.config == nil {
		return
	}

	modal := NewProfileModal()
	modal.SetProfiles(a.config.ListProfiles(), a.activeProfile, a.config)
	modal.SetOnSelect(func(name string) {
		a.closeProfileSelector()
		a.SwitchProfile(name)
	})
	modal.SetOnNew(func() {
		a.closeProfileSelector()
		a.showProfileForm("")
	})
	modal.SetOnEdit(func(name string) {
		a.closeProfileSelector()
		a.showProfileForm(name)
	})
	modal.SetOnDelete(func(name string) {
		a.deleteProfile(name)
		modal.SetProfiles(a.config.ListProfiles(), a.activeProfile, a.config)
	})
	modal.SetOnClose(func() {
		a.closeProfileSelector()
	})

	a.PushModal(modal)
	a.app.SetFocus(modal)
}

func (a *App) closeProfileSelector() {
	a.app.Pages().DismissModal()
}

func (a *App) showProfileForm(editName string) {
	form := NewProfileForm()

	if editName != "" {
		if cfg, ok := a.config.GetProfile(editName); ok {
			form.SetProfile(editName, cfg)
		}
	}

	form.SetOnSave(func(name string, cfg config.ConnectionConfig) {
		a.closeProfileForm()
		if err := a.config.SaveProfile(name, cfg); err != nil {
			// Log error but continue
			return
		}
		if err := a.SaveConfig(); err != nil {
			// Log error but continue
		}
		a.SwitchProfile(name)
	})
	form.SetOnCancel(func() {
		a.closeProfileForm()
	})

	a.PushModal(form)
	a.app.SetFocus(form)
}

func (a *App) closeProfileForm() {
	a.app.Pages().DismissModal()
}

func (a *App) deleteProfile(name string) {
	if a.config == nil {
		return
	}
	if err := a.config.DeleteProfile(name); err != nil {
		return
	}
	_ = a.SaveConfig()
}

// SwitchProfile switches to a different connection profile.
func (a *App) SwitchProfile(name string) {
	a.applyProfile(name, true)
}

func (a *App) applyProfile(name string, persist bool) {
	a.mu.RLock()
	provider := a.provider
	currentProfile := a.activeProfile
	a.mu.RUnlock()

	if a.config == nil || provider == nil {
		return
	}

	profileCfg, ok := a.config.GetProfile(name)
	if !ok {
		return
	}
	if err := profileCfg.CloudAPIKeyError(); err != nil {
		a.ToastError(err.Error())
		return
	}
	profileCfg = profileCfg.ExpandEnv()

	connConfig := temporal.ConnectionConfig{
		Address:       profileCfg.Address,
		Namespace:     profileCfg.Namespace,
		TLSCertPath:   profileCfg.TLS.Cert,
		TLSKeyPath:    profileCfg.TLS.Key,
		TLSCAPath:     profileCfg.TLS.CA,
		TLSServerName: profileCfg.TLS.ServerName,
		TLSSkipVerify: profileCfg.TLS.SkipVerify,
		APIKey:        profileCfg.APIKey,
		GRPCMeta:      profileCfg.GRPCMeta,
		CodecEndpoint: profileCfg.CodecEndpoint,
	}

	// QueueUpdateDraw from the UI thread deadlocks (Enter in the profile modal).
	go func() {
		a.app.QueueUpdateDraw(func() {
			if current := a.app.Pages().Current(); current != nil {
				current.Stop()
			}
			a.setProfile(name + " (connecting...)")
			a.setConnected(false)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := provider.ReconnectWithConfig(ctx, connConfig)
		cancel()

		if err == nil {
			a.mu.Lock()
			a.activeProfile = name
			a.currentNS = connConfig.Namespace
			a.mu.Unlock()

			a.config.SetActiveProfile(name)
			if persist {
				_ = a.SaveConfig()
			}
		}

		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.setProfile(currentProfile + " (failed)")
				a.setConnected(false)
				return
			}

			a.setProfile(name)
			a.setConnected(true)
			a.resetNamespaceCatalog()

			a.reinitializeViews()
		})
	}()
}

// reinitializeViews resets the view stack after a profile switch.
func (a *App) reinitializeViews() {
	a.app.Pages().Clear()

	// If a namespace is defined in the connection, skip namespace list and go directly to workflows
	if a.provider != nil && a.provider.Config().Namespace != "" {
		wl := NewWorkflowList(a, a.currentNS)
		a.app.Pages().Push(wl)
		a.app.SetFocus(wl)
	} else {
		a.namespaceList = NewNamespaceList(a)
		a.app.Pages().Push(a.namespaceList)
		a.app.SetFocus(a.namespaceList)
	}
}

func (a *App) handleProfileCommand(args string) {
	args = strings.TrimSpace(args)

	if args == "" {
		a.ShowProfileSelector()
		return
	}

	parts := strings.Fields(args)
	cmd := parts[0]

	switch cmd {
	case "new":
		a.showProfileForm("")
	case "edit":
		if len(parts) > 1 {
			a.showProfileForm(parts[1])
		} else {
			a.showProfileForm(a.activeProfile)
		}
	case "delete":
		if len(parts) > 1 {
			a.deleteProfile(parts[1])
		}
	case "save":
		a.showProfileForm("")
	default:
		if a.config != nil && a.config.ProfileExists(cmd) {
			a.SwitchProfile(cmd)
		}
	}
}
