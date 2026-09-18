package view

import (
	"strconv"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

type chromeSeg struct {
	text  string
	color tcell.Color
}

type chromePanel struct {
	*components.Panel
}

func newChromePanel(_ *App) *chromePanel {
	p := &chromePanel{Panel: components.NewPanel()}
	p.SetTitleAlign(components.TitleAlignLeft)
	return p
}

func (a *App) statusBarSegments() []chromeSeg {
	if a == nil {
		return nil
	}
	var segs []chromeSeg
	add := func(text string, color tcell.Color) {
		if text == "" {
			return
		}
		if len(segs) > 0 {
			segs = append(segs, chromeSeg{text: " | ", color: theme.FgDim()})
		}
		segs = append(segs, chromeSeg{text: text, color: color})
	}
	add(a.connectionChromeText(), a.connectionChromeColor())
	if icon := a.codecChromeIcon(); icon != "" {
		add(icon, a.codecChromeColor())
	}
	add(a.autoRefreshChromeText(), a.autoRefreshChromeColor())
	if wl, ok := a.workflowList(); ok && wl.workflowsActive() {
		add(workflowTreeChromeText(wl.workflowTreeMode), theme.Fg())
	}
	add(a.profileTitle(), theme.Fg())
	return segs
}

type workflowStatBadge struct {
	Text    string
	Variant components.BadgeVariant
}

func workflowTreeChromeText(tree bool) string {
	if tree {
		return theme.IconNamespace
	}
	return theme.IconList
}

func (a *App) workflowChromeBadges() []workflowStatBadge {
	if a == nil || !a.chromeStatsOn {
		return nil
	}
	var out []workflowStatBadge
	if wl, ok := a.workflowList(); ok && wl.workflowsActive() {
		out = append(out,
			workflowStatBadge{Text: formatCount(wl.loadedWorkflowCount()) + " Loaded", Variant: components.BadgeDefault},
			workflowStatBadge{Text: formatCount(wl.displayedWorkflowCount()) + " Displayed", Variant: components.BadgePrimary},
		)
	}
	return append(out, workflowStatBadges(a.chromeStats)...)
}

func workflowStatBadges(stats WorkflowStats) []workflowStatBadge {
	items := []struct {
		count   int
		label   string
		variant components.BadgeVariant
	}{
		{stats.Running, "Running", components.BadgeInfo},
		{stats.Completed, "Completed", components.BadgeSuccess},
		{stats.Failed, "Failed", components.BadgeError},
		{stats.Canceled, "Canceled", components.BadgeWarning},
		{stats.Terminated, "Terminated", components.BadgeError},
		{stats.TimedOut, "Timed Out", components.BadgeWarning},
		{stats.ContinuedAsNew, "Continued as New", components.BadgePrimary},
	}
	out := make([]workflowStatBadge, 0, len(items))
	for _, item := range items {
		if item.count == 0 {
			continue
		}
		out = append(out, workflowStatBadge{
			Text:    formatCount(item.count) + " " + item.label,
			Variant: item.variant,
		})
	}
	return out
}

func formatCount(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	pre := len(s) % 3
	if pre == 0 {
		pre = 3
	}
	out = append(out, s[:pre]...)
	for i := pre; i < len(s); i += 3 {
		out = append(out, ',')
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}

func (a *App) drawBottomChrome(screen tcell.Screen) {
	if a == nil || a.menu == nil || screen == nil || a.promptActive() {
		return
	}
	x, y, width, height := a.menu.GetInnerRect()
	if width < 8 || height < 1 {
		return
	}

	segs := a.statusBarSegments()
	segWidth := 0
	for _, seg := range segs {
		segWidth += len([]rune(seg.text))
	}

	var badges []*components.Badge
	badgeWidth := 0
	if !a.modalHintsOn {
		for i, spec := range a.workflowChromeBadges() {
			badge := components.NewBadge(spec.Text).SetVariant(spec.Variant).SetPill(true)
			if i > 0 {
				badgeWidth++
			}
			badgeWidth += badge.Width()
			badges = append(badges, badge)
		}
	}

	sep := ""
	if segWidth > 0 && badgeWidth > 0 {
		sep = " | "
	}
	total := segWidth + len([]rune(sep)) + badgeWidth
	if total == 0 {
		return
	}
	col := x + width - total - 1
	if col < x {
		col = x
	}
	for _, seg := range segs {
		style := tcell.StyleDefault.Background(theme.Bg()).Foreground(seg.color)
		for _, r := range seg.text {
			if col >= x+width-1 {
				return
			}
			screen.SetContent(col, y, r, nil, style)
			col++
		}
	}
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.FgDim())
	for _, r := range sep {
		if col >= x+width-1 {
			return
		}
		screen.SetContent(col, y, r, nil, style)
		col++
	}
	for _, badge := range badges {
		w := badge.Width()
		badge.SetRect(col, y, w, 1)
		badge.Draw(screen)
		col += w + 1
	}
}

func (a *App) connectionChromeText() string {
	if a != nil && a.connected {
		return theme.IconConnected
	}
	return theme.IconDisconnected
}

func (a *App) connectionChromeColor() tcell.Color {
	if a != nil && a.connected {
		return theme.Success()
	}
	return theme.Error()
}

func (a *App) codecChromeIcon() string {
	if a == nil {
		return ""
	}
	return a.chromeCodecIcon
}

func (a *App) codecChromeColor() tcell.Color {
	if a != nil && a.chromeCodecColor != nil {
		return a.chromeCodecColor()
	}
	return theme.FgDim()
}

func (a *App) autoRefreshChromeText() string {
	if a != nil && a.currentAutoRefresh() {
		return theme.IconRefresh
	}
	return theme.IconPause
}

func (a *App) autoRefreshChromeColor() tcell.Color {
	if a != nil && a.currentAutoRefresh() {
		return theme.Success()
	}
	return theme.FgDim()
}

func (a *App) currentAutoRefresh() bool {
	if a == nil {
		return false
	}
	switch v := a.currentContent().(type) {
	case *WorkflowList:
		if v.taskQueuesActive() && v.taskQueues != nil {
			return v.taskQueues.autoRefresh
		}
		if v.workflowsActive() {
			return v.autoRefresh
		}
	case *NamespaceList:
		return v.autoRefresh
	case *TaskQueueView:
		return v.autoRefresh
	}
	return false
}
