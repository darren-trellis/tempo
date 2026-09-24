package view

import (
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/theme/themes"
	"github.com/gdamore/tcell/v2"
)

func useTheme(t *testing.T, name string) {
	t.Helper()
	selected := themes.Get(name)
	if selected == nil {
		t.Fatalf("unknown theme %s", name)
	}
	theme.SetProvider(selected)
}

// jig paints the highlighted row with a hardcoded black foreground, which
// vanishes wherever the accent is dark. Every shipped theme should clear the
// WCAG AA bar instead.
func TestRowSelectionIsReadableInEveryTheme(t *testing.T) {
	for _, name := range themes.Names() {
		t.Run(name, func(t *testing.T) {
			useTheme(t, name)
			fg, bg, _ := rowSelectionStyle().Decompose()
			if bg != theme.Accent() {
				t.Fatalf("selection background = %v, want accent %v", bg, theme.Accent())
			}
			if got := contrastRatio(bg, fg); got < minReadableContrast {
				t.Errorf("selection contrast %.2f is below %.1f (fg=%v bg=%v)", got, minReadableContrast, fg, bg)
			}
		})
	}
}

// The reported bug: Dracula Light drew black on a dark crimson accent. Where
// the accent is dark the row should pick the light side of the theme, matching
// the active filter chip.
func TestRowSelectionUsesLightForegroundOnDarkAccents(t *testing.T) {
	for _, name := range []string{"dracula-light", "github-light", "gruvbox-light"} {
		t.Run(name, func(t *testing.T) {
			useTheme(t, name)
			fg, bg, _ := rowSelectionStyle().Decompose()
			if fg != theme.Bg() {
				t.Errorf("selection foreground = %v, want theme background %v", fg, theme.Bg())
			}
			if relativeLuminance(fg) <= relativeLuminance(bg) {
				t.Errorf("foreground %v should be lighter than the accent %v", fg, bg)
			}
			if contrastRatio(bg, fg) <= contrastRatio(bg, tcell.ColorBlack) {
				t.Errorf("chosen foreground should beat the black jig hardcodes")
			}
		})
	}
}

// The row highlight and the active chip share one foreground so they cannot
// drift apart.
func TestActiveChipAndRowSelectionShareForeground(t *testing.T) {
	for _, name := range themes.Names() {
		t.Run(name, func(t *testing.T) {
			useTheme(t, name)
			fg, _, _ := rowSelectionStyle().Decompose()
			if fg != accentTextColor() {
				t.Errorf("row foreground %v differs from the chip foreground %v", fg, accentTextColor())
			}
		})
	}
}

func TestContrastRatioBounds(t *testing.T) {
	if got := contrastRatio(tcell.ColorBlack, tcell.ColorWhite); got < 20.9 || got > 21.1 {
		t.Errorf("black vs white = %.2f, want ~21", got)
	}
	if got := contrastRatio(tcell.ColorRed, tcell.ColorRed); got < 0.99 || got > 1.01 {
		t.Errorf("identical colors = %.2f, want 1", got)
	}
}

// jig reapplies its own selection style on every draw, so the override has to
// live on the cell where tview gives it priority.
func TestSelectionStyleSurvivesTableDraw(t *testing.T) {
	useTheme(t, "dracula-light")

	table := components.NewTable()
	table.SetHeaders("NAME", "STATUS")
	table.AddRow("first", "Running")
	table.AddRow("second", "Failed")
	table.SelectRow(1)

	scroll := newCharScrollView(table, func() int { return 40 })
	scroll.SetRect(0, 0, 40, 6)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 6)
	scroll.Draw(screen)

	row, _ := table.GetSelection()
	cell := table.GetCell(row, 0)
	if cell == nil {
		t.Fatalf("no cell at selected row %d", row)
	}
	fg, bg, _ := cell.SelectedStyle.Decompose()
	if bg != theme.Accent() {
		t.Errorf("cell selection background = %v, want accent %v", bg, theme.Accent())
	}
	if fg != theme.Bg() {
		t.Errorf("cell selection foreground = %v, want %v", fg, theme.Bg())
	}
	if fg == tcell.ColorBlack {
		t.Error("cell still uses the black foreground jig hardcodes")
	}
}

func tableShowsSelectedRow(t *testing.T, table *components.Table) bool {
	t.Helper()
	if table == nil {
		return false
	}
	if rows, _ := table.GetSelectable(); !rows {
		return false
	}
	table.SetRect(0, 0, 40, 8)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 8)
	applyRowSelectionStyle(table)
	table.Draw(screen)

	wantBg := theme.Accent()
	for y := 0; y < 8; y++ {
		for x := 0; x < 40; x++ {
			_, _, style, _ := screen.GetContent(x, y)
			if _, bg, _ := style.Decompose(); bg == wantBg {
				return true
			}
		}
	}
	return false
}

func TestWorkflowAndActivityRowsStayHighlightedWhenUnfocused(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.keepDataOnStart = true
	wl.loadMockData()
	if wl.table.RowCount() == 0 {
		t.Fatal("expected mock workflows")
	}
	wl.table.SelectRow(0)
	wl.eventTable.ClearRows()
	wl.eventTable.SetHeaders("ACTIVITY", "STATUS")
	wl.eventTable.AddRow("ValidateOrder", "Completed")
	wl.eventTable.AddRow("Charge", "Failed")
	wl.eventTable.SelectRow(0)
	wl.setPreviewKind(previewActivities)

	wl.focusPane = focusTimeline
	wl.applyFocusStyles()

	if rows, _ := wl.table.GetSelectable(); !rows {
		t.Fatal("the workflow row should stay selectable when another pane is focused")
	}
	if rows, _ := wl.eventTable.GetSelectable(); !rows {
		t.Fatal("the activity row should stay selectable when another pane is focused")
	}
	if !tableShowsSelectedRow(t, wl.table) {
		t.Fatal("the workflow highlight should stay painted when the list is unfocused")
	}
	if !tableShowsSelectedRow(t, wl.eventTable) {
		t.Fatal("the activity highlight should stay painted when the list is unfocused")
	}

	wl.previewKind = previewHierarchy
	wl.applyFocusStyles()
	if rows, _ := wl.eventTable.GetSelectable(); rows {
		t.Fatal("the activity table should not stay selectable on the hierarchy tab")
	}
}
