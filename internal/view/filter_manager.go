package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	filterAndGlyph = "∧"
	filterOrGlyph  = "∨"
)

// copyFilterName suggests a free name for a copy of base, counting up until
// it finds one no filter has taken.
func copyFilterName(cfg *config.Config, profile, base string) string {
	name := base + " (copy)"
	for i := 2; ; i++ {
		if _, taken := cfg.SavedFilterFor(profile, name); !taken {
			return name
		}
		name = fmt.Sprintf("%s (copy %d)", base, i)
	}
}

// filterTreeRow is one line of the Filters dialog: a saved filter, or a
// clause or group inside one. path locates node from the filter's root.
type filterTreeRow struct {
	filter int
	node   *filterNode
	path   []int
	text   string
}

func (r filterTreeRow) isFilter() bool {
	return r.node == nil
}

// filterTreeRows lays out the filters profile sees. Global filters are only
// marked as such when there is a profile for them to stand apart from.
func filterTreeRows(wl *WorkflowList, profile string, filters []config.SavedFilter) []filterTreeRow {
	var rows []filterTreeRow
	for i, f := range filters {
		text := "[::b]" + tview.Escape(f.Name) + "[::-]"
		if profile != "" && f.Profile == "" {
			text += " [" + theme.TagFgDim() + "]· global[-]"
		}
		rows = append(rows, filterTreeRow{filter: i, text: text})
		root := parseFilterTree(compiledFilterQueryFor(wl, f))
		if root == nil {
			continue
		}
		if root.isLeaf() {
			rows = append(rows, filterTreeNodeRow(i, root, []int{}, filterGroupAnd, []bool{true}))
			continue
		}
		rows = appendFilterTreeChildren(rows, i, root, nil, nil)
	}
	return rows
}

func appendFilterTreeChildren(rows []filterTreeRow, filter int, group *filterNode, path []int, lastAtLevel []bool) []filterTreeRow {
	for i, child := range group.children {
		childPath := append(append([]int(nil), path...), i)
		childLast := append(append([]bool(nil), lastAtLevel...), i == len(group.children)-1)
		rows = append(rows, filterTreeNodeRow(filter, child, childPath, group.op, childLast))
		if !child.isLeaf() {
			rows = appendFilterTreeChildren(rows, filter, child, childPath, childLast)
		}
	}
	return rows
}

// filterTreeNodeRow renders a node with the glyph of the operator that joins
// it to its siblings, so every row shows whether it is ANDed or ORed in.
func filterTreeNodeRow(filter int, node *filterNode, path []int, joinedBy string, lastAtLevel []bool) filterTreeRow {
	prefix := "[" + theme.TagFgDim() + "]" + workflowTreePrefix(lastAtLevel) + "[-]"
	return filterTreeRow{
		filter: filter,
		node:   node,
		path:   path,
		text:   prefix + filterJoinGlyph(joinedBy) + " " + filterNodeLabel(node),
	}
}

func filterJoinGlyph(op string) string {
	if op == filterGroupOr {
		return "[" + theme.TagWarning() + "]" + filterOrGlyph + "[-]"
	}
	return "[" + theme.TagAccent() + "]" + filterAndGlyph + "[-]"
}

func filterNodeLabel(node *filterNode) string {
	switch {
	case node.op == filterGroupAnd:
		return "[" + theme.TagFgDim() + "]all of[-]"
	case node.op == filterGroupOr:
		return "[" + theme.TagFgDim() + "]any of[-]"
	case isRawFilterClause(node.clause):
		return tview.Escape(strings.TrimSpace(node.clause.Value))
	default:
		return tview.Escape(filterClauseSummary(node.clause))
	}
}

