package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type clipScreen struct {
	tcell.Screen
	x, y, w, h int
}

func (s *clipScreen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
	if x < s.x || x >= s.x+s.w || y < s.y || y >= s.y+s.h {
		return
	}
	s.Screen.SetContent(x, y, mainc, combc, style)
}

type charScrollView struct {
	*tview.Box
	app          *App
	content      tview.Primitive
	offset       int
	contentWidth func() int
}

func newCharScrollView(content tview.Primitive, contentWidth func() int) *charScrollView {
	return &charScrollView{
		Box:          tview.NewBox(),
		content:      content,
		contentWidth: contentWidth,
	}
}

func (v *charScrollView) withApp(app *App) *charScrollView {
	if v != nil {
		v.app = app
	}
	return v
}

func (v *charScrollView) Draw(screen tcell.Screen) {
	x, y, w, h := v.GetInnerRect()
	v.clamp()
	if v.content == nil || w <= 0 || h <= 0 {
		return
	}
	innerW, innerH := w, h
	vert, horiz := v.scrollBars(innerW, innerH)
	if appShowsScrollbars(v.app) {
		if vert.overflow() {
			innerW--
		}
		if horiz.overflow() {
			innerH--
		}
		if innerW < 1 {
			innerW = 1
		}
		if innerH < 1 {
			innerH = 1
		}
		vert, horiz = v.scrollBars(innerW, innerH)
	}
	width := innerW
	if v.contentWidth != nil {
		if cw := v.contentWidth(); cw > width {
			width = cw
		}
	}
	v.content.SetRect(x-v.offset, y, width, innerH)
	v.content.Draw(&clipScreen{Screen: screen, x: x, y: y, w: innerW, h: innerH})
	if !appShowsScrollbars(v.app) {
		return
	}
	vert, horiz = v.scrollBars(innerW, innerH)
	if vert.overflow() {
		drawScrollbar(screen, x+w-1, y, innerH, vert, true)
	}
	if horiz.overflow() {
		drawScrollbar(screen, x, y+h-1, innerW, horiz, false)
	}
}

func (v *charScrollView) scrollBars(width, height int) (vert, horiz scrollMetrics) {
	if table, ok := v.content.(*components.Table); ok {
		vert = tableVerticalScroll(table, height)
	}
	contentW := width
	if v.contentWidth != nil {
		contentW = v.contentWidth()
	}
	horiz = scrollMetrics{offset: v.offset, visible: width, total: contentW}
	return vert, horiz
}

func (v *charScrollView) Focus(delegate func(p tview.Primitive)) {
	if v.content != nil {
		delegate(v.content)
		return
	}
	delegate(v)
}

func (v *charScrollView) HasFocus() bool {
	if v.content != nil {
		return v.content.HasFocus()
	}
	return v.Box.HasFocus()
}

func (v *charScrollView) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return v.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if v.content != nil {
			if handler := v.content.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
		}
	})
}

func (v *charScrollView) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return v.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if v.content != nil {
			if handler := v.content.MouseHandler(); handler != nil {
				return handler(action, event, setFocus)
			}
		}
		return false, nil
	})
}

func (v *charScrollView) viewport() int {
	_, _, w, h := v.GetInnerRect()
	if w < 1 {
		return 0
	}
	if appShowsScrollbars(v.app) {
		if table, ok := v.content.(*components.Table); ok && tableVerticalScroll(table, h).overflow() && w > 1 {
			w--
		}
	}
	return w
}

func (v *charScrollView) maxOffset() int {
	width := 0
	if v.contentWidth != nil {
		width = v.contentWidth()
	}
	max := width - v.viewport()
	if max < 0 {
		return 0
	}
	return max
}

func (v *charScrollView) clamp() {
	if v.offset < 0 {
		v.offset = 0
	}
	if max := v.maxOffset(); v.offset > max {
		v.offset = max
	}
}

func (v *charScrollView) scrollChars(delta int) {
	v.offset += delta
	v.clamp()
}

func (v *charScrollView) scrollTo(offset int) {
	v.offset = offset
	v.clamp()
}

func workflowTableContentWidth(cols []workflowColumn) int {
	if len(cols) == 0 {
		return 0
	}
	width := 0
	for i, col := range cols {
		width += col.width
		if i < len(cols)-1 {
			width++
		}
	}
	return width
}

