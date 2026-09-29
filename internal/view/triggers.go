package view

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/galaxy-io/tempo/internal/command"
	"github.com/galaxy-io/tempo/internal/config"
)

// ioTriggerHit is a trigger that matched a highlighted value, with what its
// match pattern captured.
type ioTriggerHit struct {
	trigger config.TriggerConfig
	match   string
	groups  []string
}

// matchIOTrigger finds the first io_enter trigger whose conditions hold for a
// value at a dotted path. A trigger without a match pattern matches the whole
// value.
func matchIOTrigger(triggers []config.TriggerConfig, value, path string) (ioTriggerHit, bool, error) {
	for _, t := range triggers {
		if t.Event() != config.TriggerIOEnter {
			continue
		}
		if t.Path != "" {
			re, err := regexp.Compile(t.Path)
			if err != nil {
				return ioTriggerHit{}, false, fmt.Errorf("Trigger %q: bad path pattern: %s", triggerName(t), err)
			}
			if !re.MatchString(path) {
				continue
			}
		}
		hit := ioTriggerHit{trigger: t, match: value}
		if t.Match != "" {
			re, err := regexp.Compile(t.Match)
			if err != nil {
				return ioTriggerHit{}, false, fmt.Errorf("Trigger %q: bad match pattern: %s", triggerName(t), err)
			}
			found := re.FindStringSubmatch(value)
			if found == nil {
				continue
			}
			hit.match = found[0]
			hit.groups = found[1:]
		}
		return hit, true, nil
	}
	return ioTriggerHit{}, false, nil
}

func triggerName(t config.TriggerConfig) string {
	if t.Name != "" {
		return t.Name
	}
	return "trigger"
}

// jsonTreeDottedPath spells a row's path the way a trigger's path pattern
// sees it, such as items.0.url.
func jsonTreeDottedPath(path string) (dotted, key string) {
	parts := strings.Split(path, "\x00")
	return strings.Join(parts, "."), parts[len(parts)-1]
}

func (a *App) ioEnterTriggers() []config.TriggerConfig {
	if a == nil || a.config == nil {
		return nil
	}
	return a.config.GetMergedTriggers(a.activeProfile)
}

// hasIOEnterTriggers reports whether Enter on a value may do more than fold.
func (a *App) hasIOEnterTriggers() bool {
	for _, t := range a.ioEnterTriggers() {
		if t.Event() == config.TriggerIOEnter {
			return true
		}
	}
	return false
}

// runIOTrigger fires the first trigger matching a highlighted leaf row and
// reports whether one did, so Enter can fall back to folding.
func (a *App) runIOTrigger(row jsonTreeRow) bool {
	if row.foldable {
		return false
	}
	path, key := jsonTreeDottedPath(row.path)
	hit, ok, err := matchIOTrigger(a.ioEnterTriggers(), row.value, path)
	if err != nil {
		a.ToastError(err.Error())
		return true
	}
	if !ok {
		return false
	}

	t := hit.trigger
	name := triggerName(t)
	ctx := a.buildCommandContext(hit.groups)
	ctx.Value = row.value
	ctx.Match = hit.match
	ctx.Path = path
	ctx.Key = key

	switch {
	case t.URL != "":
		link, err := triggerURL(t.URL, ctx)
		if err != nil {
			a.ToastError(fmt.Sprintf("Trigger %q: %s", name, err))
			return true
		}
		open := func() {
			if err := openBrowser(link); err != nil {
				a.ToastError("Failed to open browser: " + err.Error())
				return
			}
			a.ToastSuccess("Opened " + link)
		}
		if t.Confirm {
			a.showRunConfirm(name, t.Description, "Open", link, open)
		} else {
			open()
		}
	case t.Cmd != "":
		expanded, err := command.ExpandShellCmd(t.Cmd, ctx)
		if err != nil {
			a.ToastError(fmt.Sprintf("Trigger %q: %s", name, err))
			return true
		}
		expanded = command.InjectConnectionFlags(expanded, ctx)
		cfg := config.CommandConfig{Description: t.Description, Cmd: t.Cmd, Output: t.Output}
		if cfg.Output == "" {
			cfg.Output = config.OutputNone
		}
		env := command.Env(ctx)
		if t.Confirm {
			a.showCommandConfirm(name, cfg, expanded, env)
		} else {
			a.runCommand(name, expanded, cfg, env)
		}
	default:
		a.ToastError(fmt.Sprintf("Trigger %q needs a url or a cmd", name))
	}
	return true
}

// triggerURL expands a trigger's URL and refuses anything but a web link, so a
// payload value cannot turn it into a file or an app to launch.
func triggerURL(template string, ctx command.Context) (string, error) {
	link, err := command.ExpandCmd(template, ctx)
	if err != nil {
		return "", err
	}
	link = strings.TrimSpace(link)
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("%q is not an http(s) URL", link)
	}
	return link, nil
}

// runIOTrigger lets Enter in the preview's input/output panes fire triggers.
func (wl *WorkflowList) runIOTrigger(row jsonTreeRow) bool {
	if wl.app == nil {
		return false
	}
	return wl.app.runIOTrigger(row)
}
