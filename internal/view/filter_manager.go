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
				table.AddRow(f.Name, truncate(savedFilterSummary(f), 52))
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

	createFilter := func() {
		wl.openFilterBuilder(&filterBuilderState{
			wl:             wl,
			persistOnApply: true,
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
			clauses:        append([]config.FilterClause(nil), f.Clauses...),
			name:           f.Name,
			rawQuery:       f.Query,
			persistOnApply: true,
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
		return event
	})

	refresh()

	modal := newOverlayModal(components.ModalConfig{
		Title:  fmt.Sprintf("%s Filters", theme.IconFilter),
		Width:  76,
		Height: 22,
	}, wl)
	modal.SetContent(table)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Apply"},
		{Key: "n", Description: "New Filter"},
		{Key: "e", Description: "Edit"},
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
