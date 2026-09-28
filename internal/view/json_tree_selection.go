package view

import (
	"strings"
	"unicode/utf8"

	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// jsonTreeSelection turns a text view into a navigable JSON tree: it owns the
// rendered rows so it can move a highlight, fold nodes, and hand back the JSON
// under the cursor.
type jsonTreeSelection struct {
	view       *tview.TextView
	rows       []jsonTreeRow
	content    string
	folded     map[string]bool
	selected   int
	visual     []int // visual line where each logical row starts
	visualRows int
	laidWidth  int
	text       string
	rowsWidth  int
	wrapped    [][]string
	wrapWidth  int
}

func newJSONTreeSelection(view *tview.TextView) *jsonTreeSelection {
	selection := &jsonTreeSelection{view: view}
	if view == nil {
		return selection
	}
	setTextViewSize(view, func() (int, int, bool) {
		if !selection.active() {
			return 0, 0, false
		}
		return selection.visualRows, selection.rowsWidth, true
	})
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
	prevDraw := view.GetDrawFunc()
	view.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		if prevDraw != nil {
			x, y, width, height = prevDraw(screen, x, y, width, height)
		}
		// The scrollbar may have claimed a column, and a resize changes where
		// lines break. Lay the tree out to the width it will actually draw at.
		if selection.active() && width > 0 && width != selection.laidWidth {
			selection.layout(false, width)
		}
		return x, y, width, height
	})
	return selection
}

// relayout redraws the tree after wrap is toggled, so lines break at the new
// width and the highlight covers every visual line of the selected row.
func (s *jsonTreeSelection) relayout() {
	if !s.active() {
		return
	}
	s.render(false)
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
	s.setRows(nil)
	if !enabled {
		return false
	}
	rows, ok := buildJSONTreeRows(content, s.folded)
	if !ok || len(rows) == 0 {
		return false
	}
	s.setRows(rows)
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
		case 'g':
			s.selectRow(0)
			return true
		case 'G':
			s.selectRow(len(s.rows) - 1)
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
	s.setRows(rows)
	// Folding a node removes its children, so follow the path back to wherever
	// the cursor's row ended up.
	if index, found := s.indexOfPath(row.path); found {
		s.selected = index
	} else if s.selected >= len(rows) {
		s.selected = len(rows) - 1
	}
	s.render(false)
}

func (s *jsonTreeSelection) setRows(rows []jsonTreeRow) {
	s.rows = rows
	s.wrapped = nil
	s.rowsWidth = 0
	for _, row := range rows {
		if w := tview.TaggedStringWidth(row.text); w > s.rowsWidth {
			s.rowsWidth = w
		}
	}
}

// panPadding extends the highlighted row with blank space out to the widest
// row. tview only pans as far as the longest line it has parsed, and it parses
// no further than the page, so the selected row, always on screen, keeps every
// column reachable.
func (s *jsonTreeSelection) panPadding(text string, barWidth int) string {
	w := tview.TaggedStringWidth(stripStyleTags(text))
	if w < barWidth {
		w = barWidth
	}
	if s.rowsWidth <= w {
		return ""
	}
	return strings.Repeat(" ", s.rowsWidth-w)
}

func (s *jsonTreeSelection) wrappedRow(index, width int) []string {
	if s.wrapWidth != width || len(s.wrapped) != len(s.rows) {
		s.wrapped = make([][]string, len(s.rows))
		s.wrapWidth = width
	}
	if s.wrapped[index] == nil {
		s.wrapped[index] = wrapTaggedLines(s.rows[index].text, width)
	}
	return s.wrapped[index]
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
		s.scrollIntoView()
		return
	}
	s.selected = index
	s.render(false)
}

// scrollIntoView nudges the pane just enough to show the cursor, where tview's
// own ScrollToHighlight would re-center the view on every step.
func (s *jsonTreeSelection) scrollIntoView() {
	offset, column := s.view.GetScrollOffset()
	if s.wrapping() {
		column = 0
	}
	start, span := s.visualSpan(s.selected)
	next := offset
	if _, _, _, height := s.view.GetInnerRect(); height > 0 {
		if start < next {
			next = start
		}
		if start+span > next+height {
			if span >= height {
				next = start
			} else {
				next = start + span - height
			}
		}
	}
	if next < 0 {
		next = 0
	}
	_, storedColumn := s.view.GetScrollOffset()
	if next != offset || storedColumn != column {
		s.view.ScrollTo(next, column)
	}
}

func (s *jsonTreeSelection) visualSpan(index int) (start, span int) {
	if index < 0 || index >= len(s.visual) {
		return index, 1
	}
	start = s.visual[index]
	end := s.visualRows
	if index+1 < len(s.visual) {
		end = s.visual[index+1]
	}
	span = end - start
	if span < 1 {
		span = 1
	}
	return start, span
}

func (s *jsonTreeSelection) rowAtVisual(line int) int {
	if len(s.visual) == 0 {
		return line
	}
	index := 0
	for i, start := range s.visual {
		if start > line {
			break
		}
		index = i
	}
	return index
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
	index := s.rowAtVisual(offset + y - top)
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
	_, _, width, _ := s.view.GetInnerRect()
	s.layout(toTop, width)
}

