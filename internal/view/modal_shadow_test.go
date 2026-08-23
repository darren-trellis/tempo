package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
)

func TestDrawModalShadowDarkensOffsetCells(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 20)

	fill := tcell.StyleDefault.
		Background(tcell.NewRGBColor(80, 80, 80)).
		Foreground(tcell.NewRGBColor(200, 200, 200))
	for row := 0; row < 20; row++ {
		for col := 0; col < 40; col++ {
			screen.SetContent(col, row, 'A', nil, fill)
		}
	}

	panel := components.NewPanel()
	panel.SetRect(5, 3, 10, 6)
	drawModalShadow(screen, panel)

	insideMain, _, insideStyle, _ := screen.GetContent(6, 4)
	if insideMain != 'A' {
		t.Fatalf("panel interior rune = %q", string(insideMain))
	}
	_, insideBg, _ := insideStyle.Decompose()
	ir, ig, ib := insideBg.RGB()
	if ir != 80 || ig != 80 || ib != 80 {
		t.Fatalf("panel interior should stay untouched, bg=%d,%d,%d", ir, ig, ib)
	}

	shadowMain, _, shadowStyle, _ := screen.GetContent(15, 4)
	if shadowMain != 'A' {
		t.Fatalf("shadow should keep underlying rune, got %q", string(shadowMain))
	}
	_, shadowBg, _ := shadowStyle.Decompose()
	sr, sg, sb := shadowBg.RGB()
	if sr != 40 || sg != 40 || sb != 40 {
		t.Fatalf("shadow bg=%d,%d,%d want 40,40,40", sr, sg, sb)
	}
}

func TestDrawModalShadowSkipsFullscreenPanel(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(20, 10)

	fill := tcell.StyleDefault.Background(tcell.NewRGBColor(80, 80, 80))
	for row := 0; row < 10; row++ {
		for col := 0; col < 20; col++ {
			screen.SetContent(col, row, 'B', nil, fill)
		}
	}

	panel := components.NewPanel()
	panel.SetRect(0, 0, 20, 10)
	drawModalShadow(screen, panel)

	_, _, style, _ := screen.GetContent(19, 9)
	_, bg, _ := style.Decompose()
	r, g, b := bg.RGB()
	if r != 80 || g != 80 || b != 80 {
		t.Fatalf("fullscreen panel should not paint a shadow, bg=%d,%d,%d", r, g, b)
	}
}
