package view

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const dropdownMaxSuggestions = 12

type dropdownField struct {
	*tview.Box
	name        string
	label       string
	placeholder string
	value       string
	cursor      int
	offset      int
	listOffset  int
	options     []string
	matches     []string
	selected    int
	expanded    bool
	browseAll   bool
	focused     bool
	validator   func(any) error
	onChange    func(string)
}

func newDropdownField(name, label string, options []string) *dropdownField {
	return newOrderedDropdownField(name, label, uniqueSortedStrings(options))
}

func newOrderedDropdownField(name, label string, options []string) *dropdownField {
	f := &dropdownField{
		Box:      tview.NewBox(),
		name:     name,
		label:    label,
		options:  compactStrings(options),
		selected: -1,
	}
	f.refreshMatches()
	return f
}

func (f *dropdownField) SetPlaceholder(text string) *dropdownField {
	f.placeholder = text
	return f
}

func (f *dropdownField) SetValue(value string) *dropdownField {
	f.value = value
	f.cursor = utf8.RuneCountInString(value)
	f.browseAll = false
	f.refreshMatches()
	return f
}

func (f *dropdownField) SetOptions(options []string) *dropdownField {
	return f.SetOptionsOrdered(uniqueSortedStrings(options))
}

func (f *dropdownField) SetOptionsOrdered(options []string) *dropdownField {
	f.options = compactStrings(options)
	if f.browseAll {
		f.openList()
		return f
	}
	f.refreshMatches()
	return f
}

func (f *dropdownField) SetValidator(v func(any) error) *dropdownField {
	f.validator = v
	return f
}

func (f *dropdownField) SetChangedFunc(fn func(string)) *dropdownField {
	f.onChange = fn
	return f
}

func (f *dropdownField) emitChange() {
	if f != nil && f.onChange != nil {
		f.onChange(f.value)
	}
}

func (f *dropdownField) GetName() string { return f.name }

func (f *dropdownField) GetValue() string { return f.value }

func (f *dropdownField) Validate() error {
	if f.validator == nil {
		return nil
	}
	return f.validator(f.value)
}

func (f *dropdownField) visibleSuggestionCount() int {
	n := len(f.matches)
	if n > dropdownMaxSuggestions {
		return dropdownMaxSuggestions
	}
	return n
}

func (f *dropdownField) GetFieldHeight() int {
	height := 3
	if f.label != "" {
		height++
	}
	if f.expanded {
		height += f.visibleSuggestionCount() + 2
	}
	return height
}

func (f *dropdownField) Focus(delegate func(tview.Primitive)) {
	f.focused = true
	f.Box.Focus(delegate)
}

func (f *dropdownField) Blur() {
	f.focused = false
	f.expanded = false
	f.browseAll = false
	f.Box.Blur()
}

func (f *dropdownField) HasFocus() bool {
	return f.focused
}

func (f *dropdownField) collapse() bool {
	if f == nil || !f.expanded {
		return false
	}
	f.expanded = false
	f.browseAll = false
	return true
}

func (f *dropdownField) acceptSuggestion() bool {
	if f == nil || !f.expanded || len(f.matches) == 0 {
		return false
	}
	idx := f.selected
	if idx < 0 || idx >= len(f.matches) {
		idx = 0
	}
	choice := f.matches[idx]
	f.SetValue(choice)
	f.expanded = false
	f.emitChange()
	return true
}

func (f *dropdownField) refreshMatches() {
	f.matches = filterDropdownOptions(f.options, f.value)
	f.selected = 0
	for i, opt := range f.matches {
		if opt == f.value {
			f.selected = i
			break
		}
	}
	if len(f.matches) == 0 {
		f.selected = -1
		f.expanded = false
		f.listOffset = 0
		return
	}
	f.ensureListVisible()
}

func (f *dropdownField) ensureListVisible() {
	visible := f.visibleSuggestionCount()
	if visible <= 0 {
		f.listOffset = 0
		return
	}
	if f.selected < f.listOffset {
		f.listOffset = f.selected
	}
	if f.selected >= f.listOffset+visible {
		f.listOffset = f.selected - visible + 1
	}
	maxOffset := len(f.matches) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if f.listOffset < 0 {
		f.listOffset = 0
	}
	if f.listOffset > maxOffset {
		f.listOffset = maxOffset
	}
}

