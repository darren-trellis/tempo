package view

import (
	"sort"
	"strings"
)

const commandSuggestMax = 8

type commandSuggestion struct {
	Text        string
	Label       string
	Help        string
	ReplaceFrom int
}

type commandInfo struct {
	name string
	help string
}

type commandCompletionState struct {
	items    []commandSuggestion
	selected *int
	browsed  bool
	scroll   int
}

func builtinCommandCatalog() []commandInfo {
	return []commandInfo{
		{name: "quit", help: "Exit tempo"},
		{name: "help", help: "Show help"},
		{name: "command", help: "Open command prompt"},
		{name: "debug", help: "Open debug screen"},
		{name: "refresh", help: "Refresh the current view"},
		{name: "auto", help: "Toggle auto-refresh"},
		{name: "auto on", help: "Enable auto-refresh"},
		{name: "auto off", help: "Disable auto-refresh"},
		{name: "auto toggle", help: "Toggle auto-refresh"},
		{name: "profile", help: "Switch or manage profiles"},
		{name: "profile new", help: "Create a profile"},
		{name: "profile save", help: "Create a profile"},
		{name: "profile edit", help: "Edit a profile"},
		{name: "profile delete", help: "Delete a profile"},
		{name: "config", help: "Show or change settings"},
		{name: "config get", help: "Show a setting"},
		{name: "config set", help: "Change a setting"},
		{name: "config load", help: "Reload config from disk"},
		{name: "config save", help: "Write config to disk"},
		{name: "config reset", help: "Reset settings to defaults"},
		{name: "nav up", help: "Move selection up"},
		{name: "nav down", help: "Move selection down"},
		{name: "nav top", help: "Jump to top"},
		{name: "nav bottom", help: "Jump to bottom"},
		{name: "nav page", help: "Page up or down"},
		{name: "nav page up", help: "Page up"},
		{name: "nav page down", help: "Page down"},
		{name: "nav left", help: "Scroll left"},
		{name: "nav right", help: "Scroll right"},
		{name: "focus cycle", help: "Cycle panes forward"},
		{name: "focus prev", help: "Cycle panes backward"},
		{name: "focus list", help: "Focus the primary list"},
		{name: "focus preview", help: "Focus preview"},
		{name: "focus timeline", help: "Focus timeline"},
		{name: "focus detail", help: "Focus the side pane"},
		{name: "focus pollers", help: "Focus pollers"},
		{name: "tab workflows", help: "Show workflows"},
		{name: "tab queues", help: "Show task queues"},
		{name: "tab schedules", help: "Show schedules"},
		{name: "tab workers", help: "Show workers"},
		{name: "preview", help: "Toggle preview"},
		{name: "preview on", help: "Show preview"},
		{name: "preview off", help: "Hide preview"},
		{name: "preview toggle", help: "Toggle preview"},
		{name: "preview details", help: "Show workflow details"},
		{name: "preview activities", help: "Show activities"},
		{name: "preview events", help: "Show events"},
		{name: "preview hierarchy", help: "Show hierarchy"},
		{name: "timeline", help: "Toggle timeline"},
		{name: "timeline on", help: "Show timeline"},
		{name: "timeline off", help: "Hide timeline"},
		{name: "timeline toggle", help: "Toggle timeline"},
		{name: "timeline wide", help: "Widen timeline"},
		{name: "timeline narrow", help: "Narrow timeline"},
		{name: "tree", help: "Toggle tree view"},
		{name: "tree on", help: "Show tree view"},
		{name: "tree off", help: "Show list view"},
		{name: "tree toggle", help: "Toggle tree view"},
		{name: "fold", help: "Toggle fold"},
		{name: "fold on", help: "Collapse selected"},
		{name: "fold off", help: "Expand selected"},
		{name: "fold toggle", help: "Toggle fold"},
		{name: "fold all", help: "Fold or unfold all"},
		{name: "fold all on", help: "Collapse all"},
		{name: "fold all off", help: "Expand all"},
		{name: "fold all toggle", help: "Toggle fold all"},
		{name: "columns", help: "Edit table columns"},
		{name: "legend", help: "Show timeline legend"},
		{name: "io", help: "Show input/output"},
		{name: "editor", help: "Open payload in editor"},
		{name: "ui", help: "Open in Web UI"},
		{name: "search", help: "Search the current list"},
		{name: "filter", help: "Filter workflows"},
		{name: "filter save", help: "Save the current query"},
		{name: "filter load", help: "Load a saved filter"},
		{name: "query", help: "Edit visibility query"},
		{name: "query templates", help: "Open query templates"},
		{name: "query clear", help: "Clear visibility query"},
		{name: "date", help: "Set date range"},
		{name: "workflow start", help: "Start a workflow"},
		{name: "workflow cancel", help: "Cancel selected workflow"},
		{name: "workflow terminate", help: "Terminate selected workflow"},
		{name: "workflow signal", help: "Signal selected workflow"},
		{name: "workflow query", help: "Query selected workflow"},
		{name: "workflow reset", help: "Reset selected workflow"},
		{name: "workflow delete", help: "Delete selected workflow"},
		{name: "workflow yank", help: "Copy workflow ID"},
		{name: "workflow select", help: "Toggle select mode"},
		{name: "workflow select on", help: "Enter select mode"},
		{name: "workflow select off", help: "Leave select mode"},
		{name: "workflow select toggle", help: "Toggle select mode"},
		{name: "workflow select all", help: "Select all workflows"},
		{name: "namespace create", help: "Create a namespace"},
		{name: "namespace edit", help: "Edit selected namespace"},
		{name: "namespace deprecate", help: "Deprecate selected namespace"},
		{name: "namespace delete", help: "Delete selected namespace"},
		{name: "namespace info", help: "Show namespace info"},
		{name: "namespace preview", help: "Toggle namespace preview"},
		{name: "namespace workflows", help: "Open namespace workflows"},
		{name: "namespace start", help: "Start a workflow in this namespace"},
		{name: "schedule pause", help: "Pause or unpause schedule"},
		{name: "schedule pause on", help: "Pause selected schedule"},
		{name: "schedule pause off", help: "Unpause selected schedule"},
		{name: "schedule pause toggle", help: "Toggle schedule pause"},
		{name: "schedule trigger", help: "Trigger selected schedule"},
		{name: "schedule runs", help: "Show recent schedule runs"},
		{name: "schedule delete", help: "Delete selected schedule"},
		{name: "schedule yank", help: "Copy schedule ID"},
		{name: "worker detail", help: "Show worker detail"},
		{name: "worker yank", help: "Copy worker identity"},
		{name: "poller worker", help: "Show worker for selected poller"},
		{name: "yank", help: "Copy the current ID"},
		{name: "delete", help: "Delete the current item"},
		{name: "back", help: "Go back or close a pane"},
	}
}

