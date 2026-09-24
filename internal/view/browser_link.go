package view

import (
	"fmt"

	"github.com/galaxy-io/tempo/internal/config"
)

// OpenWorkflowInBrowser opens the workflow run in the Temporal Web UI.
func (a *App) OpenWorkflowInBrowser(workflowID, runID string) {
	if a == nil {
		return
	}
	link, err := a.workflowUILink(workflowID, runID)
	if err != nil {
		a.ToastError(err.Error())
		return
	}
	if err := openBrowser(link); err != nil {
		a.ToastError("Failed to open browser: " + err.Error())
		return
	}
	a.ToastSuccess("Opened in browser")
}

func (a *App) workflowUILink(workflowID, runID string) (string, error) {
	if workflowID == "" {
		return "", fmt.Errorf("No workflow selected")
	}
	profile := a.uiProfile()
	base := profile.ResolveUIURL()
	if base == "" {
		return "", fmt.Errorf("Set ui_url on this profile to open the Web UI")
	}
	namespace := a.CurrentNamespace()
	if namespace == "" {
		namespace = profile.Namespace
	}
	link := config.WorkflowUIURL(base, namespace, workflowID, runID)
	if link == "" {
		return "", fmt.Errorf("Could not build the Web UI URL")
	}
	return link, nil
}

func (a *App) uiProfile() config.ConnectionConfig {
	if a == nil {
		return config.ConnectionConfig{}
	}
	if a.config != nil {
		if a.activeProfile != "" {
			if profile, ok := a.config.GetProfile(a.activeProfile); ok {
				return profile
			}
		}
		_, profile := a.config.GetActiveProfile()
		return profile
	}
	if a.provider != nil {
		tc := a.provider.Config()
		return config.ConnectionConfig{Address: tc.Address, Namespace: tc.Namespace}
	}
	return config.ConnectionConfig{}
}
