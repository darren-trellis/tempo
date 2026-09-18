package view

import (
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

func drawCommandSuggestionBox(screen tcell.Screen, x, y, width, height int, state *commandCompletionState) {
	if screen == nil || state == nil || width < 8 || height < 2 {
		return
	}
	bg := theme.Bg()
	fg := theme.Fg()
	dim := theme.FgDim()
	accent := theme.Accent()
	border := theme.BorderFocus()
	borderStyle := tcell.StyleDefault.Background(bg).Foreground(border)
	titleStyle := tcell.StyleDefault.Background(bg).Foreground(accent)
	clearStyle := tcell.StyleDefault.Background(bg)
	textStyle := tcell.StyleDefault.Background(bg).Foreground(fg)
	helpStyle := tcell.StyleDefault.Background(bg).Foreground(dim)
	selectedStyle := tcell.StyleDefault.Background(accent).Foreground(bg)

	fillRectCells(screen, x, y, width, height, clearStyle)
	drawRoundedTitleBorder(screen, x, y, width, height, "suggestions", borderStyle, titleStyle)

	innerX, innerY := x+1, y+1
	innerW, innerH := width-2, height-2
	if innerW < 1 || innerH < 1 {
		return
	}
	state.ensureVisible(innerH)
	listW := innerW
	if listW > 1 {
		listW--
	}
	for i := 0; i < innerH; i++ {
		idx := state.scroll + i
		row := innerY + i
		if idx >= len(state.items) {
			fillRectCells(screen, innerX, row, innerW, 1, clearStyle)
			screen.SetContent(x, row, '│', nil, borderStyle)
			screen.SetContent(x+width-1, row, '│', nil, borderStyle)
			continue
		}
		item := state.items[idx]
		selected := state.selected != nil && *state.selected == idx
		marker := "  "
		if selected {
			marker = "▸ "
		}
		style := textStyle
		hStyle := helpStyle
		if selected {
			style = selectedStyle
			hStyle = selectedStyle
		}
		fillRectCells(screen, innerX, row, innerW, 1, style)
		screen.SetContent(x, row, '│', nil, borderStyle)
		screen.SetContent(x+width-1, row, '│', nil, borderStyle)
		label := padRightRunes(marker+item.Label, 20)
		col := drawRunes(screen, innerX+1, row, listW-1, label, style)
		if item.Help != "" && col+1 < innerX+listW {
			drawRunes(screen, col+1, row, innerX+listW-col-1, item.Help, hStyle)
		}
	}
	drawScrollbar(screen, x+width-2, innerY, innerH, scrollMetrics{
		offset:  state.scroll,
		visible: innerH,
		total:   len(state.items),
	}, true)
}

func drawRoundedTitleBorder(screen tcell.Screen, x, y, width, height int, title string, border, titleStyle tcell.Style) {
	if width < 2 || height < 2 {
		return
	}
	screen.SetContent(x, y, '╭', nil, border)
	screen.SetContent(x+width-1, y, '╮', nil, border)
	col := x + 1
	if title != "" && width > 6 {
		screen.SetContent(col, y, '─', nil, border)
		col++
		screen.SetContent(col, y, ' ', nil, border)
		col++
		for _, r := range title {
			if col >= x+width-3 {
				break
			}
			screen.SetContent(col, y, r, nil, titleStyle)
			col++
		}
		if col < x+width-1 {
			screen.SetContent(col, y, ' ', nil, border)
			col++
		}
	}
	for col < x+width-1 {
		screen.SetContent(col, y, '─', nil, border)
		col++
	}
	bottom := y + height - 1
	screen.SetContent(x, bottom, '╰', nil, border)
	screen.SetContent(x+width-1, bottom, '╯', nil, border)
	for c := x + 1; c < x+width-1; c++ {
		screen.SetContent(c, bottom, '─', nil, border)
	}
	for row := y + 1; row < bottom; row++ {
		screen.SetContent(x, row, '│', nil, border)
		screen.SetContent(x+width-1, row, '│', nil, border)
	}
}

func fillRectCells(screen tcell.Screen, x, y, w, h int, style tcell.Style) {
	if screen == nil || w <= 0 || h <= 0 {
		return
	}
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
	}
}

func drawRunes(screen tcell.Screen, x, y, width int, text string, style tcell.Style) int {
	col := x
	if width <= 0 {
		return col
	}
	for _, r := range text {
		if col >= x+width {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col++
	}
	return col
}

func padRightRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) >= n {
		return string(runes[:n])
	}
	return s + padSpaces(n-len(runes))
}
