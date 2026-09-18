package view

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// pressModal sends a key to whatever modal is on top, through the same handler
// the terminal would.
func pressModal(a *App, ev *tcell.EventKey) {
	current := a.app.Pages().Current()
	if current == nil {
		return
	}
	if handler := current.InputHandler(); handler != nil {
		handler(ev, func(tview.Primitive) {})
	}
}

// TestThemeSelectorEscapeDiscardsPreview covers the live-preview contract: the
// theme changes while browsing, and escape puts back what was on screen before.
func TestThemeSelectorEscapeDiscardsPreview(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	a := NewAppWithProvider(nil, "default", cfg, "local")
	a.applyTheme("gruvbox-dark")
	before := theme.Bg()

	a.showThemeSelector()
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("the theme selector should be a modal page, or the global escape handler will close it without cancelling")
	}

	// Browsing previews other themes.
	for i := 0; i < 3; i++ {
		pressModal(a, tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	}
	if theme.Bg() == before {
		t.Fatal("browsing should preview the highlighted theme")
	}

	pressModal(a, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if theme.Bg() != before {
		t.Fatalf("escape should discard the preview: bg=%v want %v", theme.Bg(), before)
	}
	if a.config.Theme != "gruvbox-dark" {
		t.Fatalf("escape should leave the remembered theme alone, got %q", a.config.Theme)
	}
	if a.app.Pages().CurrentIsModal() {
		t.Fatal("escape should close the selector")
	}
}

// TestColumnEditorEscapeDiscardsEdits is the same contract for column edits: they
// show up in the list straight away, and escape throws them away unsaved.
func TestColumnEditorEscapeDiscardsEdits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	a.app.Pages().Push(wl)

	wl.showColumnEditor()
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("the column editor should be a modal page")
	}

	pressModal(a, tcell.NewEventKey(tcell.KeyRune, '+', tcell.ModNone))
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if cfg.WorkflowColumns == nil {
		t.Fatal("edits should apply to the list behind the modal")
	}

	pressModal(a, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if cfg.WorkflowColumns != nil {
		t.Fatalf("escape should discard the edits, got %+v", cfg.WorkflowColumns)
	}
	if _, err := os.Stat(filepath.Join(dir, "tempo", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("escape must not write the config, stat err = %v", err)
	}
	if a.app.Pages().CurrentIsModal() {
		t.Fatal("escape should close the editor")
	}
}

// TestColumnEditorEnterSavesEdits is the other half: enter keeps them.
func TestColumnEditorEnterSavesEdits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	a.app.Pages().Push(wl)

	wl.showColumnEditor()
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)) // hide a column
	staged := len(cfg.WorkflowColumns)
	if staged == 0 {
		t.Fatal("hiding a column should stage a layout")
	}

	pressModal(a, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if len(cfg.WorkflowColumns) != staged {
		t.Fatalf("enter should keep the edit: %+v", cfg.WorkflowColumns)
	}
	if _, err := os.Stat(filepath.Join(dir, "tempo", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("enter should not write the config when autosave is off, stat err = %v", err)
	}
}

// The app's own key capture dismisses a modal on escape before the modal or its
// content sees the key, and that route runs OnDismiss rather than OnCancel.
// These cover it by dismissing the way that capture does.
func TestThemeSelectorDismissDiscardsPreview(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	a.applyTheme("gruvbox-dark")
	before := theme.Bg()

	a.showThemeSelector()
	behavior := a.app.Pages().CurrentModalBehavior()
	if behavior == nil || !behavior.DismissOnEsc {
		t.Fatal("escape is handled by the app's dismiss path, so that is the path the revert must cover")
	}
	for i := 0; i < 3; i++ {
		pressModal(a, tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	}
	if theme.Bg() == before {
		t.Fatal("browsing should preview the highlighted theme")
	}

	a.app.Pages().DismissModal()
	if theme.Bg() != before {
		t.Fatalf("dismissing should discard the preview: bg=%v want %v", theme.Bg(), before)
	}
	if a.config.Theme != "gruvbox-dark" {
		t.Fatalf("dismissing should leave the remembered theme alone, got %q", a.config.Theme)
	}
}

func TestThemeSelectorDismissKeepsChosenTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	a.applyTheme("gruvbox-dark")

	a.showThemeSelector()
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	pressModal(a, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	chosen := a.config.Theme
	if chosen == "gruvbox-dark" {
		t.Fatalf("enter should have chosen the highlighted theme, still %q", chosen)
	}
	kept := theme.Bg()
	// Choosing already closed the modal; a stray dismiss must not undo it.
	a.app.Pages().DismissModal()
	if theme.Bg() != kept || a.config.Theme != chosen {
		t.Fatalf("a chosen theme must survive: bg=%v theme=%q", theme.Bg(), a.config.Theme)
	}
}

func TestColumnEditorDismissDiscardsEdits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	a.app.Pages().Push(wl)

	wl.showColumnEditor()
	behavior := a.app.Pages().CurrentModalBehavior()
	if behavior == nil || !behavior.DismissOnEsc {
		t.Fatal("escape is handled by the app's dismiss path, so that is the path the revert must cover")
	}
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, '+', tcell.ModNone))
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if cfg.WorkflowColumns == nil {
		t.Fatal("edits should apply to the list behind the modal")
	}

	a.app.Pages().DismissModal()
	if cfg.WorkflowColumns != nil {
		t.Fatalf("dismissing should discard the edits, got %+v", cfg.WorkflowColumns)
	}
	if _, err := os.Stat(filepath.Join(dir, "tempo", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dismissing must not write the config, stat err = %v", err)
	}
}

func TestColumnEditorDismissKeepsSavedEdits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	a.app.Pages().Push(wl)

	wl.showColumnEditor()
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	pressModal(a, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	saved := append([]config.WorkflowColumnConfig(nil), cfg.WorkflowColumns...)
	if len(saved) == 0 {
		t.Fatal("enter should have saved a layout")
	}

	// A stray dismiss after saving must not roll the save back.
	a.app.Pages().DismissModal()
	if len(cfg.WorkflowColumns) != len(saved) {
		t.Fatalf("a saved layout must survive: %+v", cfg.WorkflowColumns)
	}
}

func TestActivityColumnEditorEscapeDiscardsEdits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	a.app.Pages().Push(wl)

	wl.showActivityColumnEditor()
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("the activity column editor should be a modal page")
	}

	pressModal(a, tcell.NewEventKey(tcell.KeyRune, '+', tcell.ModNone))
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if cfg.ActivityColumns == nil {
		t.Fatal("edits should apply to the table behind the modal")
	}

	pressModal(a, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if cfg.ActivityColumns != nil {
		t.Fatalf("escape should discard the edits, got %+v", cfg.ActivityColumns)
	}
}

func TestActivityColumnEditorOpensFromPreview(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	a.app.Pages().Push(wl)
	wl.previewKind = previewActivities

	if ev := wl.handlePreviewKeys(tcell.NewEventKey(tcell.KeyRune, '|', 0)); ev != nil {
		t.Fatal("pipe should open the activity column editor")
	}
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("the activity column editor should be a modal page")
	}
}
