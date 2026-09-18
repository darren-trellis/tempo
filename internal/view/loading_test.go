package view

import (
	"strings"
	"testing"
	"time"

	"github.com/atterpac/jig/layout"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
)

func TestLoadingLabelCyclesFrames(t *testing.T) {
	first := loadingLabel(0)
	if first == loadingLabel(1) {
		t.Fatal("consecutive frames should differ")
	}
	if loadingLabel(len(loadingFrames)) != first {
		t.Fatal("frames should wrap around")
	}
	if !strings.Contains(first, "Loading") {
		t.Fatalf("hint bar label should say Loading, got %q", first)
	}
	if loadingSpinner(0) == "" || strings.Contains(loadingSpinner(0), "Loading") {
		t.Fatalf("spinner should be the glyph only, got %q", loadingSpinner(0))
	}
}

func TestQuietRefreshOmitsLoadingWord(t *testing.T) {
	a := &App{}
	a.SetViewRefreshing("workflows", true)
	got := a.loadingText()
	if got == "" || strings.Contains(got, "Loading") {
		t.Fatalf("auto-refresh should show only the spinner, got %q", got)
	}
	a.SetViewLoading("workers", true)
	if !strings.Contains(a.loadingText(), "Loading") {
		t.Fatal("an explicit load should still say Loading")
	}
	a.SetViewLoading("workers", false)
	if strings.Contains(a.loadingText(), "Loading") {
		t.Fatal("quiet refresh should return to spinner-only after labeled loads finish")
	}
	a.SetViewRefreshing("workflows", false)
	if a.loadingText() != "" {
		t.Fatal("spinner should stop once every refresh is done")
	}
}

func TestRenderLoadingStaysOnHintBar(t *testing.T) {
	a := &App{connected: true, menu: layout.NewMenu()}
	a.menu.SetRect(0, 0, 40, 1)
	a.menu.SetHints([]KeyHint{{Key: "r", Description: "Refresh"}})

	a.renderLoading(loadingLabel(0))
	if a.connectionChromeText() != theme.IconConnected {
		t.Fatalf("loading should not replace the connection glyph, got %q", a.connectionChromeText())
	}
	if a.connectionLabel != loadingLabel(0) {
		t.Fatalf("loading should live on the hint bar, got %q", a.connectionLabel)
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 1)
	a.menu.Draw(screen)
	a.drawHintLoading(screen)
	got := rowText(screen, 0, 40)
	if !strings.HasPrefix(strings.TrimLeft(got, " "), loadingLabel(0)) {
		t.Fatalf("Loading should cover the hint bar on the left, got %q", got)
	}
	if main, _, style, _ := screen.GetContent(1, 0); main == 'r' {
		t.Fatal("Loading should draw over key hints")
	} else if fg, _, _ := style.Decompose(); fg != theme.Fg() {
		t.Fatalf("Loading should use the app foreground, got %v", fg)
	}

	a.renderLoading("")
	if a.connectionLabel != "" {
		t.Fatal("clearing should remove the hint bar spinner")
	}
	if a.connectionChromeText() != theme.IconConnected {
		t.Fatal("connection glyph should still be connected")
	}
}

func TestSetViewLoadingTracksViewsIndependently(t *testing.T) {
	a := &App{}

	a.SetViewLoading("workflows", true)
	a.SetViewLoading("workers", true)
	if a.loadingText() == "" {
		t.Fatal("spinner should run while a view is loading")
	}

	a.SetViewLoading("workflows", false)
	if a.loadingText() == "" {
		t.Fatal("spinner should keep running while another view is loading")
	}

	a.SetViewLoading("workers", false)
	if a.loadingText() != "" {
		t.Fatal("spinner should stop once every view is done")
	}
}

func TestSetConnectedKeepsSpinnerThenRestores(t *testing.T) {
	a := &App{connected: true}
	a.SetViewLoading("workflows", true)
	a.renderLoading(a.loadingText())
	if a.connectionLabel == "" {
		t.Fatal("loading should show on the hint bar")
	}
	if a.connectionChromeText() != theme.IconConnected {
		t.Fatal("a connection update should keep the server glyph")
	}

	a.setConnected(true)
	if a.connectionLabel == "" {
		t.Fatal("a connection update should not hide the spinner")
	}

	a.SetViewLoading("workflows", false)
	a.renderLoading(a.loadingText())
	if a.connectionLabel != "" {
		t.Fatal("finished load should clear the hint bar spinner")
	}
}

func TestSetViewLoadingIsIdempotent(t *testing.T) {
	a := &App{}
	a.SetViewLoading("workflows", true)
	a.SetViewLoading("workflows", true)
	a.SetViewLoading("workflows", false)
	if a.loadingText() != "" {
		t.Fatal("repeated starts should not need repeated stops")
	}
}

func TestSetViewLoadingDoesNotBlockBeforeRun(t *testing.T) {
	a := &App{app: layout.NewApp(layout.AppConfig{})}
	done := make(chan struct{})
	go func() {
		a.SetViewLoading("workflows", true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SetViewLoading should not wait for the UI loop")
	}
}

func rowText(screen tcell.SimulationScreen, y, width int) string {
	var b strings.Builder
	for x := 0; x < width; x++ {
		ch, _, _, _ := screen.GetContent(x, y)
		if ch != 0 {
			b.WriteRune(ch)
		}
	}
	return b.String()
}
