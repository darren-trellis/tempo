package config

import (
	"fmt"
	"strings"
)

// EnsureSavedFilters seeds the built-in filters into a config that has no
// saved filters anywhere.
func (c *Config) EnsureSavedFilters() {
	if c == nil || c.SavedFilters != nil || len(c.ProfileFilters) > 0 {
		return
	}
	c.SavedFilters = DefaultSavedFilters()
}

func DefaultSavedFilters() []SavedFilter {
	filter := func(name, query string) SavedFilter {
		return SavedFilter{Name: name, Query: query}
	}
	status := func(name, value string) SavedFilter {
		return filter(name, "ExecutionStatus = '"+value+"'")
	}
	started := func(name, value string) SavedFilter {
		return filter(name, "StartTime > "+value)
	}
	return []SavedFilter{
		status("Running Workflows", "Running"),
		filter("Unhandled Failures", "`ExecutionStatus`=\"Running\" AND `TemporalReportedProblems` IN (\"category=WorkflowTaskFailed\", \"category=WorkflowTaskTimedOut\")"),
		status("Failed Workflows", "Failed"),
		status("Completed Workflows", "Completed"),
		status("Cancelled Workflows", "Canceled"),
		status("Timed Out Workflows", "TimedOut"),
		started("Started Today", "$TODAY"),
		filter("Started Yesterday", "StartTime > $YESTERDAY AND StartTime < $TODAY"),
		started("Started This Week", "$THIS_WEEK"),
		started("Started Last Hour", "$HOUR_AGO"),
		started("Started Last 30 Min", "$MINUTES_AGO_30"),
		started("Started Last 24 Hours", "$HOURS_AGO_24"),
		started("Started Last 7 Days", "$DAYS_AGO_7"),
		started("Started Last 30 Days", "$DAYS_AGO_30"),
		filter("Long Running (>1h)", "ExecutionStatus = 'Running' AND StartTime < $HOUR_AGO"),
		filter("Long Running (>6h)", "ExecutionStatus = 'Running' AND StartTime < $HOURS_AGO_6"),
		filter("Failed Today", "ExecutionStatus = 'Failed' AND StartTime > $TODAY"),
	}
}

// filterScopes lists the lists a profile sees, its own before the global one.
func filterScopes(profile string) []string {
	if profile == "" {
		return []string{""}
	}
	return []string{profile, ""}
}

func (c *Config) filterList(scope string) []SavedFilter {
	if scope == "" {
		return c.SavedFilters
	}
	return c.ProfileFilters[scope]
}

func (c *Config) setFilterList(scope string, list []SavedFilter) {
	for i := range list {
		list[i].Profile = ""
	}
	if scope == "" {
		if list == nil {
			list = []SavedFilter{}
		}
		c.SavedFilters = list
		return
	}
	if len(list) == 0 {
		delete(c.ProfileFilters, scope)
		return
	}
	if c.ProfileFilters == nil {
		c.ProfileFilters = make(map[string][]SavedFilter)
	}
	c.ProfileFilters[scope] = list
}

func (c *Config) findFilter(profile, name string) (scope string, idx int, ok bool) {
	for _, scope := range filterScopes(profile) {
		for i, f := range c.filterList(scope) {
			if f.Name == name {
				return scope, i, true
			}
		}
	}
	return "", -1, false
}

// SavedFiltersFor lists the filters a profile sees: its own, then the global
// ones, each with Profile set to its owner.
func (c *Config) SavedFiltersFor(profile string) []SavedFilter {
	if c == nil {
		return nil
	}
	var out []SavedFilter
	for _, scope := range filterScopes(profile) {
		for _, f := range c.filterList(scope) {
			f.Profile = scope
			out = append(out, f)
		}
	}
	return out
}

// SavedFilterFor returns a filter the profile sees by name, preferring the
// profile's own over a global one of the same name.
func (c *Config) SavedFilterFor(profile, name string) (SavedFilter, bool) {
	if c == nil {
		return SavedFilter{}, false
	}
	scope, idx, ok := c.findFilter(profile, name)
	if !ok {
		return SavedFilter{}, false
	}
	f := c.filterList(scope)[idx]
	f.Profile = scope
	return f, true
}

