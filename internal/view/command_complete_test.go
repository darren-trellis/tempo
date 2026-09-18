package view

import (
	"testing"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
)

func testCommandCatalog() []commandInfo {
	return append(builtinCommandCatalog(), commandInfo{name: "logs", help: "Run logs"}, commandInfo{name: "reorder", help: "Run reorder"})
}

func suggestionLabels(items []commandSuggestion) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Label
	}
	return out
}

func hasLabel(items []commandSuggestion, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func TestCommandSuggestionsEmptyListsRoots(t *testing.T) {
	items := suggestionsFor("", testCommandCatalog(), commandExtras{})
	if !hasLabel(items, "profile") || !hasLabel(items, "logs") {
		t.Fatalf("roots=%v", suggestionLabels(items))
	}
	if hasLabel(items, "new") {
		t.Fatalf("empty input should not list nested tokens, got %v", suggestionLabels(items))
	}
}

func TestCommandSuggestionsPrefixThenContains(t *testing.T) {
	items := suggestionsFor("ord", testCommandCatalog(), commandExtras{})
	if len(items) == 0 || items[0].Label != "reorder" {
		t.Fatalf("prefix/contains should rank reorder first, got %v", suggestionLabels(items))
	}
}

func TestCommandSuggestionsProfileNested(t *testing.T) {
	catalog := testCommandCatalog()
	items := suggestionsFor("profile ", catalog, commandExtras{})
	if !hasLabel(items, "new") || !hasLabel(items, "edit") || !hasLabel(items, "delete") {
		t.Fatalf("profile subcommands=%v", suggestionLabels(items))
	}

	items = suggestionsFor("profile e", catalog, commandExtras{})
	if len(items) != 1 || items[0].Label != "edit" {
		t.Fatalf("profile e=%v", suggestionLabels(items))
	}

	items = suggestionsFor("profile", catalog, commandExtras{})
	foundLead := false
	for _, item := range items {
		if item.Label == "new" && item.Text == " new" {
			foundLead = true
		}
	}
	if !foundLead {
		t.Fatalf("profile without space should insert a leading space, got %+v", items)
	}
}

func TestCommandSuggestionsProfileNames(t *testing.T) {
	catalog := testCommandCatalog()
	profiles := []string{"staging", "prod"}
	items := suggestionsFor("profile ", catalog, commandExtras{profiles: profiles})
	if !hasLabel(items, "staging") || !hasLabel(items, "prod") {
		t.Fatalf("profile names=%v", suggestionLabels(items))
	}

	items = suggestionsFor("profile edit s", catalog, commandExtras{profiles: profiles})
	if len(items) != 1 || items[0].Label != "staging" {
		t.Fatalf("profile edit s=%v", suggestionLabels(items))
	}
	if items[0].Help != "Edit this profile" {
		t.Fatalf("help=%q", items[0].Help)
	}

	items = suggestionsFor("profile delete p", catalog, commandExtras{profiles: profiles})
	if len(items) != 1 || items[0].Label != "prod" {
		t.Fatalf("profile delete p=%v", suggestionLabels(items))
	}
}

func TestCommandCatalogIncludesUserCommands(t *testing.T) {
	a := &App{
		activeProfile: "default",
		config: &config.Config{
			Profiles: map[string]config.ConnectionConfig{
				"default": {
					Commands: map[string]config.CommandConfig{
						"logs": {Description: "Tail worker logs", Cmd: "echo hi"},
					},
				},
			},
		},
	}
	items := suggestionsFor("", a.commandCatalog(), a.commandExtras())
	if !hasLabel(items, "logs") {
		t.Fatalf("user command missing: %v", suggestionLabels(items))
	}
	for _, item := range items {
		if item.Label == "logs" && item.Help != "Tail worker logs" {
			t.Fatalf("logs help=%q", item.Help)
		}
	}
}

func TestCommandPromptEscEmptyExitsImmediately(t *testing.T) {
	p := newHintPrompt()
	canceled := false
	p.onCancel = func() { canceled = true }
	p.suggestFn = func(string) []commandSuggestion {
		return []commandSuggestion{{Text: "profile", Label: "profile", Help: "Switch or manage profiles"}}
	}
	p.enter(": ", "command...")
	p.refreshSuggestions()
	if len(p.suggestions.items) == 0 {
		t.Fatal("opening : should list suggestions")
	}
	if ev := p.capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Fatal("esc should leave command mode when the buffer is empty")
	}
	if !canceled {
		t.Fatal("empty command bar should exit on the first esc")
	}
}

func TestCommandPromptEscClosesSuggestionsFirst(t *testing.T) {
	p := newHintPrompt()
	p.suggestFn = func(string) []commandSuggestion {
		return []commandSuggestion{{Text: "profile", Label: "profile", Help: "Switch or manage profiles"}}
	}
	p.enter(": ", "command...")
	p.setInputText("pro")
	p.refreshSuggestions()
	if len(p.suggestions.items) == 0 {
		t.Fatal("typed text should keep suggestions open")
	}
	if ev := p.capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Fatal("esc should close the box")
	}
	if len(p.suggestions.items) != 0 {
		t.Fatal("suggestions should be cleared")
	}
	canceled := false
	p.onCancel = func() { canceled = true }
	p.capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !canceled {
		t.Fatal("a second esc should leave command mode")
	}
}

