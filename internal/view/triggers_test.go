package view

import (
	"strings"
	"testing"

	"github.com/galaxy-io/tempo/internal/command"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestMatchIOTriggerFirstMatchWins(t *testing.T) {
	triggers := []config.TriggerConfig{
		{Name: "other-event", On: "something_else", URL: "https://x"},
		{Name: "orders", Path: `orderId$`, Match: `^ord_(\w+)$`, Cmd: "open {1}"},
		{Name: "links", Match: `https?://\S+`, URL: "{match}"},
	}
	hit, ok, err := matchIOTrigger(triggers, "ord_42", "items.0.orderId")
	if err != nil || !ok || hit.trigger.Name != "orders" || hit.match != "ord_42" || len(hit.groups) != 1 || hit.groups[0] != "42" {
		t.Fatalf("hit=%+v ok=%v err=%v", hit, ok, err)
	}
	hit, ok, _ = matchIOTrigger(triggers, "see https://example.com/a now", "note")
	if !ok || hit.trigger.Name != "links" || hit.match != "https://example.com/a" {
		t.Fatalf("hit=%+v ok=%v", hit, ok)
	}
	if _, ok, _ := matchIOTrigger(triggers, "plain", "note"); ok {
		t.Fatal("nothing should match a plain value")
	}
}

func TestMatchIOTriggerWithoutPatternMatchesWholeValue(t *testing.T) {
	hit, ok, _ := matchIOTrigger([]config.TriggerConfig{{Path: `^url$`, URL: "{match}"}}, "https://x.io", "url")
	if !ok || hit.match != "https://x.io" {
		t.Fatalf("hit=%+v ok=%v", hit, ok)
	}
}

func TestMatchIOTriggerReportsBadPattern(t *testing.T) {
	_, _, err := matchIOTrigger([]config.TriggerConfig{{Name: "broken", Match: "("}}, "x", "")
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err=%v", err)
	}
}

func TestTriggerURLRefusesNonWebLinks(t *testing.T) {
	for _, value := range []string{"file:///etc/passwd", "/Applications/Calculator.app", "javascript:alert(1)", "https://"} {
		if _, err := triggerURL("{value}", command.Context{Value: value}); err == nil {
			t.Fatalf("%q should be refused", value)
		}
	}
	link, err := triggerURL("https://admin.example.com/orders/{1}", command.Context{Args: []string{"42"}})
	if err != nil || link != "https://admin.example.com/orders/42" {
		t.Fatalf("link=%q err=%v", link, err)
	}
}

func TestJSONTreeDottedPath(t *testing.T) {
	path, key := jsonTreeDottedPath(jsonTreePathKey([]string{"items", "0", "url"}))
	if path != "items.0.url" || key != "url" {
		t.Fatalf("path=%q key=%q", path, key)
	}
}

func TestRunIOTriggerOpensMatchedURL(t *testing.T) {
	var opened []string
	prev := openBrowser
	openBrowser = func(link string) error { opened = append(opened, link); return nil }
	t.Cleanup(func() { openBrowser = prev })

	a := &App{config: &config.Config{Triggers: []config.TriggerConfig{
		{Name: "links", Match: `https?://\S+`, URL: "{match}"},
	}}}
	view := tview.NewTextView().SetDynamicColors(true)
	selection := newJSONTreeSelection(view)
	selection.onEnter = a.runIOTrigger
	if !selection.setContent(`{"meta":{"a":1},"url":"go to https://example.com/x"}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	enter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)

	// The object row still folds: triggers only look at values.
	selection.handleKey(enter)
	if len(opened) != 0 || !selection.folded["meta"] {
		t.Fatalf("opened=%v folded=%v", opened, selection.folded)
	}

	selection.handleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	selection.handleKey(enter)
	if len(opened) != 1 || opened[0] != "https://example.com/x" {
		t.Fatalf("opened=%v", opened)
	}
}
