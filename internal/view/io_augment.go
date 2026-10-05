package view

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/galaxy-io/tempo/internal/command"
	"github.com/galaxy-io/tempo/internal/config"
)

const (
	// ioAugmentTimeout bounds one io_show command.
	ioAugmentTimeout = 30 * time.Second
	// ioAugmentRetry is how long a failed io_show command's error is shown
	// before the next render runs it again.
	ioAugmentRetry = 30 * time.Second
	// ioAugmentCacheSize is how many io_show results are kept.
	ioAugmentCacheSize = 256
	// ioAugmentPending stands in for a result still being fetched.
	ioAugmentPending = "fetching…"
)

type ioAugmentResult struct {
	value    any
	done     bool
	failedAt time.Time
}

// ioAugmenter runs io_show commands in the background and keeps what they
// printed, so redrawing a payload does not run them again.
type ioAugmenter struct {
	mu      sync.Mutex
	results map[string]*ioAugmentResult
	order   []string
	// spawn runs a command off the UI thread; tests run it in place.
	spawn func(func())
}

// lookup returns the result for key, starting run when there is none yet or
// a failure is old enough to try again. done is called once run's result is
// stored.
func (g *ioAugmenter) lookup(key string, run func() (any, bool), done func()) (any, bool) {
	g.mu.Lock()
	if r, ok := g.results[key]; ok {
		retry := !r.failedAt.IsZero() && time.Since(r.failedAt) > ioAugmentRetry
		if !r.done || !retry {
			g.mu.Unlock()
			return r.value, r.done
		}
	}
	if g.results == nil {
		g.results = make(map[string]*ioAugmentResult)
	}
	if _, exists := g.results[key]; !exists {
		g.order = append(g.order, key)
	}
	g.results[key] = &ioAugmentResult{value: ioAugmentPending}
	for len(g.order) > ioAugmentCacheSize {
		delete(g.results, g.order[0])
		g.order = g.order[1:]
	}
	spawn := g.spawn
	g.mu.Unlock()

	work := func() {
		value, ok := run()
		g.mu.Lock()
		if r, exists := g.results[key]; exists {
			r.value, r.done = value, true
			if !ok {
				r.failedAt = time.Now()
			}
		}
		g.mu.Unlock()
		if done != nil {
			done()
		}
	}
	if spawn == nil {
		go work()
		return ioAugmentPending, false
	}
	spawn(work)
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.results[key]; ok {
		return r.value, r.done
	}
	return ioAugmentPending, false
}

// ioAugmentEntry is one io_show result to add to a payload.
type ioAugmentEntry struct {
	label string
	value any
}

// augmentIO adds to content what each io_show trigger's command prints for
// the values it matches. Commands run in the background; until one is done
// its entry reads "fetching…", and rerender is called when it finishes.
func (a *App) augmentIO(content string, rerender func()) string {
	if a == nil || a.config == nil {
		return content
	}
	var triggers []config.TriggerConfig
	for _, t := range a.config.GetMergedTriggers(a.activeProfile) {
		if t.Event() == config.TriggerIOShow && t.Cmd != "" {
			triggers = append(triggers, t)
		}
	}
	if len(triggers) == 0 {
		return content
	}
	rows, ok := buildJSONTreeRows(content, nil)
	if !ok {
		return content
	}

	var entries []ioAugmentEntry
	for _, row := range rows {
		if row.foldable {
			continue
		}
		path, key := jsonTreeDottedPath(row.path)
		for _, t := range triggers {
			hit, matched, err := matchTrigger(t, row.value, path)
			if err != nil {
				entries = append(entries, ioAugmentEntry{label: triggerName(t), value: "error: " + err.Error()})
				continue
			}
			if !matched {
				continue
			}
			ctx := a.buildCommandContext(hit.groups)
			ctx.Value, ctx.Match, ctx.Path, ctx.Key = row.value, hit.match, path, key
			expanded, err := command.ExpandShellCmd(t.Cmd, ctx)
			if err != nil {
				entries = append(entries, ioAugmentEntry{label: triggerName(t), value: "error: " + err.Error()})
				continue
			}
			env := command.Env(ctx)
			value, _ := a.augments.lookup(triggerName(t)+"\x00"+expanded, func() (any, bool) {
				return runIOAugment(expanded, env)
			}, func() { a.queueUI(rerender) })
			entries = append(entries, ioAugmentEntry{label: triggerName(t), value: value})
		}
	}
	if len(entries) == 0 {
		return content
	}
	return appendIOEntries(content, labelIOEntries(entries))
}

// runIOAugment runs an io_show command. JSON it prints is added as JSON, so
// it can be folded and browsed like the rest of the payload; anything else
// is added as text.
func runIOAugment(expanded string, env []string) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), ioAugmentTimeout)
	defer cancel()
	stdout, stderr, err := command.Output(ctx, expanded, env...)
	if err != nil {
		reason := lastOutputLine(stderr)
		if reason == "" {
			reason = err.Error()
		}
		return "error: " + reason, false
	}
	if values, ok := decodeJSONValues(strings.TrimSpace(stdout)); ok {
		if len(values) == 1 {
			return values[0], true
		}
		return values, true
	}
	return strings.TrimSpace(stdout), true
}

// labelIOEntries tells apart entries from the same trigger, such as two S3
// keys in one payload, by numbering them.
func labelIOEntries(entries []ioAugmentEntry) []ioAugmentEntry {
	counts := make(map[string]int)
	for _, e := range entries {
		counts[e.label]++
	}
	seen := make(map[string]int)
	for i, e := range entries {
		if counts[e.label] > 1 {
			seen[e.label]++
			entries[i].label = fmt.Sprintf("%s %d", e.label, seen[e.label])
		}
	}
	return entries
}

// appendIOEntries adds entries to a payload: as keys when it is one object,
// otherwise as one more value after it.
func appendIOEntries(content string, entries []ioAugmentEntry) string {
	values, ok := decodeJSONValues(strings.TrimSpace(content))
	if !ok {
		return content
	}
	if len(values) == 1 {
		if object, isObject := values[0].(map[string]any); isObject {
			for _, e := range entries {
				label := e.label
				for n := 2; ; n++ {
					if _, taken := object[label]; !taken {
						break
					}
					label = fmt.Sprintf("%s (%d)", e.label, n)
				}
				object[label] = e.value
			}
			if encoded, err := json.Marshal(object); err == nil {
				return string(encoded)
			}
			return content
		}
	}
	extra := make(map[string]any, len(entries))
	for _, e := range entries {
		extra[e.label] = e.value
	}
	encoded, err := json.Marshal(extra)
	if err != nil {
		return content
	}
	return strings.TrimSpace(content) + ", " + string(encoded)
}

// queueUI runs fn on the UI thread, or in place when there is no UI.
func (a *App) queueUI(fn func()) {
	if fn == nil {
		return
	}
	if a == nil || a.app == nil {
		fn()
		return
	}
	go a.app.QueueUpdateDraw(fn)
}