type commandExtras struct {
	profiles []string
	filters  []string
}

func (a *App) commandCatalog() []commandInfo {
	catalog := builtinCommandCatalog()
	if a == nil || a.config == nil {
		return catalog
	}
	cmds := a.config.GetMergedCommands(a.activeProfile)
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		help := strings.TrimSpace(cmds[name].Description)
		if help == "" {
			help = "Run " + name
		}
		catalog = append(catalog, commandInfo{name: name, help: help})
	}
	return catalog
}

func (a *App) commandExtras() commandExtras {
	extras := commandExtras{}
	if a == nil || a.config == nil {
		return extras
	}
	extras.profiles = a.config.ListProfiles()
	for _, f := range a.config.GetSavedFilters() {
		if name := strings.TrimSpace(f.Name); name != "" {
			extras.filters = append(extras.filters, name)
		}
	}
	return extras
}

func suggestionsFor(buffer string, catalog []commandInfo, extras commandExtras) []commandSuggestion {
	trimmedStart := len(buffer) - len(strings.TrimLeft(buffer, " \t"))
	body := strings.TrimLeft(buffer, " \t")
	var items []commandSuggestion
	if body == "" {
		items = commandPrefixSuggestions(catalog, "", trimmedStart)
	} else {
		items = commandPathSuggestions(catalog, extras, buffer, body, trimmedStart)
	}
	return items
}

