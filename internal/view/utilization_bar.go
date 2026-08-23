package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	utilizationBarFilled = "█"
	utilizationBarEmpty  = "░"
	// Utilization ratios where a bar turns amber, then red.
	utilizationWarn = 0.6
	utilizationHigh = 0.85
	// Room reserved for the label and the trailing percentage.
	utilizationLabelWidth   = 4
	utilizationPercentWidth = 5
)

// utilizationBar draws a labeled utilization meter across the full width of its rect.
type utilizationBar struct {
	*tview.Box
	label string
	value float32
	known bool
}

func newUtilizationBar(label string) *utilizationBar {
	box := tview.NewBox()
	box.SetBackgroundColor(theme.Bg())
	return &utilizationBar{Box: box, label: label}
}

// setValue records the ratio to draw; known reports whether the worker sent one.
func (b *utilizationBar) setValue(value float32, known bool) {
	b.value = value
	b.known = known
}

func (b *utilizationBar) Draw(screen tcell.Screen) {
	b.Box.SetBackgroundColor(theme.Bg())
	b.Box.DrawForSubclass(screen, b)
	x, y, width, height := b.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	tview.Print(screen, renderUtilizationBar(b.label, b.value, b.known, width), x, y, width, tview.AlignLeft, theme.Fg())
}

// renderUtilizationBar lays out "CPU ███░░░ 41%" for the given width. An unknown
// value keeps the label and a dash so the row does not look like zero load.
func renderUtilizationBar(label string, value float32, known bool, width int) string {
	head := fmt.Sprintf("[%s]%-*s[-]", theme.TagFgDim(), utilizationLabelWidth, label)
	if !known {
		return head + fmt.Sprintf("[%s]%*s[-]", theme.TagFgDim(), utilizationPercentWidth, "-")
	}
	ratio := clampRatio(value)
	tail := fmt.Sprintf("[%s]%*s[-]", theme.TagFg(), utilizationPercentWidth, formatPercent(ratio))
	cells := width - utilizationLabelWidth - utilizationPercentWidth
	if cells < 1 {
		return head + tail
	}
	filled := int(ratio*float64(cells) + 0.5)
	if filled == 0 && ratio > 0 {
		filled = 1
	}
	return head + fmt.Sprintf("[%s]%s[%s]%s[-]",
		utilizationTag(ratio),
		strings.Repeat(utilizationBarFilled, filled),
		theme.TagFgMuted(),
		strings.Repeat(utilizationBarEmpty, cells-filled),
	) + tail
}

func utilizationTag(ratio float64) string {
	switch {
	case ratio >= utilizationHigh:
		return theme.TagError()
	case ratio >= utilizationWarn:
		return theme.TagWarning()
	default:
		return theme.TagSuccess()
	}
}

func clampRatio(value float32) float64 {
	ratio := float64(value)
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

func formatPercent(ratio float64) string {
	return fmt.Sprintf("%.0f%%", ratio*100)
}