func filterDropdownOptions(options []string, query string) []string {
	if query == "" {
		return options
	}
	q := strings.ToLower(query)
	var prefix, rest []string
	for _, opt := range options {
		low := strings.ToLower(opt)
		switch {
		case strings.HasPrefix(low, q):
			prefix = append(prefix, opt)
		case strings.Contains(low, q):
			rest = append(rest, opt)
		}
	}
	return append(prefix, rest...)
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range compactStrings(values) {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func (f *dropdownField) runeValue() []rune {
	return []rune(f.value)
}

func (f *dropdownField) setRunes(runes []rune) {
	f.value = string(runes)
	if f.cursor < 0 {
		f.cursor = 0
	}
	if f.cursor > len(runes) {
		f.cursor = len(runes)
	}
	f.browseAll = false
	f.refreshMatches()
	if len(f.matches) > 0 {
		f.expanded = true
	}
}

func (f *dropdownField) openList() {
	f.browseAll = true
	f.matches = append([]string(nil), f.options...)
	f.selected = 0
	for i, opt := range f.matches {
		if opt == f.value {
			f.selected = i
			break
		}
	}
	if len(f.matches) == 0 {
		f.selected = -1
		f.expanded = false
		f.listOffset = 0
		return
	}
	f.expanded = true
	f.ensureListVisible()
}

func (f *dropdownField) moveSelection(delta int) {
	if !f.expanded {
		f.openList()
		return
	}
	if len(f.matches) == 0 {
		return
	}
	f.selected += delta
	if f.selected < 0 {
		f.selected = len(f.matches) - 1
	}
	if f.selected >= len(f.matches) {
		f.selected = 0
	}
	f.ensureListVisible()
}

func (f *dropdownField) visibleMatches() (start, end int) {
	visible := f.visibleSuggestionCount()
	start = f.listOffset
	if start < 0 {
		start = 0
	}
	end = start + visible
	if end > len(f.matches) {
		end = len(f.matches)
	}
	return start, end
}

func (f *dropdownField) Draw(screen tcell.Screen) {
	f.Box.DrawForSubclass(screen, f)
	x, y, width, height := f.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	bg := theme.Bg()
	fg := theme.Fg()
	dim := theme.FgDim()
	accent := theme.Accent()
	border := theme.Border()
	if f.focused {
		border = theme.BorderFocus()
	}

	row := y
	if f.label != "" {
		style := tcell.StyleDefault.Background(bg).Foreground(fg)
		col := x
		for _, r := range f.label {
			if col < x+width {
				screen.SetContent(col, row, r, nil, style)
				col++
			}
		}
		row++
	}

	borderStyle := tcell.StyleDefault.Background(bg).Foreground(border)
	textStyle := tcell.StyleDefault.Background(bg).Foreground(fg)
	placeholderStyle := tcell.StyleDefault.Background(bg).Foreground(dim)
	clearStyle := tcell.StyleDefault.Background(bg)

	screen.SetContent(x, row, '╭', nil, borderStyle)
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, row, '─', nil, borderStyle)
	}
	screen.SetContent(x+width-1, row, '╮', nil, borderStyle)
	row++

	screen.SetContent(x, row, '│', nil, borderStyle)
	screen.SetContent(x+width-1, row, '│', nil, borderStyle)
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, row, ' ', nil, clearStyle)
	}

	inputWidth := width - 5
	if inputWidth < 1 {
		inputWidth = 1
	}
	runes := f.runeValue()
	if f.cursor < f.offset {
		f.offset = f.cursor
	}
	if f.cursor >= f.offset+inputWidth {
		f.offset = f.cursor - inputWidth + 1
	}

	col := x + 2
	if len(runes) == 0 && !f.focused {
		for _, r := range f.placeholder {
			if col >= x+2+inputWidth {
				break
			}
			screen.SetContent(col, row, r, nil, placeholderStyle)
			col++
		}
	} else {
		visible := runes
		if f.offset < len(visible) {
			visible = visible[f.offset:]
		} else {
			visible = nil
		}
		if len(visible) > inputWidth {
			visible = visible[:inputWidth]
		}
		for i, r := range visible {
			style := textStyle
			if f.focused && f.offset+i == f.cursor {
				style = tcell.StyleDefault.Background(accent).Foreground(bg)
			}
			screen.SetContent(col, row, r, nil, style)
			col++
		}
		if f.focused && f.cursor >= len(runes) && col < x+2+inputWidth {
			screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(accent).Foreground(bg))
		}
	}

	indicator := '▼'
	if f.expanded {
		indicator = '▲'
	}
	screen.SetContent(x+width-3, row, indicator, nil, tcell.StyleDefault.Background(bg).Foreground(accent))
	row++

	screen.SetContent(x, row, '╰', nil, borderStyle)
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, row, '─', nil, borderStyle)
	}
	screen.SetContent(x+width-1, row, '╯', nil, borderStyle)
	row++

	if !f.expanded {
		return
	}
	start, end := f.visibleMatches()
	visible := end - start
	if visible < 0 {
		visible = 0
	}

	if row >= y+height {
		return
	}
	screen.SetContent(x, row, '╭', nil, borderStyle)
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, row, '─', nil, borderStyle)
	}
	screen.SetContent(x+width-1, row, '╮', nil, borderStyle)
	row++

	listTop := row
	for i := start; i < end; i++ {
		if row >= y+height-1 {
			break
		}
		style := tcell.StyleDefault.Background(bg).Foreground(fg)
		if i == f.selected {
			style = tcell.StyleDefault.Background(accent).Foreground(bg)
		}
		screen.SetContent(x, row, '│', nil, borderStyle)
		screen.SetContent(x+width-1, row, '│', nil, borderStyle)
		for col := x + 1; col < x+width-2; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
		col := x + 2
		textLimit := x + width - 3
		if textLimit < col {
			textLimit = col
		}
		for _, r := range f.matches[i] {
			if col >= textLimit {
				break
			}
			screen.SetContent(col, row, r, nil, style)
			col++
		}
		row++
	}

	if row < y+height {
		screen.SetContent(x, row, '╰', nil, borderStyle)
		for col := x + 1; col < x+width-1; col++ {
			screen.SetContent(col, row, '─', nil, borderStyle)
		}
		screen.SetContent(x+width-1, row, '╯', nil, borderStyle)
	}

	drawScrollbar(screen, x+width-2, listTop, visible, scrollMetrics{
		offset:  f.listOffset,
		visible: visible,
		total:   len(f.matches),
	}, true)
}

