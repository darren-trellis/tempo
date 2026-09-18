package view

import (
	"strings"
	"unicode/utf8"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type filterBarChip struct {
	label  string
	name   string
	active bool
}

type filterBarHit struct {
	name   string
	x0, x1 int
	y      int
}

type filterChipBar struct {
	*tview.Box
	wl      *WorkflowList
	hits    []filterBarHit
	cursor  int
	offset  int
	hOffset int
}

func newFilterChipBar(wl *WorkflowList) *filterChipBar {
	b := &filterChipBar{Box: tview.NewBox(), wl: wl}
	b.SetBackgroundColor(theme.Bg())
	return b
}

func (b *filterChipBar) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return b.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if b == nil || event == nil || !b.InRect(event.Position()) {
			return false, nil
		}
		side := b.wl != nil && b.wl.filtersOnSide()
		wrap := b.wl != nil && b.wl.shouldWrapFilters()
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftClick, tview.MouseLeftDoubleClick:
			if side && b.wl != nil {
				b.wl.setFocusPane(focusFilters)
			}
			if action == tview.MouseLeftClick || action == tview.MouseLeftDoubleClick {
				px, py := event.Position()
				b.handleClick(px, py)
			}
			if side {
				return true, b
			}
			return true, nil
		case tview.MouseScrollUp, tview.MouseScrollDown, tview.MouseScrollLeft, tview.MouseScrollRight:
			if side {
				if action == tview.MouseScrollUp {
					b.moveCursor(-1)
					return true, b
				}
				if action == tview.MouseScrollDown {
					b.moveCursor(1)
					return true, b
				}
				return false, nil
			}
			if !wrap {
				if delta := filterBarScrollDelta(action); delta != 0 {
					b.scrollHoriz(delta * b.horizScrollStep())
					return true, nil
				}
			}
		}
		return false, nil
	})
}

func (b *filterChipBar) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if b == nil || event == nil || b.wl == nil || !b.wl.filtersOnSide() {
			return
		}
		if b.wl.handlePaneResizeKey(event) || b.wl.handleListTabKey(event) {
			return
		}
		switch event.Key() {
		case tcell.KeyTab:
			b.wl.cycleFocus(1)
			return
		case tcell.KeyBacktab:
			b.wl.cycleFocus(-1)
			return
		case tcell.KeyEscape:
			b.wl.setFocusPane(focusWorkflows)
			return
		case tcell.KeyUp, tcell.KeyCtrlP:
			b.moveCursor(-1)
			return
		case tcell.KeyDown, tcell.KeyCtrlN:
			b.moveCursor(1)
			return
		case tcell.KeyEnter:
			b.applyCursor()
			return
		case tcell.KeyHome:
			b.setCursor(0)
			return
		case tcell.KeyEnd:
			b.setCursor(b.itemCount() - 1)
			return
		case tcell.KeyPgUp:
			b.moveCursor(-b.pageSize())
			return
		case tcell.KeyPgDn:
			b.moveCursor(b.pageSize())
			return
		}
		switch event.Rune() {
		case 'k':
			b.moveCursor(-1)
		case 'j':
			b.moveCursor(1)
		case 'g':
			b.setCursor(0)
		case 'G':
			b.setCursor(b.itemCount() - 1)
		case 'l':
			b.applyCursor()
		}
	})
}

func filterBarItems(wl *WorkflowList) []filterBarChip {
	allActive := wl == nil || (wl.activeFilterName == "" && wl.visibilityQuery == "")
	items := []filterBarChip{{label: "All", active: allActive}}
	if wl == nil || wl.app == nil || wl.app.Config() == nil {
		return items
	}
	for _, f := range wl.app.Config().GetSavedFilters() {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			continue
		}
		items = append(items, filterBarChip{
			label:  name,
			name:   name,
			active: wl.activeFilterName != "" && strings.EqualFold(wl.activeFilterName, name),
		})
	}
	return items
}

const filterBarChipSep = "|"

func filterChipWidth(label string) int {
	return utf8.RuneCountInString(label) + 2
}

func filterBarSepWidth() int {
	return utf8.RuneCountInString(filterBarChipSep)
}