// SaveFilterFor replaces the filter of the same name the profile sees, or
// adds a new one to the profile's own list.
func (c *Config) SaveFilterFor(profile string, filter SavedFilter) {
	if c == nil {
		return
	}
	if scope, idx, ok := c.findFilter(profile, filter.Name); ok {
		list := c.filterList(scope)
		list[idx] = filter
		c.setFilterList(scope, list)
		return
	}
	c.setFilterList(profile, append(c.filterList(profile), filter))
}

// InsertFilter adds a filter to one scope's list at index, where scope is a
// profile name or empty for the global list.
func (c *Config) InsertFilter(scope string, index int, filter SavedFilter) {
	if c == nil {
		return
	}
	list := c.filterList(scope)
	index = max(0, min(index, len(list)))
	out := make([]SavedFilter, 0, len(list)+1)
	out = append(out, list[:index]...)
	out = append(out, filter)
	out = append(out, list[index:]...)
	c.setFilterList(scope, out)
}

// RenameFilterFor changes a filter's name in place, keeping its query,
// scope, and position. Renaming to the current name is a no-op.
func (c *Config) RenameFilterFor(profile, oldName, newName string) error {
	if c == nil {
		return fmt.Errorf("filter %q not found", oldName)
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("name is required")
	}
	scope, idx, ok := c.findFilter(profile, oldName)
	if !ok {
		return fmt.Errorf("filter %q not found", oldName)
	}
	if newName == oldName {
		return nil
	}
	if _, _, taken := c.findFilter(profile, newName); taken {
		return fmt.Errorf("a filter named %q already exists", newName)
	}
	list := c.filterList(scope)
	list[idx].Name = newName
	c.setFilterList(scope, list)
	return nil
}

// DeleteFilterFor removes a filter the profile sees by name.
func (c *Config) DeleteFilterFor(profile, name string) error {
	if c == nil {
		return fmt.Errorf("filter %q not found", name)
	}
	scope, idx, ok := c.findFilter(profile, name)
	if !ok {
		return fmt.Errorf("filter %q not found", name)
	}
	list := c.filterList(scope)
	c.setFilterList(scope, append(list[:idx:idx], list[idx+1:]...))
	return nil
}

// MoveSavedFilterFor moves a filter within the list SavedFiltersFor returns.
// A filter only moves among the filters of its own scope.
func (c *Config) MoveSavedFilterFor(profile string, from, to int) {
	if c == nil {
		return
	}
	visible := c.SavedFiltersFor(profile)
	if from < 0 || from >= len(visible) || to < 0 || to >= len(visible) || from == to {
		return
	}
	scope := visible[from].Profile
	if visible[to].Profile != scope {
		return
	}
	offset := 0
	if scope == "" && profile != "" {
		offset = len(c.filterList(profile))
	}
	list := c.filterList(scope)
	item := list[from-offset]
	list = append(list[:from-offset], list[from-offset+1:]...)
	list = append(list[:to-offset], append([]SavedFilter{item}, list[to-offset:]...)...)
	c.setFilterList(scope, list)
}

// SetFilterScope moves a filter between the profile's own list and the
// global list, appending it to the end of the one it joins.
func (c *Config) SetFilterScope(profile, name string, global bool) error {
	if c == nil {
		return fmt.Errorf("filter %q not found", name)
	}
	target := profile
	if global {
		target = ""
	}
	scope, idx, ok := c.findFilter(profile, name)
	if !ok {
		return fmt.Errorf("filter %q not found", name)
	}
	if scope == target {
		return nil
	}
	list := c.filterList(scope)
	f := list[idx]
	c.setFilterList(scope, append(list[:idx:idx], list[idx+1:]...))
	c.setFilterList(target, append(c.filterList(target), f))
	return nil
}