func (f *dropdownField) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return f.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		runes := f.runeValue()
		switch event.Key() {
		case tcell.KeyEscape:
			f.collapse()
		case tcell.KeyUp:
			f.moveSelection(-1)
		case tcell.KeyDown:
			f.moveSelection(1)
		case tcell.KeyLeft:
			if f.cursor > 0 {
				f.cursor--
			}
		case tcell.KeyRight:
			if f.cursor < len(runes) {
				f.cursor++
			}
		case tcell.KeyHome, tcell.KeyCtrlA:
			f.cursor = 0
		case tcell.KeyEnd, tcell.KeyCtrlE:
			f.cursor = len(runes)
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if f.cursor > 0 {
				runes = append(runes[:f.cursor-1], runes[f.cursor:]...)
				f.cursor--
				f.setRunes(runes)
			}
		case tcell.KeyDelete:
			if f.cursor < len(runes) {
				f.setRunes(append(runes[:f.cursor], runes[f.cursor+1:]...))
			}
		case tcell.KeyCtrlU:
			f.cursor = 0
			f.setRunes(nil)
		case tcell.KeyRune:
			r := event.Rune()
			runes = append(runes[:f.cursor], append([]rune{r}, runes[f.cursor:]...)...)
			f.cursor++
			f.setRunes(runes)
		}
	})
}

func collapseOpenDropdowns(fields ...*dropdownField) bool {
	for _, field := range fields {
		if field != nil && field.collapse() {
			return true
		}
	}
	return false
}

func dropdownFormCapture(fields ...*dropdownField) func(*tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		if event == nil {
			return event
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if collapseOpenDropdowns(fields...) {
				return nil
			}
		case tcell.KeyTab:
			// With several options showing, Tab walks them; a single option is
			// unambiguous enough to fill in. A closed dropdown lets Tab reach
			// the form, where it moves to the next field.
			for _, field := range fields {
				if field == nil || !field.HasFocus() || !field.expanded {
					continue
				}
				if len(field.matches) > 1 {
					field.moveSelection(1)
					return nil
				}
				if field.acceptSuggestion() {
					return nil
				}
			}
		case tcell.KeyEnter:
			// An open list owns Enter: it picks the highlighted option and
			// closes. Only a closed dropdown lets Enter reach the modal, where
			// it submits the form.
			for _, field := range fields {
				if field != nil && field.HasFocus() && field.acceptSuggestion() {
					return nil
				}
			}
		}
		return event
	}
}
