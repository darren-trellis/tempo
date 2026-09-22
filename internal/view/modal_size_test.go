package view

import (
	"fmt"
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestResizableModalToggleMaximize(t *testing.T) {
	modal := newResizableModal(components.ModalConfig{
		Title:     "IO",
		MinWidth:  20,
		MinHeight: 10,
	})
	modal.SetRect(0, 0, 80, 24)
	if modal.maximized {
		t.Fatal("modal should start restored")
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.Draw(screen)
	_, _, restW, restH := modal.GetPanel().GetRect()

	modal.toggleMaximize()
	modal.Draw(screen)
	_, _, maxW, maxH := modal.GetPanel().GetRect()
	if maxW <= restW || maxH <= restH {
		t.Fatalf("maximize should grow the panel: rest=%dx%d max=%dx%d", restW, restH, maxW, maxH)
	}
	if maxW != 80 || maxH != 24 {
		t.Fatalf("maximized size should be full screen, got %dx%d", maxW, maxH)
	}

	modal.toggleMaximize()
	modal.Draw(screen)
	_, _, backW, backH := modal.GetPanel().GetRect()
	if backW != restW || backH != restH {
		t.Fatalf("restore should return to %dx%d, got %dx%d", restW, restH, backW, backH)
	}
}

// The highlighted row is padded to the viewport, and it sits above a much wider
// line. Scrolling has to be able to leave that bar, the way the tertiary input
// and output tabs already can.
func TestFramelessModalScrollsPastTheHighlight(t *testing.T) {
	modal := newResizableModal(components.ModalConfig{
		Title:     "IO",
		MinWidth:  80,
		MinHeight: 16,
	})
	modal.frameless = true
	modal.SetRect(0, 0, 160, 40)

	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	setTextViewWrap(view, false)
	attachTextViewScrollbar(view, nil)
	tree := newJSONTreeSelection(view)

	var payload strings.Builder
	payload.WriteString("{")
	for i := 0; i < 30; i++ {
		if i > 0 {
			payload.WriteByte(',')
		}
		fmt.Fprintf(&payload, `"k%02d":1`, i)
	}
	payload.WriteString(`,"note":"`)
	payload.WriteString(strings.Repeat("x", 200))
	payload.WriteString(`"}`)
	if !tree.setContent(payload.String(), true) {
		t.Fatal("expected selectable JSON tree")
	}
	modal.SetContent(view)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(160, 40)
	modal.Draw(screen)
	modal.Draw(screen)

	scrollTextViewHoriz(view, 24)
	modal.Draw(screen)
	if _, col := view.GetScrollOffset(); col < 24 {
		t.Fatalf("horizontal scroll should move past the highlight, col=%d", col)
	}
	left, top, width, _ := view.GetInnerRect()
	_, _, onBar, _ := screen.GetContent(left, top)
	_, barBG, _ := onBar.Decompose()
	_, _, pastBar, _ := screen.GetContent(left+width-1, top)
	_, pastBG, _ := pastBar.Decompose()
	if pastBG == barBG {
		t.Fatal("horizontal scroll should reveal columns past the highlighted row")
	}

	scrollTextView(view, 8)
	row, col := view.GetScrollOffset()
	modal.Draw(screen)
	modal.Draw(screen)
	gotRow, gotCol := view.GetScrollOffset()
	if gotRow != row || gotCol != col {
		t.Fatalf("draw snapped scroll back to the highlight: want row=%d col=%d, got row=%d col=%d", row, col, gotRow, gotCol)
	}
}
