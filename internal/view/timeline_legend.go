package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type timelineLegendItem struct {
	glyph string
	color tcell.Color
	label string
}

func timelineLegendStatuses() []timelineLegendItem {
	statuses := []string{
		"Completed",
		"Failed",
		"Fired",
		"Received",
		"TimedOut",
		"Canceled",
		"Terminated",
		"Running",
		"Pending",
	}
	items := make([]timelineLegendItem, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, timelineLegendItem{
			glyph: timelineStatusGlyph(status),
			color: timelineStatusColor(status),
			label: timelineStatusLabel(status),
		})
	}
	return items
}

func timelineLegendTypes() []temporal.EventGroupType {
	return []temporal.EventGroupType{
		temporal.GroupActivity,
		temporal.GroupTimer,
		temporal.GroupChildWorkflow,
		temporal.GroupSignal,
		temporal.GroupMarker,
		temporal.GroupOther,
	}
}

func timelineLegendTypeItems() []timelineLegendItem {
	types := timelineLegendTypes()
	items := make([]timelineLegendItem, 0, len(types))
	for _, typ := range types {
		items = append(items, timelineLegendItem{
			glyph: string(timelineTypeGlyph(typ)),
			color: timelineTypeColor(typ),
			label: timelineTypeLabel(typ),
		})
	}
	return items
}

func formatTimelineLegendColumn(title string, items []timelineLegendItem, colorLabels bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[%s::b]%s[-:-:-]\n\n", theme.TagAccent(), title))
	for _, item := range items {
		hex := theme.ColorToHex(item.color)
		glyph := item.glyph
		if !colorLabels && len([]rune(glyph)) < 2 {
			glyph += " "
		}
		if colorLabels {
			b.WriteString(fmt.Sprintf("[%s]%s  %s[-]\n", hex, glyph, item.label))
		} else {
			b.WriteString(fmt.Sprintf("[%s]%s[-]  %s\n", hex, glyph, item.label))
		}
	}
	return b.String()
}

type TimelineLegendModal struct {
	*shadowedModal
	closeFunc func()
}

func NewTimelineLegendModal() *TimelineLegendModal {
	statusItems := timelineLegendStatuses()
	typeItems := timelineLegendTypeItems()
	rows := len(statusItems)
	if len(typeItems) > rows {
		rows = len(typeItems)
	}
	m := &TimelineLegendModal{
		shadowedModal: newModal(components.ModalConfig{
			Title:    fmt.Sprintf("%s Timeline Legend", theme.IconEvent),
			Width:    58,
			Height:   rows + 8,
			Backdrop: true,
		}),
	}
	m.setup(statusItems, typeItems)
	return m
}

func (m *TimelineLegendModal) setup(statusItems, typeItems []timelineLegendItem) {
	left := tview.NewTextView().SetDynamicColors(true)
	left.SetBackgroundColor(theme.Bg())
	left.SetText(formatTimelineLegendColumn("Status", statusItems, false))

	right := tview.NewTextView().SetDynamicColors(true)
	right.SetBackgroundColor(theme.Bg())
	right.SetText(formatTimelineLegendColumn("Event Types", typeItems, true))

	columns := tview.NewFlex().SetDirection(tview.FlexColumn)
	columns.SetBackgroundColor(theme.Bg())
	columns.AddItem(left, 0, 1, true)
	columns.AddItem(right, 0, 1, false)

	m.SetContent(columns)
	m.SetHints([]components.KeyHint{
		{Key: "Esc", Description: "Close"},
	})
}

func (m *TimelineLegendModal) SetOnClose(fn func()) {
	m.closeFunc = fn
	m.Modal.SetOnClose(fn)
}
