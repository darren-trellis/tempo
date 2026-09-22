package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// jsonTreeSelection turns a text view into a navigable JSON tree: it owns the
// rendered rows so it can move a highlight, fold nodes, and hand back the JSON
// under the cursor.
type jsonTreeSelection struct {
	view     *tview.TextView
	rows     []jsonTreeRow
	content  string
	folded   map[string]bool
	selected int
}

func newJSONTreeSelection(view *tview.TextView) *jsonTreeSelection {
	selection := &jsonTreeSelection{view: view}
	if view == nil {
		return selection
	}
	prev := view.GetMouseCapture()
	view.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick && selection.clickRow(event) {
			return tview.MouseConsumed, nil
		}
		if prev != nil {
			return prev(action, event)
		}
		return action, event
	})
	return selection
}

// setContent renders content as a tree and reports whether it could. Folds and
// the highlight are kept when the same payload is rendered again, so a preview
// refresh does not undo the reader's place.
func (s *jsonTreeSelection) setContent(content string, enabled bool) bool {
	if s == nil || s.view == nil {
		return false
	}
	fresh := content != s.content
	if fresh {
		s.content = content
		s.folded = nil
		s.selected = 0
	}
	s.rows = nil
	if !enabled {
		return false
	}
	rows, ok := buildJSONTreeRows(content, s.folded)
	if !ok || len(rows) == 0 {
		return false
	}
	s.rows = rows
	if s.selected >= len(rows) {
		s.selected = len(rows) - 1
	}
	s.render(fresh)
	return true
}

func (s *jsonTreeSelection) handleKey(event *tcell.EventKey) bool {
	if !s.active() || event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyUp:
		s.selectRow(s.selected - 1)
		return true
	case tcell.KeyDown:
		s.selectRow(s.selected + 1)
		return true
	case tcell.KeyHome:
		s.selectRow(0)
		return true
	case tcell.KeyEnd:
		s.selectRow(len(s.rows) - 1)
		return true
	case tcell.KeyEnter:
		s.toggleFold()
		return true
	case tcell.KeyRune:
		switch event.Rune() {
		case 'k':
			s.selectRow(s.selected - 1)
			return true
		case 'j':
			s.selectRow(s.selected + 1)
			return true
		case ' ':
			s.toggleFold()
			return true
		}
	}
	return false
}

func (s *jsonTreeSelection) toggleFold() {
	row, ok := s.selectedRow()
	if !ok || !row.foldable {
		return
	}
	if s.folded == nil {
		s.folded = make(map[string]bool)
	}
	if s.folded[row.path] {
		delete(s.folded, row.path)
	} else {
		s.folded[row.path] = true
	}
	rows, built := buildJSONTreeRows(s.content, s.folded)
	if !built || len(rows) == 0 {
		return
	}
	s.rows = rows
	// Folding a node removes its children, so follow the path back to wherever
	// the cursor's row ended up.
	if index, found := s.indexOfPath(row.path); found {
		s.selected = index
	} else if s.selected >= len(rows) {
		s.selected = len(rows) - 1
	}
	s.render(false)
}

func (s *jsonTreeSelection) selectRow(index int) {
	if !s.active() {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(s.rows) {
		index = len(s.rows) - 1
	}
	if index == s.selected {
		return
	}
	s.selected = index
	s.render(false)
}

func (s *jsonTreeSelection) clickRow(event *tcell.EventMouse) bool {
	if !s.active() || event == nil {
		return false
	}
	x, y := event.Position()
	left, top, width, height := s.view.GetInnerRect()
	if x < left || x >= left+width || y < top || y >= top+height {
		return false
	}
	offset, _ := s.view.GetScrollOffset()
	index := offset + y - top
	if index < 0 || index >= len(s.rows) {
		return false
	}
	s.selectRow(index)
	return true
}

// render redraws every row because the highlight is baked into the text: tview
// only inverts the styled runes of a region, which left the tree striped in as
// many colors as the row had tags.
func (s *jsonTreeSelection) render(toTop bool) {
	offset, column := s.view.GetScrollOffset()
	if toTop {
		offset, column = 0, 0
	}
	_, _, innerWidth, innerHeight := s.view.GetInnerRect()
	width := innerWidth
	for _, row := range s.rows {
		if w := jsonTreeRowWidth(row.text); w > width {
			width = w
		}
	}
	lines := make([]string, len(s.rows))
	for i, row := range s.rows {
		if i == s.selected {
			lines[i] = jsonTreeHighlightedRow(row.text, width)
			continue
		}
		lines[i] = row.text
	}
	s.view.SetText(strings.Join(lines, "\n"))

	if innerHeight > 0 {
		if s.selected < offset {
			offset = s.selected
		}
		if s.selected >= offset+innerHeight {
			offset = s.selected - innerHeight + 1
		}
	}
	if offset < 0 {
		offset = 0
	}
	s.view.ScrollTo(offset, column)
}

// jsonTreeHighlightedRow paints the whole row in the selection colors the
// tables use, padded so the bar runs the full width rather than stopping at the
// end of the text.
func jsonTreeHighlightedRow(text string, width int) string {
	plain := stripStyleTags(text)
	if pad := width - tview.TaggedStringWidth(plain); pad > 0 {
		plain += strings.Repeat(" ", pad)
	}
	return fmt.Sprintf("[%s:%s:b]%s[-:-:-]", theme.ColorToHex(accentTextColor()), theme.TagAccent(), plain)
}

// jsonTreeRowWidth measures a row as it appears on screen. The tags have to go
// before measuring: a theme that yields an unparseable color leaves the tag
// counted as text, which would pad the highlight to the wrong width.
func jsonTreeRowWidth(text string) int {
	return tview.TaggedStringWidth(stripStyleTags(text))
}

// stripStyleTags drops tview style tags. Content brackets are already escaped
// to their full-width form, so every remaining bracket pair is a tag.
func stripStyleTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for {
		open := strings.IndexByte(s, '[')
		if open < 0 {
			b.WriteString(s)
			return b.String()
		}
		close := strings.IndexByte(s[open:], ']')
		if close < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:open])
		s = s[open+close+1:]
	}
}

func (s *jsonTreeSelection) indexOfPath(path string) (int, bool) {
	for i, row := range s.rows {
		if row.path == path {
			return i, true
		}
	}
	return 0, false
}

func (s *jsonTreeSelection) selectedRow() (jsonTreeRow, bool) {
	if !s.active() || s.selected < 0 || s.selected >= len(s.rows) {
		return jsonTreeRow{}, false
	}
	return s.rows[s.selected], true
}

func (s *jsonTreeSelection) value() (string, bool) {
	row, ok := s.selectedRow()
	if !ok {
		return "", false
	}
	return row.value, true
}

func (s *jsonTreeSelection) active() bool {
	return s != nil && s.view != nil && len(s.rows) > 0
}