func workflowColumnOffsets(cols []workflowColumn) []int {
	offs := make([]int, len(cols))
	x := 0
	for i, col := range cols {
		offs[i] = x
		x += col.width
		if i < len(cols)-1 {
			x++
		}
	}
	return offs
}

func scrollOffsetByColumn(offset int, cols []workflowColumn, delta int) int {
	offs := workflowColumnOffsets(cols)
	if len(offs) == 0 {
		return 0
	}
	idx := 0
	for i, start := range offs {
		if start <= offset {
			idx = i
		}
	}
	if delta < 0 {
		if offset > offs[idx] {
			return offs[idx]
		}
		if idx > 0 {
			return offs[idx-1]
		}
		return 0
	}
	if idx+1 < len(offs) {
		return offs[idx+1]
	}
	return offs[idx]
}

// attachTableCharScroll wraps a table in a char scroll view and gives it
// horizontal mouse scrolling.
func attachTableCharScroll(table *components.Table, app *App) *charScrollView {
	view := newCharScrollView(table, func() int {
		return tableContentWidth(table)
	}).withApp(app)
	bindTableCharScroll(table, view, func() int {
		return mouseScrollStepFromApp(app)
	})
	return view
}

// handleCharScrollKeys scrolls a view horizontally: one char for left/right and
// h/l, one column for Home/End.
func handleCharScrollKeys(view *charScrollView, event *tcell.EventKey, cols func() []workflowColumn) bool {
	if view == nil || event == nil || event.Modifiers() != tcell.ModNone {
		return false
	}
	switch event.Key() {
	case tcell.KeyLeft:
		view.scrollChars(-1)
		return true
	case tcell.KeyRight:
		view.scrollChars(1)
		return true
	case tcell.KeyHome:
		if cols == nil {
			return false
		}
		view.scrollTo(scrollOffsetByColumn(view.offset, cols(), -1))
		return true
	case tcell.KeyEnd:
		if cols == nil {
			return false
		}
		view.scrollTo(scrollOffsetByColumn(view.offset, cols(), 1))
		return true
	case tcell.KeyRune:
		switch event.Rune() {
		case 'h':
			view.scrollChars(-1)
			return true
		case 'l':
			view.scrollChars(1)
			return true
		}
	}
	return false
}

// handleTableCharScroll scrolls an auto-sized table, measuring its columns for
// the Home/End jumps.
func handleTableCharScroll(view *charScrollView, table *components.Table, event *tcell.EventKey) bool {
	return handleCharScrollKeys(view, event, func() []workflowColumn {
		return tableColumnLayout(table)
	})
}

// tableColumnLayout measures a table's cells so char scrolling knows how wide
// it renders. tview sizes each column to its widest cell.
func tableColumnLayout(table *components.Table) []workflowColumn {
	if table == nil {
		return nil
	}
	rows := table.GetRowCount()
	cols := table.GetColumnCount()
	if rows == 0 || cols == 0 {
		return nil
	}
	layout := make([]workflowColumn, cols)
	for col := 0; col < cols; col++ {
		width := 0
		for row := 0; row < rows; row++ {
			cell := table.GetCell(row, col)
			if cell == nil {
				continue
			}
			cellWidth := tview.TaggedStringWidth(cell.Text)
			if cell.MaxWidth > 0 && cellWidth > cell.MaxWidth {
				cellWidth = cell.MaxWidth
			}
			if cellWidth > width {
				width = cellWidth
			}
		}
		layout[col] = workflowColumn{width: width}
	}
	return layout
}

// tableContentWidth is the rendered width of an auto-sized table.
func tableContentWidth(table *components.Table) int {
	return workflowTableContentWidth(tableColumnLayout(table))
}

func bindTableCharScroll(table *components.Table, view *charScrollView, step func() int) {
	if table == nil || view == nil {
		return
	}
	prev := table.GetMouseCapture()
	table.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if delta := horizontalMouseDelta(action, event); delta != 0 {
			view.scrollChars(delta * resolveMouseScrollStep(step))
			return tview.MouseConsumed, nil
		}
		if prev != nil {
			return prev(action, event)
		}
		return action, event
	})
}
