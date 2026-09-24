package view

import (
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

const statusMessageHold = 3 * time.Second

func (a *App) queueStatusMessage(message string) {
	if a == nil || a.app == nil {
		a.setStatusMessage(message)
		return
	}
	go a.app.QueueUpdateDraw(func() {
		a.setStatusMessage(message)
	})
}

func (a *App) setStatusMessage(message string) {
	if a == nil {
		return
	}
	a.statusMu.Lock()
	a.statusText = message
	if a.statusClear != nil {
		a.statusClear.Stop()
		a.statusClear = nil
	}
	if message != "" {
		a.statusClear = time.AfterFunc(statusMessageHold, func() {
			a.statusMu.Lock()
			a.statusText = ""
			a.statusClear = nil
			a.statusMu.Unlock()
			if a.app != nil {
				go a.app.QueueUpdateDraw(func() {})
			}
		})
	}
	a.statusMu.Unlock()
}

func (a *App) drawHintLoading(screen tcell.Screen) {
	if a == nil || a.menu == nil || screen == nil || a.promptActive() {
		return
	}
	if a.hintBarMessage() != "" {
		return
	}
	label := a.connectionLabel
	if label == "" {
		return
	}
	x, y, width, height := a.menu.GetInnerRect()
	if width < 1 || height < 1 {
		return
	}
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	for col := x; col < x+width; col++ {
		screen.SetContent(col, y, ' ', nil, style)
	}
	col := x + 1
	for _, r := range label {
		if col >= x+width-1 {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col++
	}
}

func (a *App) drawHintStatus(screen tcell.Screen) {
	if a == nil || a.menu == nil || screen == nil {
		return
	}
	a.statusMu.Lock()
	text := a.statusText
	a.statusMu.Unlock()
	if text == "" {
		return
	}
	x, y, width, height := a.menu.GetInnerRect()
	if width < 1 || height < 1 {
		return
	}
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg())
	for col := x; col < x+width; col++ {
		screen.SetContent(col, y, ' ', nil, style)
	}
	col := x + 1
	for _, r := range text {
		if col >= x+width-1 {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col++
	}
}

func (a *App) setSearchStatus(text string) {
	if a == nil {
		return
	}
	a.searchStatus = text
}

func (a *App) hintBarMessage() string {
	if a == nil {
		return ""
	}
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	return a.statusText
}

func (a *App) ShowToastError(message string) {
	a.queueStatusMessage(message)
}

func (a *App) ShowToastWarning(message string) {
	a.queueStatusMessage(message)
}

func (a *App) ShowToastSuccess(message string) {
	a.queueStatusMessage(message)
}

func (a *App) ToastSuccess(message string) {
	a.setStatusMessage(message)
}

func (a *App) ToastError(message string) {
	a.setStatusMessage(message)
}

func (a *App) ToastWarning(message string) {
	a.setStatusMessage(message)
}

func (a *App) ToastInfo(message string) {
	a.setStatusMessage(message)
}