func filterBarContentWidth(items []filterBarChip) int {
	if len(items) == 0 {
		return 0
	}
	used := 0
	for i, item := range items {
		if i > 0 {
			used += filterBarSepWidth()
		}
		used += filterChipWidth(item.label)
	}
	return used
}

func filterBarScrollDelta(action tview.MouseAction) int {
	switch action {
	case tview.MouseScrollLeft, tview.MouseScrollUp:
		return -1
	case tview.MouseScrollRight, tview.MouseScrollDown:
		return 1
	}
	return 0
}

func layoutFilterBarLines(items []filterBarChip, width int) [][]filterBarChip {
	if len(items) == 0 {
		return nil
	}
	if width <= 0 {
		return [][]filterBarChip{items[:1]}
	}
	var lines [][]filterBarChip
	var line []filterBarChip
	used := 0
	for _, item := range items {
		w := filterChipWidth(item.label)
		need := w
		if len(line) > 0 {
			need += filterBarSepWidth()
		}
		if len(line) > 0 && used+need > width {
			lines = append(lines, line)
			line = nil
			used = 0
			need = w
		}
		line = append(line, item)
		used += need
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

func (wl *WorkflowList) filtersOnSide() bool {
	if wl == nil || wl.app == nil {
		return false
	}
	return wl.app.Config().ResolvedSavedFiltersPosition() == config.SavedFiltersPositionSide
}

func (wl *WorkflowList) shouldWrapFilters() bool {
	if wl == nil || wl.app == nil || wl.filtersOnSide() {
		return false
	}
	return wl.app.Config().ShouldWrapFilters()
}

func (wl *WorkflowList) filterBarWidth() int {
	if wl == nil {
		return 0
	}
	if wl.filterBar != nil {
		_, _, w, _ := wl.filterBar.GetInnerRect()
		if w > 0 {
			return w
		}
	}
	if wl.workflowStack != nil {
		_, _, w, _ := wl.workflowStack.GetInnerRect()
		if w > 0 {
			return w
		}
	}
	if wl.listTabs != nil {
		_, _, w, _ := wl.listTabs.GetInnerRect()
		if w > 0 {
			return w
		}
	}
	return 0
}

func (wl *WorkflowList) filterSidebarWidth() int {
	max := utf8.RuneCountInString("All")
	for _, item := range filterBarItems(wl) {
		if n := utf8.RuneCountInString(item.label); n > max {
			max = n
		}
	}
	w := max + 3
	if w < 14 {
		w = 14
	}
	if w > 36 {
		w = 36
	}
	return w
}

func (wl *WorkflowList) filterBarChipRows() int {
	if wl == nil || !wl.shouldWrapFilters() {
		return 1
	}
	lines := layoutFilterBarLines(filterBarItems(wl), wl.filterBarWidth())
	if len(lines) == 0 {
		return 1
	}
	return len(lines)
}

func (wl *WorkflowList) filterBarHeight() int {
	if wl != nil && wl.filtersOnSide() {
		return 0
	}
	return wl.filterBarChipRows() + 1
}

func (wl *WorkflowList) syncFilterBarLayout() {
	if wl == nil || wl.workflowStack == nil || wl.filterBar == nil {
		return
	}
	side := wl.filtersOnSide()
	if side != wl.filterBarSide {
		if !side && wl.focusPane == focusFilters {
			wl.focusPane = focusWorkflows
		}
		wl.mountWorkflowContent()
		return
	}
	size := wl.filterSidebarWidth()
	if !side {
		size = wl.filterBarHeight()
	}
	if size == wl.filterBarRows {
		return
	}
	wl.filterBarRows = size
	wl.workflowStack.ResizeItem(wl.filterBar, size, 0)
}

func (b *filterChipBar) Draw(screen tcell.Screen) {
	if b == nil {
		return
	}
	b.Box.SetBackgroundColor(theme.Bg())
	b.Box.DrawForSubclass(screen, b)
	b.hits = nil
	if b.wl != nil && b.wl.filtersOnSide() {
		b.drawSide(screen)
		return
	}
	b.drawTop(screen)
}

func (b *filterChipBar) drawTop(screen tcell.Screen) {
	x, y, width, height := b.GetInnerRect()
	if width < 1 || height < 1 {
		return
	}
	bg := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	for row := 0; row < height; row++ {
		for col := x; col < x+width; col++ {
			screen.SetContent(col, y+row, ' ', nil, bg)
		}
	}
	chipRows := height
	if height > 1 {
		chipRows = height - 1
	}
	items := filterBarItems(b.wl)
	if b.wl != nil && b.wl.shouldWrapFilters() {
		lines := layoutFilterBarLines(items, width)
		for row, shown := range lines {
			if row >= chipRows {
				break
			}
			b.paintChipRow(screen, shown, x, y+row, width, 0)
		}
	} else {
		b.clampHOffset()
		b.paintChipRow(screen, items, x, y, width, b.hOffset)
	}
	if height > 1 {
		b.paintTopDivider(screen, x, y+height-1, width, items)
	}
}

func (b *filterChipBar) paintTopDivider(screen tcell.Screen, x, y, width int, items []filterBarChip) {
	line := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Border())
	thumb := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	pos, size := -1, 0
	if b.wl != nil && !b.wl.shouldWrapFilters() && appShowsScrollbars(b.wl.app) {
		metrics := scrollMetrics{offset: b.hOffset, visible: width, total: filterBarContentWidth(items)}
		if metrics.overflow() {
			pos, size = scrollbarThumb(metrics.offset, metrics.visible, metrics.total, width)
		}
	}
	for col := 0; col < width; col++ {
		style := line
		if col >= pos && col < pos+size {
			style = thumb
		}
		screen.SetContent(x+col, y, '─', nil, style)
	}
}

func (b *filterChipBar) paintChipRow(screen tcell.Screen, items []filterBarChip, x, y, width, start int) {
	col := x - start
	limit := x + width
	for i, item := range items {
		style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
		if item.active {
			style = tcell.StyleDefault.Background(theme.Accent()).Foreground(theme.Bg())
		}
		vis0, vis1 := -1, -1
		write := func(r rune, st tcell.Style) {
			if col >= x && col < limit {
				screen.SetContent(col, y, r, nil, st)
				if vis0 < 0 {
					vis0 = col
				}
				vis1 = col + 1
			}
			col++
		}
		write(' ', style)
		for _, r := range item.label {
			write(r, style)
		}
		write(' ', style)
		if vis0 >= 0 {
			b.hits = append(b.hits, filterBarHit{name: item.name, x0: vis0, x1: vis1, y: y})
		}
		if i < len(items)-1 {
			sep := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.FgDim())
			for _, r := range filterBarChipSep {
				write(r, sep)
			}
		}
	}
}