func (wl *WorkflowList) showFilterManager() {
	if wl == nil || wl.app == nil {
		return
	}
	cfg := wl.app.Config()
	if cfg == nil {
		wl.app.ToastWarning("No saved filters")
		return
	}

	profile := wl.app.activeProfile
	filters := func() []config.SavedFilter {
		return cfg.SavedFiltersFor(profile)
	}

	table := components.NewTable()
	table.SetBorder(false)

	var rows []filterTreeRow

	filterRow := func(idx int) int {
		for i, row := range rows {
			if row.filter == idx && row.isFilter() {
				return i
			}
		}
		return -1
	}

	refresh := func() {
		row := table.SelectedRow()
		table.ClearRows()
		table.SetHeaders("FILTER")
		rows = filterTreeRows(wl, profile, filters())
		if len(rows) == 0 {
			table.AddRow("Press n to create a filter")
		}
		for _, r := range rows {
			table.AddRow(r.text)
		}
		if row < 0 {
			row = 0
		}
		if n := table.RowCount(); row >= n {
			row = n - 1
		}
		if row >= 0 {
			table.SelectRow(row)
		}
	}

	selectFilter := func(idx int) {
		if row := filterRow(idx); row >= 0 {
			table.SelectRow(row)
		}
	}

	selectFilterNamed := func(name string) {
		for i, f := range filters() {
			if f.Name == name {
				selectFilter(i)
				return
			}
		}
	}

	selectedRow := func() (filterTreeRow, bool) {
		row := table.SelectedRow()
		if row < 0 || row >= len(rows) {
			return filterTreeRow{}, false
		}
		return rows[row], true
	}

	selectedFilter := func() (int, bool) {
		row, ok := selectedRow()
		if !ok {
			return -1, false
		}
		return row.filter, true
	}

	applySelected := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		wl.closeModal()
		wl.applySavedFilter(filters()[idx])
	}

	// A new filter always needs at least one clause, so go straight to the
	// clause editor and open the builder around whatever comes back.
	createFilter := func() {
		wl.showClauseEditor(newFilterClause(), func(clauses []config.FilterClause) {
			wl.openFilterBuilder(&filterBuilderState{
				wl:             wl,
				clauses:        clauses,
				persistOnApply: true,
				onSaved:        refresh,
			})
		})
	}

	editFilter := func(idx int) {
		f := filters()[idx]
		wl.openFilterBuilder(&filterBuilderState{
			wl:             wl,
			clauses:        savedFilterClauses(f),
			name:           f.Name,
			persistOnApply: true,
			onSaved:        refresh,
		})
	}

	// rewriteFilter swaps the node at path for with, or drops it when with is
	// nil, and saves the recompiled query in place.
	rewriteFilter := func(idx int, path []int, with *filterNode) {
		f := filters()[idx]
		root := parseFilterTree(compiledFilterQueryFor(wl, f)).replaced(path, with)
		if root == nil {
			wl.app.ToastWarning("A filter needs at least one clause")
			return
		}
		f.Query = root.query(wl)
		f.Clauses = nil
		wl.persistSavedFilter(f)
		if wl.activeFilterName == f.Name {
			wl.applySavedFilter(f)
		}
		row := table.SelectedRow()
		refresh()
		if row >= len(rows) {
			row = len(rows) - 1
		}
		table.SelectRow(row)
	}

	editSelected := func() {
		row, ok := selectedRow()
		if !ok {
			createFilter()
			return
		}
		if row.isFilter() || !row.node.isLeaf() {
			editFilter(row.filter)
			return
		}
		wl.showClauseEditor(row.node.clause, func(clauses []config.FilterClause) {
			rewriteFilter(row.filter, row.path, parseFilterTree(compileFilterClausesFor(wl, clauses)))
		})
	}

	renameFilter := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		f := filters()[idx]
		wl.showFilterNamePrompt("Rename Filter", f.Name, func(name string) {
			if err := cfg.RenameFilterFor(profile, f.Name, name); err != nil {
				wl.app.ToastWarning(err.Error())
				return
			}
			_ = wl.app.SaveConfig()
			if strings.EqualFold(wl.activeFilterName, f.Name) {
				wl.activeFilterName = name
			}
			refresh()
			selectFilter(idx)
			if name != f.Name {
				wl.app.ToastSuccess("Renamed filter " + name)
			}
		})
	}

	// A clone lands right below its original, in the same scope, under a
	// name of its own. It carries the query over verbatim, placeholders and
	// all.
	cloneFilter := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		visible := filters()
		f := visible[idx]
		wl.showFilterNamePrompt("Clone Filter", copyFilterName(cfg, profile, f.Name), func(name string) {
			if _, taken := cfg.SavedFilterFor(profile, name); taken {
				wl.app.ToastWarning("A filter named " + name + " already exists")
				return
			}
			pos := 0
			for _, other := range visible[:idx] {
				if other.Profile == f.Profile {
					pos++
				}
			}
			cfg.InsertFilter(f.Profile, pos+1, config.SavedFilter{Name: name, Query: compiledFilterQueryFor(wl, f)})
			_ = wl.app.SaveConfig()
			refresh()
			selectFilter(idx + 1)
			wl.app.ToastSuccess("Cloned filter " + name)
		})
	}

	deleteFilter := func(idx int) {
		name := filters()[idx].Name
		if err := cfg.DeleteFilterFor(profile, name); err != nil {
			wl.app.ToastWarning(err.Error())
			return
		}
		_ = wl.app.SaveConfig()
		if strings.EqualFold(wl.activeFilterName, name) {
			wl.applyAllWorkflowsFilter()
		}
		refresh()
		if n := len(filters()); n > 0 {
			selectFilter(min(idx, n-1))
		}
	}

	deleteSelected := func() {
		row, ok := selectedRow()
		if !ok {
			return
		}
		if row.isFilter() {
			deleteFilter(row.filter)
			return
		}
		rewriteFilter(row.filter, row.path, nil)
	}

	move := func(delta int) {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		name := filters()[idx].Name
		cfg.MoveSavedFilterFor(profile, idx, idx+delta)
		_ = wl.app.SaveConfig()
		refresh()
		selectFilterNamed(name)
	}

	toggleScope := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		if profile == "" {
			wl.app.ToastWarning("No active profile to scope filters to")
			return
		}
		f := filters()[idx]
		global := f.Profile != ""
		if err := cfg.SetFilterScope(profile, f.Name, global); err != nil {
			wl.app.ToastWarning(err.Error())
			return
		}
		_ = wl.app.SaveConfig()
		refresh()
		selectFilterNamed(f.Name)
		if global {
			wl.app.ToastSuccess("Filter " + f.Name + " is now shared by every profile")
		} else {
			wl.app.ToastSuccess("Filter " + f.Name + " now belongs to " + profile)
		}
	}

	bindings := input.NewKeyBindings().
		OnRune('n', func(e *tcell.EventKey) bool {
			createFilter()
			return true
		}).
		OnRune('e', func(e *tcell.EventKey) bool {
			editSelected()
			return true
		}).
		OnRune('r', func(e *tcell.EventKey) bool {
			renameFilter()
			return true
		}).
		OnRune('c', func(e *tcell.EventKey) bool {
			cloneFilter()
			return true
		}).
		OnRune('d', func(e *tcell.EventKey) bool {
			deleteSelected()
			return true
		}).
		OnRune('g', func(e *tcell.EventKey) bool {
			toggleScope()
			return true
		}).
		OnRune('J', func(e *tcell.EventKey) bool {
			move(1)
			return true
		}).
		OnRune('K', func(e *tcell.EventKey) bool {
			move(-1)
			return true
		})

	refresh()
	if wl.activeFilterName != "" {
		selectFilterNamed(wl.activeFilterName)
	}

	scroll := attachTableCharScroll(table, wl.app)
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			applySelected()
			return nil
		}
		if event.Key() == tcell.KeyUp && event.Modifiers()&tcell.ModCtrl != 0 {
			move(-1)
			return nil
		}
		if event.Key() == tcell.KeyDown && event.Modifiers()&tcell.ModCtrl != 0 {
			move(1)
			return nil
		}
		if bindings.Handle(event) {
			return nil
		}
		if handleTableCharScroll(scroll, table, event) {
			return nil
		}
		return event
	})

	title := fmt.Sprintf("%s Filters", theme.IconFilter)
	if profile != "" {
		title += " · " + profile
	}
	modal := newOverlayModal(components.ModalConfig{
		Title:  title,
		Width:  76,
		Height: 22,
	}, wl)
	modal.SetContent(scroll)
	hints := []components.KeyHint{
		{Key: "Enter", Description: "Apply"},
		{Key: "n", Description: "New Filter"},
		{Key: "e", Description: "Edit"},
		{Key: "r", Description: "Rename"},
		{Key: "c", Description: "Clone"},
		{Key: "d", Description: "Delete"},
	}
	if profile != "" {
		hints = append(hints, components.KeyHint{Key: "g", Description: "Global/Profile"})
	}
	hints = append(hints,
		components.KeyHint{Key: "J/K", Description: "Reorder"},
		components.KeyHint{Key: "Esc", Description: "Close"},
	)
	modal.SetHints(hints)
	modal.SetOnCancel(func() {
		wl.closeModal()
	})
	wl.app.PushModal(modal)
	wl.app.JigApp().SetFocus(table)
}