func (s *jsonTreeSelection) wrapping() bool {
	return s != nil && s.view != nil && textViewWraps(s.view)
}

func (s *jsonTreeSelection) layout(toTop bool, width int) {
	offset, column := s.view.GetScrollOffset()
	if toTop {
		offset, column = 0, 0
	}
	// A view that has not been given a size yet stays one line per row. Breaking
	// at a width of 0 would turn every character into its own line, and the
	// first real draw lays the tree out once the pane is measured.
	wrapping := s.wrapping() && width > 0
	if width < 1 {
		s.laidWidth = 0
		width = 0
	} else {
		s.laidWidth = width
	}
	if wrapping {
		column = 0
	} else if width > 0 {
		// Panning shifts the bar with the text, and the bar still stops at the
		// right edge of the viewport.
		width += column
	}
	s.visual = make([]int, len(s.rows))
	lines := make([]string, 0, len(s.rows))
	for i, row := range s.rows {
		s.visual[i] = len(lines)
		switch {
		case wrapping && i == s.selected:
			lines = append(lines, jsonTreeHighlightedWrapped(row.text, s.laidWidth)...)
		case wrapping:
			lines = append(lines, s.wrappedRow(i, s.laidWidth)...)
		case i == s.selected:
			lines = append(lines, jsonTreeHighlightedRow(row.text, width)+s.panPadding(row.text, width))
		default:
			lines = append(lines, row.text)
		}
	}
	s.visualRows = len(lines)
	s.text = strings.Join(lines, "\n")
	s.view.SetText(s.text)
	s.view.ScrollTo(offset, column)
	s.scrollIntoView()
}

// jsonTreeHighlightedRow paints the whole row in the selection colors the
// tables use, padded so the bar runs the full width rather than stopping at the
// end of the text.
func jsonTreeHighlightedRow(text string, width int) string {
	return jsonTreeBar(stripStyleTags(text), width)
}

// jsonTreeHighlightedWrapped breaks a row at the pane width and paints every
// visual line, padded out to that width, so a wrapped value is one solid bar.
func jsonTreeHighlightedWrapped(text string, width int) []string {
	parts := wrapTaggedLines(stripStyleTags(text), width)
	lines := make([]string, len(parts))
	for i, part := range parts {
		lines[i] = jsonTreeBar(part, width)
	}
	return lines
}

// jsonTreeBar paints plain in the selection colors, padded out to width.
// The opening tag is repeated before the padding because tview runs every
// line through Unicode sentence segmentation, which rescans to the end of the
// line for each space that follows a full stop. A value ending in "." padded
// out to a very wide row took minutes to draw; the letter in the repeated tag
// ends the sentence so the padding costs nothing extra.
func jsonTreeBar(plain string, width int) string {
	open := "[" + theme.ColorToHex(accentTextColor()) + ":" + theme.TagAccent() + ":b]"
	pad := ""
	if n := width - tview.TaggedStringWidth(plain); n > 0 {
		pad = strings.Repeat(" ", n)
	}
	return open + plain + open + pad + "[-:-:-]"
}

type wrapCell struct {
	tag string
	r   rune
	w   int
}

func wrapTaggedLines(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	cells := splitWrapCells(text)
	if len(cells) == 0 {
		return []string{""}
	}
	var lines []string
	for len(cells) > 0 {
		n := takeWrapLine(cells, width)
		if n <= 0 {
			n = 1
		}
		lines = append(lines, joinWrapCells(cells[:n]))
		cells = cells[n:]
	}
	return lines
}

func splitWrapCells(text string) []wrapCell {
	var cells []wrapCell
	for len(text) > 0 {
		if text[0] == '[' {
			end := strings.IndexByte(text, ']')
			if end >= 0 {
				cells = append(cells, wrapCell{tag: text[:end+1]})
				text = text[end+1:]
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		w := tview.TaggedStringWidth(string(r))
		if w < 1 && r != 0 {
			w = 1
		}
		cells = append(cells, wrapCell{r: r, w: w})
	}
	return cells
}

// takeWrapLine reports how many cells fit on one line, breaking at the last
// space when there is one and hard-breaking a token that is wider than the pane.
func takeWrapLine(cells []wrapCell, width int) int {
	used := 0
	lastBreak := 0
	for i, cell := range cells {
		if cell.tag != "" {
			continue
		}
		if used+cell.w > width && used > 0 {
			if lastBreak > 0 {
				return lastBreak
			}
			return i
		}
		used += cell.w
		if cell.r == ' ' || cell.r == '\t' {
			lastBreak = i + 1
		}
	}
	return len(cells)
}

func joinWrapCells(cells []wrapCell) string {
	var b strings.Builder
	for _, cell := range cells {
		if cell.tag != "" {
			b.WriteString(cell.tag)
			continue
		}
		b.WriteRune(cell.r)
	}
	return b.String()
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

// active reports whether the view still shows the tree. A status message
// written straight into the view replaces it until the next setContent.
func (s *jsonTreeSelection) active() bool {
	return s != nil && s.view != nil && len(s.rows) > 0 && s.view.GetText(false) == s.text
}