func (b *filterChipBar) horizScrollStep() int {
	if b == nil || b.wl == nil {
		return 1
	}
	return mouseScrollStepFromApp(b.wl.app)
}

func (b *filterChipBar) maxHOffset() int {
	if b == nil {
		return 0
	}
	_, _, w, _ := b.GetInnerRect()
	max := filterBarContentWidth(filterBarItems(b.wl)) - w
	if max < 0 {
		return 0
	}
	return max
}

func (b *filterChipBar) clampHOffset() {
	if b == nil {
		return
	}
	if b.hOffset < 0 {
		b.hOffset = 0
	}
	if max := b.maxHOffset(); b.hOffset > max {
		b.hOffset = max
	}
}

func (b *filterChipBar) scrollHoriz(delta int) {
	if b == nil {
		return
	}
	b.hOffset += delta
	b.clampHOffset()
}

func (b *filterChipBar) revealChip(name string) {
	if b == nil {
		return
	}
	items := filterBarItems(b.wl)
	_, _, vw, _ := b.GetInnerRect()
	if vw < 1 {
		return
	}
	col := 0
	for i, item := range items {
		w := filterChipWidth(item.label)
		if strings.EqualFold(item.name, name) {
			if col < b.hOffset {
				b.hOffset = col
			} else if col+w > b.hOffset+vw {
				b.hOffset = col + w - vw
			}
			b.clampHOffset()
			return
		}
		col += w
		if i < len(items)-1 {
			col += filterBarSepWidth()
		}
	}
}