func commandPathSuggestions(catalog []commandInfo, extras commandExtras, buffer, body string, trimmedStart int) []commandSuggestion {
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return commandPrefixSuggestions(catalog, "", trimmedStart)
	}
	if !hasTrailingWS(body) {
		partial := fields[len(fields)-1]
		complete := fields[:len(fields)-1]
		if len(complete) == 0 {
			if commandPrefixHasChildren(catalog, extras, []string{partial}) {
				return nextTokenSuggestions(catalog, extras, []string{partial}, "", len(buffer), true)
			}
			return commandPrefixSuggestions(catalog, partial, trimmedStart)
		}
		prefix := append(append([]string{}, complete...), partial)
		if commandPrefixHasChildren(catalog, extras, prefix) {
			return nextTokenSuggestions(catalog, extras, prefix, "", len(buffer), true)
		}
		return nextTokenSuggestions(catalog, extras, complete, partial, len(buffer)-len(partial), false)
	}
	return nextTokenSuggestions(catalog, extras, fields, "", len(buffer), false)
}

func hasTrailingWS(s string) bool {
	if s == "" {
		return false
	}
	return s[len(s)-1] == ' ' || s[len(s)-1] == '\t'
}

func commandPrefixHasChildren(catalog []commandInfo, extras commandExtras, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, c := range catalog {
		fields := strings.Fields(c.name)
		if len(fields) <= len(tokens) {
			continue
		}
		if commandPathPrefix(fields, tokens) {
			return true
		}
	}
	return len(extraDynamicTokens(tokens, "", 0, false, extras)) > 0
}

func commandPathPrefix(fields, tokens []string) bool {
	if len(fields) < len(tokens) {
		return false
	}
	for i, t := range tokens {
		if !strings.EqualFold(fields[i], t) {
			return false
		}
	}
	return true
}

func nextTokenSuggestions(catalog []commandInfo, extras commandExtras, complete []string, partial string, replaceFrom int, leadSpace bool) []commandSuggestion {
	partialL := strings.ToLower(partial)
	seen := map[string]bool{}
	var items []commandSuggestion
	for _, c := range catalog {
		fields := strings.Fields(c.name)
		if !commandPathPrefix(fields, complete) || len(fields) <= len(complete) {
			continue
		}
		next := fields[len(complete)]
		if partialL != "" && !strings.HasPrefix(strings.ToLower(next), partialL) {
			continue
		}
		if seen[next] {
			continue
		}
		seen[next] = true
		text := next
		if leadSpace {
			text = " " + next
		}
		items = append(items, commandSuggestion{
			Text:        text,
			Label:       next,
			Help:        nextTokenHelp(catalog, complete, next),
			ReplaceFrom: replaceFrom,
		})
	}
	for _, s := range extraDynamicTokens(complete, partial, replaceFrom, leadSpace, extras) {
		if seen[s.Label] {
			continue
		}
		seen[s.Label] = true
		items = append(items, s)
	}
	return rankTokenMatches(items, partialL)
}

func nextTokenHelp(catalog []commandInfo, complete []string, next string) string {
	name := strings.TrimSpace(strings.Join(append(append([]string{}, complete...), next), " "))
	for _, c := range catalog {
		if strings.EqualFold(c.name, name) {
			return c.help
		}
	}
	return name + " commands"
}

func commandPrefixSuggestions(catalog []commandInfo, prefix string, replaceFrom int) []commandSuggestion {
	prefixL := strings.ToLower(prefix)
	seen := map[string]bool{}
	var prefixHits, rest []commandSuggestion
	add := func(dst *[]commandSuggestion, name, help string) {
		if seen[name] {
			return
		}
		seen[name] = true
		*dst = append(*dst, commandSuggestion{
			Text:        name,
			Label:       name,
			Help:        help,
			ReplaceFrom: replaceFrom,
		})
	}
	for _, parent := range parentCommandNames(catalog) {
		low := strings.ToLower(parent)
		help := parentCommandHelp(catalog, parent)
		switch {
		case prefixL == "" || strings.HasPrefix(low, prefixL):
			add(&prefixHits, parent, help)
		case strings.Contains(low, prefixL):
			add(&rest, parent, help)
		}
	}
	return append(prefixHits, rest...)
}

