package view

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/atterpac/jig/theme"
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
	wl   *WorkflowList
	hits []filterBarHit
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
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftClick, tview.MouseLeftDoubleClick:
			if action == tview.MouseLeftClick || action == tview.MouseLeftDoubleClick {
				px, py := event.Position()
				b.handleClick(px, py)
			}
			return true, nil
		}
		return false, nil
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

func filterChipWidth(label string) int {
	return utf8.RuneCountInString(label) + 2
}

func filterOverflowWidth(n int) int {
	if n <= 0 {
		return 0
	}
	return 1 + utf8.RuneCountInString(fmt.Sprintf("+%d", n))
}

func layoutFilterBar(items []filterBarChip, width int) ([]filterBarChip, int) {
	if width <= 0 || len(items) == 0 {
		return nil, 0
	}
	best := 1
	if best > len(items) {
		best = len(items)
	}
	for n := 1; n <= len(items); n++ {
		used := 0
		for i := 0; i < n; i++ {
			if i > 0 {
				used++
			}
			used += filterChipWidth(items[i].label)
		}
		used += filterOverflowWidth(len(items) - n)
		if used <= width {
			best = n
			continue
		}
		break
	}
	return items[:best], len(items) - best
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
			need++
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

func (wl *WorkflowList) shouldWrapFilters() bool {
	if wl == nil || wl.app == nil {
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
	return wl.filterBarChipRows() + 1
}

func (wl *WorkflowList) syncFilterBarHeight() {
	if wl == nil || wl.workflowStack == nil || wl.filterBar == nil {
		return
	}
	rows := wl.filterBarHeight()
	if rows == wl.filterBarRows {
		return
	}
	wl.filterBarRows = rows
	wl.workflowStack.ResizeItem(wl.filterBar, rows, 0)
}

func (b *filterChipBar) Draw(screen tcell.Screen) {
	if b == nil {
		return
	}
	b.Box.SetBackgroundColor(theme.Bg())
	b.Box.DrawForSubclass(screen, b)
	b.hits = nil
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
	var lines [][]filterBarChip
	extra := 0
	if b.wl != nil && b.wl.shouldWrapFilters() {
		lines = layoutFilterBarLines(items, width)
	} else {
		shown, n := layoutFilterBar(items, width)
		lines = [][]filterBarChip{shown}
		extra = n
	}
	for row, shown := range lines {
		if row >= chipRows {
			break
		}
		col := x
		for _, item := range shown {
			style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
			if item.active {
				style = tcell.StyleDefault.Background(theme.Accent()).Foreground(theme.Bg())
			}
			x0 := col
			if col < x+width {
				screen.SetContent(col, y+row, ' ', nil, style)
				col++
			}
			for _, r := range item.label {
				if col >= x+width {
					break
				}
				screen.SetContent(col, y+row, r, nil, style)
				col++
			}
			if col < x+width {
				screen.SetContent(col, y+row, ' ', nil, style)
				col++
			}
			b.hits = append(b.hits, filterBarHit{name: item.name, x0: x0, x1: col, y: y + row})
			if col < x+width {
				screen.SetContent(col, y+row, ' ', nil, bg)
				col++
			}
		}
		if extra > 0 && row == len(lines)-1 {
			dim := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.FgDim())
			for _, r := range fmt.Sprintf("+%d", extra) {
				if col >= x+width {
					break
				}
				screen.SetContent(col, y+row, r, nil, dim)
				col++
			}
		}
	}
	if height > 1 {
		line := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Border())
		row := y + height - 1
		for col := x; col < x+width; col++ {
			screen.SetContent(col, row, '─', nil, line)
		}
	}
}

func (b *filterChipBar) handleClick(x, y int) {
	if b == nil || b.wl == nil {
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