func (b *filterChipBar) drawSide(screen tcell.Screen) {
	x, y, width, height := b.GetInnerRect()
	if width < 1 || height < 1 {
		return
	}
	bg := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	for row := 0; row < height; row++ {
		for col := x; col < x+width; col++ {
			screen.SetContent(col, y+row, ' ', nil, bg)
		}
	}
	items := filterBarItems(b.wl)
	b.clampCursor(len(items))
	inner := width - 1
	if inner < 1 {
		inner = width
	}
	b.ensureCursorVisible()
	focused := b.HasFocus() || (b.wl != nil && b.wl.focusPane == focusFilters)
	for row := 0; row < height; row++ {
		idx := b.offset + row
		if idx >= 0 && idx < len(items) {
			item := items[idx]
			style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
			switch {
			case item.active:
				style = tcell.StyleDefault.Background(theme.Accent()).Foreground(theme.Bg())
			case focused && idx == b.cursor:
				style = tcell.StyleDefault.Background(theme.BgLight()).Foreground(theme.Fg())
			}
			label := truncateIfNeeded(item.label, inner-2)
			col := x
			if col < x+inner {
				screen.SetContent(col, y+row, ' ', nil, style)
				col++
			}
			for _, r := range label {
				if col >= x+inner {
					break
				}
				screen.SetContent(col, y+row, r, nil, style)
				col++
			}
			for col < x+inner {
				screen.SetContent(col, y+row, ' ', nil, style)
				col++
			}
			b.hits = append(b.hits, filterBarHit{name: item.name, x0: x, x1: x + inner, y: y + row})
		}
		if inner < width {
			line := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Border())
			if focused {
				line = tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Accent())
			}
			screen.SetContent(x+width-1, y+row, '│', nil, line)
		}
	}
}

func (b *filterChipBar) itemCount() int {
	return len(filterBarItems(b.wl))
}

func (b *filterChipBar) pageSize() int {
	if b == nil {
		return 1
	}
	_, _, _, h := b.GetInnerRect()
	if h < 1 {
		return 1
	}
	return h
}

func (b *filterChipBar) clampCursor(n int) {
	if n <= 0 {
		b.cursor = 0
		b.offset = 0
		return
	}
	if b.cursor < 0 {
		b.cursor = 0
	}
	if b.cursor >= n {
		b.cursor = n - 1
	}
	if b.offset < 0 {
		b.offset = 0
	}
}

func (b *filterChipBar) setCursor(i int) {
	b.cursor = i
	b.clampCursor(b.itemCount())
	b.ensureCursorVisible()
}

func (b *filterChipBar) moveCursor(delta int) {
	b.setCursor(b.cursor + delta)
}

func (b *filterChipBar) ensureCursorVisible() {
	if b == nil {
		return
	}
	_, _, _, h := b.GetInnerRect()
	if h < 1 {
		return
	}
	if b.cursor < b.offset {
		b.offset = b.cursor
	}
	if b.cursor >= b.offset+h {
		b.offset = b.cursor - h + 1
	}
	if b.offset < 0 {
		b.offset = 0
	}
}

func (b *filterChipBar) applyCursor() {
	items := filterBarItems(b.wl)
	if b.cursor < 0 || b.cursor >= len(items) {
		return
	}
	b.applyItem(items[b.cursor])
}

func (b *filterChipBar) applyItem(item filterBarChip) {
	if b == nil || b.wl == nil {
		return
	}
	a := b.wl.app
	if a != nil && a.app != nil && a.app.Pages() != nil && a.app.Pages().CurrentIsModal() {
		return
	}
	if item.name == "" {
		b.wl.applyAllWorkflowsFilter()
		return
	}
	if a != nil {
		a.loadSavedFilter(b.wl, item.name)
	}
}

func (b *filterChipBar) handleClick(x, y int) {
	if b == nil || b.wl == nil {
		return
	}
	if b.wl.filtersOnSide() {
		_, iy, _, _ := b.GetInnerRect()
		idx := b.offset + (y - iy)
		items := filterBarItems(b.wl)
		if idx >= 0 && idx < len(items) {
			b.cursor = idx
			b.applyItem(items[idx])
		}
		return
	}
	a := b.wl.app
	if a != nil && a.app != nil && a.app.Pages() != nil && a.app.Pages().CurrentIsModal() {
		return
	}
	for _, hit := range b.hits {
		if y == hit.y && x >= hit.x0 && x < hit.x1 {
			if hit.name == "" {
				b.wl.applyAllWorkflowsFilter()
				return
			}
			if a != nil {
				a.loadSavedFilter(b.wl, hit.name)
			}
			return
		}
	}
}