func parentCommandNames(catalog []commandInfo) []string {
	seen := map[string]bool{}
	var names []string
	for _, c := range catalog {
		fields := strings.Fields(c.name)
		if len(fields) == 0 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		names = append(names, fields[0])
	}
	return names
}

func parentCommandHelp(catalog []commandInfo, parent string) string {
	for _, c := range catalog {
		if strings.EqualFold(c.name, parent) {
			return c.help
		}
	}
	return parent + " commands"
}

func extraDynamicTokens(complete []string, partial string, replaceFrom int, leadSpace bool, extras commandExtras) []commandSuggestion {
	var items []commandSuggestion
	items = append(items, extraProfileTokens(complete, partial, replaceFrom, leadSpace, extras.profiles)...)
	items = append(items, extraConfigTokens(complete, partial, replaceFrom, leadSpace)...)
	items = append(items, extraFilterTokens(complete, partial, replaceFrom, leadSpace, extras.filters)...)
	return items
}

func extraConfigTokens(complete []string, partial string, replaceFrom int, leadSpace bool) []commandSuggestion {
	var choices []string
	helpFor := func(next string) string { return next }
	if len(complete) == 2 && strings.EqualFold(complete[0], "config") {
		switch strings.ToLower(complete[1]) {
		case "get", "reset":
			choices = tempoSettingNames()
			helpFor = func(next string) string { return complete[1] + " " + next }
		case "set":
			choices = tempoSettingNames()
			helpFor = func(next string) string { return "Set " + next }
		}
	}
	if len(complete) == 3 && strings.EqualFold(complete[0], "config") && strings.EqualFold(complete[1], "set") {
		if s, ok := lookupTempoSetting(complete[2]); ok && s.values != nil {
			choices = s.values()
			helpFor = func(next string) string { return s.name + "=" + next }
		}
	}
	return tokenChoices(choices, partial, replaceFrom, leadSpace, helpFor)
}

func extraFilterTokens(complete []string, partial string, replaceFrom int, leadSpace bool, filters []string) []commandSuggestion {
	if len(complete) != 2 || !strings.EqualFold(complete[0], "filter") || !strings.EqualFold(complete[1], "load") {
		return nil
	}
	return tokenChoices(filters, partial, replaceFrom, leadSpace, func(next string) string {
		return "Load " + next
	})
}

func tokenChoices(choices []string, partial string, replaceFrom int, leadSpace bool, helpFor func(string) string) []commandSuggestion {
	partialL := strings.ToLower(partial)
	var items []commandSuggestion
	for _, next := range choices {
		if partialL != "" && !tokenMatches(next, partialL) {
			continue
		}
		text := next
		if leadSpace {
			text = " " + next
		}
		help := next
		if helpFor != nil {
			help = helpFor(next)
		}
		items = append(items, commandSuggestion{
			Text:        text,
			Label:       next,
			Help:        help,
			ReplaceFrom: replaceFrom,
		})
	}
	return items
}

func extraProfileTokens(complete []string, partial string, replaceFrom int, leadSpace bool, profiles []string) []commandSuggestion {
	if len(complete) == 0 || !strings.EqualFold(complete[0], "profile") {
		return nil
	}
	help := "Switch profile"
	switch {
	case len(complete) == 1:
	case len(complete) == 2 && strings.EqualFold(complete[1], "edit"):
		help = "Edit this profile"
	case len(complete) == 2 && strings.EqualFold(complete[1], "delete"):
		help = "Delete this profile"
	default:
		return nil
	}
	partialL := strings.ToLower(partial)
	var items []commandSuggestion
	for _, name := range profiles {
		if partialL != "" && !tokenMatches(name, partialL) {
			continue
		}
		text := name
		if leadSpace {
			text = " " + name
		}
		items = append(items, commandSuggestion{
			Text:        text,
			Label:       name,
			Help:        help,
			ReplaceFrom: replaceFrom,
		})
	}
	return items
}