func TestHintPromptApplyThemeUpdatesFieldBackground(t *testing.T) {
	p := newHintPrompt()
	old := tcell.ColorRed
	p.input.SetPlaceholderStyle(tcell.StyleDefault.Background(old).Foreground(old))
	p.input.SetFieldStyle(tcell.StyleDefault.Background(old).Foreground(old))
	p.applyTheme()
	_, placeholderBg, _ := p.input.GetPlaceholderStyle().Decompose()
	if placeholderBg != theme.Bg() {
		t.Fatalf("placeholder bg=%v want %v", placeholderBg, theme.Bg())
	}
	_, fieldBg, _ := p.input.GetFieldStyle().Decompose()
	if fieldBg != theme.Bg() {
		t.Fatalf("field bg=%v want %v", fieldBg, theme.Bg())
	}
}

func TestCommandPromptTabAppliesAndEnterRuns(t *testing.T) {
	p := newHintPrompt()
	submitted := ""
	p.onSubmit = func(text string) { submitted = text }
	p.suggestFn = func(input string) []commandSuggestion {
		return suggestionsFor(input, builtinCommandCatalog(), commandExtras{})
	}
	p.enter(": ", "command...")
	p.refreshSuggestions()
	p.capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if p.input.GetText() != "quit" {
		t.Fatalf("tab should apply the first command, got %q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if submitted != "quit" {
		t.Fatalf("enter after apply should run the command, got %q", submitted)
	}
}

func TestCommandPromptDownBrowsesThenEnterApplies(t *testing.T) {
	p := newHintPrompt()
	submitted := false
	p.onSubmit = func(string) { submitted = true }
	p.suggestFn = func(string) []commandSuggestion {
		return []commandSuggestion{
			{Text: "profile", Label: "profile", Help: "Switch or manage profiles"},
			{Text: "logs", Label: "logs", Help: "Run logs"},
		}
	}
	p.enter(": ", "command...")
	p.refreshSuggestions()
	p.capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if !p.suggestions.browsed || p.suggestions.selectedItem() == nil || p.suggestions.selectedItem().Label != "profile" {
		t.Fatal("down should focus the first suggestion without inserting it")
	}
	if p.input.GetText() != "" {
		t.Fatalf("down should not replace the input, got %q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if submitted {
		t.Fatal("enter while browsing should apply, not run")
	}
	if p.input.GetText() != "profile" {
		t.Fatalf("applied %q", p.input.GetText())
	}
	p.capture(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
}

func TestCommandPromptUpFromFirstReturnsToInput(t *testing.T) {
	p := newHintPrompt()
	p.suggestFn = func(string) []commandSuggestion {
		return []commandSuggestion{{Text: "profile", Label: "profile"}}
	}
	p.enter(": ", "command...")
	p.refreshSuggestions()
	p.capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	p.capture(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if p.suggestions.browsed || p.suggestions.selected != nil {
		t.Fatal("up from the first item should return to the input")
	}
}

func TestCommandSuggestionsConfigAndFilters(t *testing.T) {
	catalog := builtinCommandCatalog()
	items := suggestionsFor("", catalog, commandExtras{})
	if !hasLabel(items, "config") || hasLabel(items, "get") || hasLabel(items, "theme") {
		t.Fatalf("empty should list config root only, got %v", suggestionLabels(items))
	}

	items = suggestionsFor("config ", catalog, commandExtras{})
	if !hasLabel(items, "get") || !hasLabel(items, "set") || !hasLabel(items, "save") {
		t.Fatalf("config subcommands=%v", suggestionLabels(items))
	}

	items = suggestionsFor("config set ", catalog, commandExtras{})
	if !hasLabel(items, "theme") || !hasLabel(items, "autosave") || !hasLabel(items, "filter_wrap") || !hasLabel(items, "saved_filters_position") {
		t.Fatalf("config set names=%v", suggestionLabels(items))
	}

	items = suggestionsFor("config set saved_filters_position ", catalog, commandExtras{})
	if !hasLabel(items, "top") || !hasLabel(items, "side") {
		t.Fatalf("saved_filters_position values=%v", suggestionLabels(items))
	}

	items = suggestionsFor("config set theme ", catalog, commandExtras{})
	if !hasLabel(items, config.DefaultTheme) {
		t.Fatalf("theme values=%v", suggestionLabels(items))
	}

	items = suggestionsFor("filter ", catalog, commandExtras{})
	if !hasLabel(items, "manage") || !hasLabel(items, "load") {
		t.Fatalf("filter subcommands=%v", suggestionLabels(items))
	}
	if hasLabel(items, "save") {
		t.Fatalf("filter save should be gone, got %v", suggestionLabels(items))
	}

	items = suggestionsFor("filter load ", catalog, commandExtras{filters: []string{"failed-today"}})
	if !hasLabel(items, "failed-today") {
		t.Fatalf("filter load names=%v", suggestionLabels(items))
	}
}
