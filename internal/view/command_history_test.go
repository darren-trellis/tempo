package view

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tempo-state-")
	if err != nil {
		os.Exit(1)
	}
	_ = os.Setenv("XDG_STATE_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestCommandHistoryPrevNextAndDedupe(t *testing.T) {
	h := newCommandHistory(nil)
	h.add("")
	h.add("  refresh  ")
	h.add("refresh")
	h.add("config get theme")
	if len(h.items) != 2 || h.items[0] != "refresh" || h.items[1] != "config get theme" {
		t.Fatalf("items=%v", h.items)
	}

	if got := h.prev(""); got != "config get theme" {
		t.Fatalf("first prev=%q", got)
	}
	if got := h.prev(""); got != "refresh" {
		t.Fatalf("second prev=%q", got)
	}
	if got := h.prev(""); got != "refresh" {
		t.Fatalf("oldest should stick, got %q", got)
	}
	if got := h.next(""); got != "config get theme" {
		t.Fatalf("next=%q", got)
	}
	if got := h.next("ignored"); got != "" {
		t.Fatalf("past newest should restore draft, got %q", got)
	}
	if h.browsing() {
		t.Fatal("draft should not count as browsing")
	}
}

func TestCommandHistoryPreservesDraft(t *testing.T) {
	h := newCommandHistory([]string{"refresh"})
	if got := h.prev("config set theme nord"); got != "refresh" {
		t.Fatalf("prev=%q", got)
	}
	if got := h.next(""); got != "config set theme nord" {
		t.Fatalf("draft=%q", got)
	}
}

func TestCommandHistoryCapsLength(t *testing.T) {
	h := newCommandHistory(nil)
	for i := 0; i < commandHistoryMax+5; i++ {
		h.add(strconv.Itoa(i))
	}
	if len(h.items) != commandHistoryMax {
		t.Fatalf("len=%d", len(h.items))
	}
	if h.items[0] != "5" || h.items[len(h.items)-1] != strconv.Itoa(commandHistoryMax+4) {
		t.Fatalf("window=%q..%q", h.items[0], h.items[len(h.items)-1])
	}
}

func TestCommandHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	h := newCommandHistory(nil)
	h.add("refresh")
	h.add("config save")
	if err := saveCommandHistory(path, h.items); err != nil {
		t.Fatal(err)
	}
	got := loadCommandHistory(path)
	if len(got.items) != 2 || got.items[0] != "refresh" || got.items[1] != "config save" {
		t.Fatalf("loaded=%v", got.items)
	}
	if got.browsing() {
		t.Fatal("loaded history should start at the draft")
	}
}

func TestCommandPromptUpWalksHistory(t *testing.T) {
	p := newHintPrompt()
	p.history = newCommandHistory([]string{"refresh", "config save"})
	p.suggestFn = func(string) []commandSuggestion {
		return []commandSuggestion{{Text: "profile", Label: "profile"}}
	}
	p.enter(": ", "command...")
	p.setInputText("hel")
	p.refreshSuggestions()
	p.capture(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if p.input.GetText() != "config save" {
		t.Fatalf("up should recall the last command, got %q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if p.input.GetText() != "refresh" {
		t.Fatalf("second up=%q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if p.input.GetText() != "config save" {
		t.Fatalf("down should move toward the draft, got %q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if p.input.GetText() != "hel" {
		t.Fatalf("down past newest should restore the draft, got %q", p.input.GetText())
	}
	if p.suggestions.browsed {
		t.Fatal("restoring the draft should not start suggestion browse")
	}
}

func TestCommandPromptDownStillBrowsesSuggestions(t *testing.T) {
	p := newHintPrompt()
	p.history = newCommandHistory([]string{"refresh"})
	p.suggestFn = func(string) []commandSuggestion {
		return []commandSuggestion{{Text: "profile", Label: "profile"}}
	}
	p.enter(": ", "command...")
	p.refreshSuggestions()
	p.capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if !p.suggestions.browsed || p.input.GetText() != "" {
		t.Fatal("down on a fresh prompt should browse suggestions, not history")
	}
}

func TestRecordCommandPersistsHistory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := &App{}
	a.hintPrompt = newHintPrompt()
	a.recordCommand("refresh")
	a.recordCommand("")
	a.recordCommand("refresh")
	got := loadCommandHistory(config.HistoryPath())
	if len(got.items) != 1 || got.items[0] != "refresh" {
		t.Fatalf("persisted=%v path=%s", got.items, config.HistoryPath())
	}
}

func TestFilterPromptUpWalksSearchHistory(t *testing.T) {
	a := &App{}
	p := a.prompt()
	p.searchHistory = newCommandHistory([]string{"orders", "payments"})
	a.ShowFilterMode("", FilterModeCallbacks{})
	p.capture(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if p.input.GetText() != "payments" {
		t.Fatalf("up should recall the last search, got %q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if p.input.GetText() != "orders" {
		t.Fatalf("second up=%q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if p.input.GetText() != "payments" {
		t.Fatalf("down should move toward the draft, got %q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if p.input.GetText() != "" {
		t.Fatalf("down past newest should restore the draft, got %q", p.input.GetText())
	}
}

func TestRecordSearchPersistsHistory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := &App{}
	a.hintPrompt = newHintPrompt()
	a.recordSearch("orders")
	a.recordSearch("")
	a.recordSearch("orders")
	got := loadCommandHistory(config.SearchHistoryPath())
	if len(got.items) != 1 || got.items[0] != "orders" {
		t.Fatalf("persisted=%v path=%s", got.items, config.SearchHistoryPath())
	}
}