func tokenMatches(value, queryLower string) bool {
	if queryLower == "" {
		return true
	}
	low := strings.ToLower(value)
	return strings.HasPrefix(low, queryLower) || strings.Contains(low, queryLower)
}

func rankTokenMatches(items []commandSuggestion, queryLower string) []commandSuggestion {
	if queryLower == "" || len(items) < 2 {
		return items
	}
	var prefix, rest []commandSuggestion
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), queryLower) {
			prefix = append(prefix, item)
		} else {
			rest = append(rest, item)
		}
	}
	return append(prefix, rest...)
}

func (c *commandCompletionState) clear() {
	if c == nil {
		return
	}
	c.items = nil
	c.selected = nil
	c.browsed = false
	c.scroll = 0
}

func (c *commandCompletionState) selectedItem() *commandSuggestion {
	if c == nil || c.selected == nil {
		return nil
	}
	i := *c.selected
	if i < 0 || i >= len(c.items) {
		return nil
	}
	return &c.items[i]
}

func (c *commandCompletionState) desiredHeight(maxH int) int {
	if c == nil || len(c.items) == 0 || maxH < 2 {
		return 0
	}
	n := len(c.items)
	if n > commandSuggestMax {
		n = commandSuggestMax
	}
	h := n + 2
	if h > maxH {
		return maxH
	}
	return h
}

func (c *commandCompletionState) step(delta int) {
	if c == nil || len(c.items) == 0 {
		c.clear()
		return
	}
	if !c.browsed || c.selected == nil {
		if delta > 0 {
			z := 0
			c.selected = &z
			c.browsed = true
			c.centerOnSelection()
		}
		return
	}
	next := *c.selected + delta
	if next < 0 {
		c.selected = nil
		c.browsed = false
		return
	}
	if next >= len(c.items) {
		next = len(c.items) - 1
	}
	c.selected = &next
	c.browsed = true
	c.centerOnSelection()
}

func (c *commandCompletionState) selectNext() {
	if c == nil || len(c.items) == 0 {
		return
	}
	if c.selected == nil {
		z := 0
		c.selected = &z
	} else {
		n := (*c.selected + 1) % len(c.items)
		c.selected = &n
	}
	c.centerOnSelection()
}

func (c *commandCompletionState) selectPrev() {
	if c == nil || len(c.items) == 0 {
		return
	}
	if c.selected == nil || *c.selected == 0 {
		n := len(c.items) - 1
		c.selected = &n
	} else {
		n := *c.selected - 1
		c.selected = &n
	}
	c.centerOnSelection()
}

func (c *commandCompletionState) ensureVisible(viewport int) {
	if c == nil {
		return
	}
	if viewport < 1 {
		viewport = 1
	}
	if len(c.items) == 0 {
		c.scroll = 0
		return
	}
	if c.selected == nil {
		maxStart := max(0, len(c.items)-viewport)
		if c.scroll > maxStart {
			c.scroll = maxStart
		}
		return
	}
	sel := *c.selected
	if sel < c.scroll {
		c.scroll = sel
	} else if sel >= c.scroll+viewport {
		c.scroll = sel + 1 - viewport
	}
	maxStart := max(0, len(c.items)-viewport)
	if c.scroll > maxStart {
		c.scroll = maxStart
	}
}

func (c *commandCompletionState) centerOnSelection() {
	if c == nil || c.selected == nil || len(c.items) == 0 {
		return
	}
	viewport := commandSuggestMax
	c.scroll = max(0, *c.selected-viewport/2)
	maxStart := max(0, len(c.items)-viewport)
	if c.scroll > maxStart {
		c.scroll = maxStart
	}
}
