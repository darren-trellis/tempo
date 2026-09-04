package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type scrollMetrics struct {
	offset  int
	visible int
	total   int
}

func (m scrollMetrics) overflow() bool {
	return m.total > m.visible && m.visible > 0 && m.total > 0
}

func appShowsScrollbars(app *App) bool {
	if app == nil {
		return true
	}
	return app.Config().ShouldShowScrollbars()
}

func scrollbarThumb(offset, visible, total, length int) (pos, size int) {
	if length <= 0 || total <= 0 || visible <= 0 {
		return 0, 0
	}
	if total <= visible {
		return 0, length
	}
	size = length * visible / total
	if size < 1 {
		size = 1
	}
	if size > length {
		size = length
	}
	span := length - size
	maxOffset := total - visible
	if maxOffset < 1 {
		maxOffset = 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	pos = offset * span / maxOffset
	if pos < 0 {
		pos = 0
	}
	if pos+size > length {
		pos = length - size
	}
	return pos, size
}

const (
	scrollbarThinHoriz = '▁'
	scrollbarThinVert  = '▕'
)

func drawScrollbar(screen tcell.Screen, x, y, length int, m scrollMetrics, vertical bool) {
	if screen == nil || length <= 0 || !m.overflow() {
		return
	}
	pos, size := scrollbarThumb(m.offset, m.visible, m.total, length)
	if size <= 0 {
		return
	}
	track := tcell.StyleDefault.Foreground(theme.FgDim()).Background(theme.Bg())
	thumb := tcell.StyleDefault.Foreground(theme.Fg()).Background(theme.Bg())
	glyph := scrollbarThinHoriz
	if vertical {
		glyph = scrollbarThinVert
	}
	for i := 0; i < length; i++ {
		style := track
		if i >= pos && i < pos+size {
			style = thumb
		}
		if vertical {
			screen.SetContent(x, y+i, glyph, nil, style)
			continue
		}
		screen.SetContent(x+i, y, glyph, nil, style)
	}
}

func tableVerticalScroll(table *components.Table, height int) scrollMetrics {
	if table == nil || height <= 0 {
		return scrollMetrics{}
	}
	header := table.GetRowCount() - table.GetDataRowCount()
	if header < 0 {
		header = 0
	}
	visible := height - header
	if visible < 1 {
		visible = 1
	}
	rowOffset, _ := table.GetOffset()
	return scrollMetrics{offset: rowOffset, visible: visible, total: table.GetDataRowCount()}
}

func textViewScrollMetrics(view *tview.TextView, width, height int) (vert, horiz scrollMetrics) {
	if view == nil || width <= 0 || height <= 0 {
		return scrollMetrics{}, scrollMetrics{}
	}
	row, col := view.GetScrollOffset()
	total := view.GetWrappedLineCount()
	if total < 1 {
		total = view.GetOriginalLineCount()
	}
	vert = scrollMetrics{offset: row, visible: height, total: total}
	horiz = scrollMetrics{offset: col, visible: width, total: textViewContentWidth(view)}
	return vert, horiz
}

func textViewContentWidth(view *tview.TextView) int {
	if view == nil {
		return 0
	}
	width := 0
	for _, line := range splitTextViewLines(view.GetText(true)) {
		if n := tview.TaggedStringWidth(line); n > width {
			width = n
		}
	}
	return width
}

func splitTextViewLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := make([]string, 0, 8)
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
		}
	}
	lines = append(lines, text[start:])
	return lines
}

func treeScrollMetrics(tree *tview.TreeView, height int) scrollMetrics {
	if tree == nil || height <= 0 {
		return scrollMetrics{}
	}
	return scrollMetrics{
		offset:  tree.GetScrollOffset(),
		visible: height,
		total:   tree.GetRowCount(),
	}
}

func attachTextViewScrollbar(view *tview.TextView, app *App) {
	if view == nil {
		return
	}
	view.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		if !appShowsScrollbars(app) || width < 2 || height < 1 {
			return x, y, width, height
		}
		vert, horiz := textViewScrollMetrics(view, width, height)
		innerW, innerH := width, height
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
		vert, horiz = textViewScrollMetrics(view, innerW, innerH)
		if vert.overflow() {
			drawScrollbar(screen, x+width-1, y, innerH, vert, true)
		}
		if horiz.overflow() {
			drawScrollbar(screen, x, y+height-1, innerW, horiz, false)
		}
		return x, y, innerW, innerH
	})
}

func attachTreeScrollbar(tree *tview.TreeView, app *App) {
	if tree == nil {
		return
	}
	tree.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		if !appShowsScrollbars(app) || width < 2 || height < 1 {
			return x, y, width, height
		}
		vert := treeScrollMetrics(tree, height)
		if !vert.overflow() {
			return x, y, width, height
		}
		drawScrollbar(screen, x+width-1, y, height, vert, true)
		return x, y, width - 1, height
	})
}
