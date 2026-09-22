package view

import (
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const jsonTreeRegionPrefix = "json-tree-"

type jsonTreeSelection struct {
	view     *tview.TextView
	rows     []jsonTreeRow
	selected int
}

func newJSONTreeSelection(view *tview.TextView) *jsonTreeSelection {
	selection := &jsonTreeSelection{view: view}
	if view != nil {
		view.SetHighlightedFunc(func(added, _, _ []string) {
			for _, id := range added {
				if index, ok := jsonTreeRegionIndex(id); ok {
					selection.selected = index
					return
				}
			}
		})
	}
	return selection
}

func (s *jsonTreeSelection) setContent(content string, enabled bool) bool {
	if s == nil || s.view == nil {
		return false
	}
	s.rows = nil
	s.selected = 0
	s.view.SetRegions(false).Highlight()
	if !enabled {
		return false
	}
	rows, ok := buildJSONTreeRows(content)
	if !ok || len(rows) == 0 {
		return false
	}
	s.rows = rows
	lines := make([]string, len(rows))
	for i, row := range rows {
		id := jsonTreeRegionID(i)
		lines[i] = `["` + id + `"]` + row.text + `[""]`
	}
	s.view.SetRegions(true).SetText(strings.Join(lines, "\n"))
	s.selectRow(0)
	return true
}

func (s *jsonTreeSelection) handleKey(event *tcell.EventKey) bool {
	if s == nil || len(s.rows) == 0 || event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyUp:
		s.move(-1)
		return true
	case tcell.KeyDown:
		s.move(1)
		return true
	case tcell.KeyHome:
		s.selectRow(0)
		return true
	case tcell.KeyEnd:
		s.selectRow(len(s.rows) - 1)
		return true
	case tcell.KeyRune:
		switch event.Rune() {
		case 'k':
			s.move(-1)
			return true
		case 'j':
			s.move(1)
			return true
		}
	}
	return false
}

func (s *jsonTreeSelection) move(delta int) {
	s.selectRow(s.selected + delta)
}

func (s *jsonTreeSelection) selectRow(index int) {
	if s == nil || s.view == nil || len(s.rows) == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(s.rows) {
		index = len(s.rows) - 1
	}
	s.selected = index
	s.view.Highlight(jsonTreeRegionID(index)).ScrollToHighlight()
}

func (s *jsonTreeSelection) value() (string, bool) {
	if s == nil || s.selected < 0 || s.selected >= len(s.rows) {
		return "", false
	}
	return s.rows[s.selected].value, true
}

func (s *jsonTreeSelection) active() bool {
	return s != nil && len(s.rows) > 0
}

func jsonTreeRegionID(index int) string {
	return jsonTreeRegionPrefix + strconv.Itoa(index)
}

func jsonTreeRegionIndex(id string) (int, bool) {
	if !strings.HasPrefix(id, jsonTreeRegionPrefix) {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(id, jsonTreeRegionPrefix))
	return index, err == nil
}
