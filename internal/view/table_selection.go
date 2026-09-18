package view

import (
	"math"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// minReadableContrast is the WCAG AA ratio for normal text.
const minReadableContrast = 4.5

// accentTextColor returns the foreground to draw on top of the accent color,
// used by both the highlighted row and the active filter chips so they stay in
// step. The theme's own background is preferred, which is what gives light
// themes their light-on-dark chips, and only when that is illegible (accents
// too close to the page background, as in Rose Pine Dawn) does it fall back to
// whatever reads best.
func accentTextColor() tcell.Color {
	accent := theme.Accent()
	if contrastRatio(accent, theme.Bg()) >= minReadableContrast {
		return theme.Bg()
	}
	return contrastingColor(accent, theme.Bg(), theme.Fg(), tcell.ColorBlack, tcell.ColorWhite)
}

// rowSelectionStyle styles the highlighted row. jig paints it with a hardcoded
// black foreground on the accent background, which disappears on light themes
// whose accent is dark, Dracula Light's #a3144d being one.
func rowSelectionStyle() tcell.Style {
	return tcell.StyleDefault.
		Background(theme.Accent()).
		Foreground(accentTextColor()).
		Bold(true)
}

// applyRowSelectionStyle overrides jig's table-wide selection style on the
// highlighted row. tview gives a cell's own SelectedStyle priority, and jig
// never touches it, so this survives jig reapplying its style on every draw.
func applyRowSelectionStyle(p tview.Primitive) {
	table, ok := p.(*components.Table)
	if !ok || table == nil {
		return
	}
	row, _ := table.GetSelection()
	if row < 0 || row >= table.GetRowCount() {
		return
	}
	style := rowSelectionStyle()
	for col := 0; col < table.GetColumnCount(); col++ {
		if cell := table.GetCell(row, col); cell != nil {
			cell.SetSelectedStyle(style)
		}
	}
}

// contrastingColor returns whichever candidate is easiest to read on bg.
func contrastingColor(bg tcell.Color, candidates ...tcell.Color) tcell.Color {
	best := tcell.ColorBlack
	bestRatio := -1.0
	for _, c := range candidates {
		if ratio := contrastRatio(bg, c); ratio > bestRatio {
			best, bestRatio = c, ratio
		}
	}
	return best
}

// contrastRatio is the WCAG contrast ratio between two colors, from 1 for
// identical colors to 21 for black on white.
func contrastRatio(a, b tcell.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(c tcell.Color) float64 {
	r, g, b := c.RGB()
	return 0.2126*linearizeChannel(r) + 0.7152*linearizeChannel(g) + 0.0722*linearizeChannel(b)
}

func linearizeChannel(v int32) float64 {
	s := float64(v) / 255
	if s <= 0.03928 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}
