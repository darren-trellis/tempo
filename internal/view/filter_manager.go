package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
)

// copyFilterName suggests a free name for a copy of base, counting up until
// it finds one no filter has taken.
func copyFilterName(cfg *config.Config, base string) string {
	name := base + " (copy)"
	for i := 2; ; i++ {
		if _, taken := cfg.GetSavedFilter(name); !taken {
			return name
		}
		name = fmt.Sprintf("%s (copy %d)", base, i)
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

	table := components.NewTable()
	table.SetBorder(false)

	refresh := func() {
		row := table.SelectedRow()
		table.ClearRows()
		table.SetHeaders("NAME", "FILTER")
		filters := cfg.GetSavedFilters()
		if len(filters) == 0 {
			table.AddRow("(none)", "Press n to create a filter")
		} else {
			for _, f := range filters {
				table.AddRow(f.Name, savedFilterSummary(f))
			}
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

	selectedFilter := func() (int, bool) {
		filters := cfg.GetSavedFilters()
		row := table.SelectedRow()
		if row < 0 || row >= len(filters) {
			return -1, false
		}
		return row, true
	}

	applySelected := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		wl.closeModal()
		wl.applySavedFilter(cfg.GetSavedFilters()[idx])
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

	editFilter := func() {
		idx, ok := selectedFilter()
		if !ok {
			createFilter()
			return
		}
		f := cfg.GetSavedFilters()[idx]
		wl.openFilterBuilder(&filterBuilderState{
			wl:             wl,
			clauses:        savedFilterClauses(f),
			name:           f.Name,
			persistOnApply: true,
			onSaved:        refresh,
		})
	}

	renameFilter := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		f := cfg.GetSavedFilters()[idx]
		wl.showFilterNamePrompt("Rename Filter", f.Name, func(name string) {
			if err := cfg.RenameFilter(f.Name, name); err != nil {
				wl.app.ToastWarning(err.Error())
				return
			}
			_ = wl.app.SaveConfig()
			if strings.EqualFold(wl.activeFilterName, f.Name) {
				wl.activeFilterName = name
			}
			refresh()
			table.SelectRow(idx)
			if name != f.Name {
				wl.app.ToastSuccess("Renamed filter " + name)
			}
		})
	}

	// A clone lands right below its original under a name of its own. It
	// carries the query over verbatim, placeholders and all, but never
	// IsDefault: only one filter can be the default and the copy is not it.
	cloneFilter := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		f := cfg.GetSavedFilters()[idx]
		wl.showFilterNamePrompt("Clone Filter", copyFilterName(cfg, f.Name), func(name string) {
			if _, taken := cfg.GetSavedFilter(name); taken {
				wl.app.ToastWarning("A filter named " + name + " already exists")
				return
			}
			cfg.SaveFilter(config.SavedFilter{Name: name, Query: compiledFilterQueryFor(wl, f)})
			cfg.MoveSavedFilter(len(cfg.GetSavedFilters())-1, idx+1)
			_ = wl.app.SaveConfig()
			refresh()
			table.SelectRow(idx + 1)
			wl.app.ToastSuccess("Cloned filter " + name)
		})
	}

	deleteFilter := func() {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		f := cfg.GetSavedFilters()[idx]
		name := f.Name
		if err := cfg.DeleteFilter(name); err != nil {
			wl.app.ToastWarning(err.Error())
			return
		}
		_ = wl.app.SaveConfig()
		if strings.EqualFold(wl.activeFilterName, name) {
			wl.applyAllWorkflowsFilter()
		}
		refresh()
	}

	move := func(delta int) {
		idx, ok := selectedFilter()
		if !ok {
			return
		}
		next := idx + delta
		cfg.MoveSavedFilter(idx, next)
		_ = wl.app.SaveConfig()
		refresh()
		filters := cfg.GetSavedFilters()
		if next < 0 {
			next = 0
		}
		if next >= len(filters) {
			next = len(filters) - 1
		}
		if next >= 0 {
			table.SelectRow(next)
		}
	}

	bindings := input.NewKeyBindings().
		OnRune('n', func(e *tcell.EventKey) bool {
			createFilter()
			return true
		}).
		OnRune('e', func(e *tcell.EventKey) bool {
			editFilter()
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
			deleteFilter()
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
	for i, f := range cfg.GetSavedFilters() {
		if wl.activeFilterName != "" && f.Name == wl.activeFilterName {
			table.SelectRow(i)
			break
		}
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

	modal := newOverlayModal(components.ModalConfig{
		Title:  fmt.Sprintf("%s Filters", theme.IconFilter),
		Width:  76,
		Height: 22,
	}, wl)
	modal.SetContent(scroll)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Apply"},
		{Key: "n", Description: "New Filter"},
		{Key: "e", Description: "Edit"},
		{Key: "r", Description: "Rename"},
		{Key: "c", Description: "Clone"},
		{Key: "d", Description: "Delete"},
		{Key: "J/K", Description: "Reorder"},
		{Key: "Esc", Description: "Close"},
	})
	modal.SetOnCancel(func() {
		wl.closeModal()
	})
	wl.app.PushModal(modal)
	wl.app.JigApp().SetFocus(table)
}
