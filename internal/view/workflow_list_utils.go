package view

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// formatRelativeTime formats a time as a human-readable relative string.
func formatRelativeTime(now time.Time, t time.Time) string {
	d := now.Sub(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd ago", days)
}

// truncate truncates a string to maxLen, adding ellipsis if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// truncateIfNeeded only truncates if the string exceeds maxLen.
// If maxLen is 0 or negative, returns the string unchanged.
func truncateIfNeeded(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// copyToClipboard copies text to the system clipboard.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("clipboard not available: install xclip or xsel")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := pipe.Write([]byte(text)); err != nil {
		return err
	}

	if err := pipe.Close(); err != nil {
		return err
	}

	return cmd.Wait()
}

var openBrowser = openBrowserOS

func openBrowserOS(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("empty URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("opening a browser is not supported on %s", runtime.GOOS)
	}
	return cmd.Start()
}

func workflowEndTime(now time.Time, w temporal.Workflow) string {
	if w.EndTime != nil {
		return formatRelativeTime(now, *w.EndTime)
	}
	return "-"
}

func workflowDuration(now time.Time, w temporal.Workflow) string {
	if w.EndTime != nil {
		return w.EndTime.Sub(w.StartTime).Round(time.Second).String()
	}
	if w.Status == "Running" {
		return now.Sub(w.StartTime).Round(time.Second).String()
	}
	return "-"
}

func (wl *WorkflowList) copyWorkflowID() {
	row := wl.table.SelectedRow()
	if row < 0 || row >= len(wl.workflows) {
		return
	}

	wf := wl.workflows[row]
	if err := copyToClipboard(wf.ID); err != nil {
		if wl.app != nil {
			wl.app.ToastError("Failed to copy: " + err.Error())
		}
		return
	}
	if wl.app != nil {
		wl.app.ToastSuccess("Copied workflow ID")
	}
}

func (wl *WorkflowList) openSelectedWorkflowUI() bool {
	if wl == nil || wl.app == nil {
		return false
	}
	w, ok := wl.selectedWorkflow()
	if !ok {
		wl.app.ToastError("No workflow selected")
		return true
	}
	wl.app.OpenWorkflowInBrowser(w.ID, w.RunID)
	return true
}
