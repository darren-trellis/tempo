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
	app *App
}

func newChromePanel(app *App) *chromePanel {
	p := &chromePanel{Panel: components.NewPanel(), app: app}
	p.SetTitleAlign(components.TitleAlignLeft)
	return p
}

func (p *chromePanel) Draw(screen tcell.Screen) {
	if p == nil || p.Panel == nil {
		return
	}
	p.Panel.Draw(screen)
	drawTrailingChrome(screen, p.Panel, p.app)
}

func drawTrailingChrome(screen tcell.Screen, panel *components.Panel, app *App) {
	if screen == nil || panel == nil || app == nil {
		return
	}
	x, y, width, _ := panel.GetInnerRect()
	if width < 8 {
		return
	}
	segs := app.tabChromeSegments()
	if len(segs) == 0 {
		return
	}
	total := 0
	for _, seg := range segs {
		total += len([]rune(seg.text))
	}
	col := x + width - total - 2
	if col < x+2 {
		col = x + 2
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
}

func (a *App) tabChromeSegments() []chromeSeg {
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
	return segs
}

type crumbStatBadge struct {
	Text    string
	Variant components.BadgeVariant
}

func crumbStatBadges(stats WorkflowStats) []crumbStatBadge {
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
	out := make([]crumbStatBadge, 0, len(items))
	for _, item := range items {
		if item.count == 0 {
			continue
		}
		out = append(out, crumbStatBadge{
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

func (a *App) drawCrumbStats(screen tcell.Screen) {
	if a == nil || !a.chromeStatsOn || a.app == nil || screen == nil {
		return
	}
	crumbs := a.app.Crumbs()
	if crumbs == nil {
		return
	}
	specs := crumbStatBadges(a.chromeStats)
	if len(specs) == 0 {
		return
	}
	x, y, width, height := crumbs.GetRect()
	if width < 8 || height < 1 {
		return
	}
	badges := make([]*components.Badge, len(specs))
	total := 0
	for i, spec := range specs {
		badge := components.NewBadge(spec.Text).SetVariant(spec.Variant).SetPill(true)
		badges[i] = badge
		if i > 0 {
			total++
		}
		total += badge.Width()
	}
	col := x + width - total - 1
	if col < x {
		col = x
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
