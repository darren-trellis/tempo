package view

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/rivo/tview"
)

func augmentApp(cmd string) *App {
	return &App{config: &config.Config{Triggers: []config.TriggerConfig{{
		Name: "s3-output",
		On:   config.TriggerIOShow,
		Path: `(^|\.)output_s3_key$`,
		Cmd:  cmd,
	}}}}
}

func decodeObject(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not one object: %v\n%s", err, s)
	}
	return m
}

func TestAugmentAddsCommandJSONUnderTheTriggerName(t *testing.T) {
	a := augmentApp(`printf '{"key":"%s","rows":3}' {value}`)
	a.augments.spawn = func(f func()) { f() }
	got := a.augmentIO(`{"output_s3_key":"executions/01a1/output.json","status":"ok"}`, nil)

	m := decodeObject(t, got)
	if m["status"] != "ok" || m["output_s3_key"] != "executions/01a1/output.json" {
		t.Fatalf("the payload should keep its own keys, got %v", m)
	}
	fetched, ok := m["s3-output"].(map[string]any)
	if !ok || fetched["key"] != "executions/01a1/output.json" || fetched["rows"] != float64(3) {
		t.Fatalf("the command's JSON should be added as JSON, got %#v", m["s3-output"])
	}
}

func TestAugmentNestedMatchesAndNonObjectPayloads(t *testing.T) {
	a := augmentApp(`printf '%s' {value}`)
	a.augments.spawn = func(f func()) { f() }
	got := a.augmentIO(`{"result":{"output_s3_key":"a"}}, "second"`, nil)
	if !strings.HasPrefix(got, `{"result":{"output_s3_key":"a"}}, "second"`) || !strings.HasSuffix(got, `{"s3-output":"a"}`) {
		t.Fatalf("a payload of several values should get one more, got %s", got)
	}

	got = a.augmentIO(`{"one":{"output_s3_key":"a"},"two":{"output_s3_key":"b"}}`, nil)
	m := decodeObject(t, got)
	if m["s3-output 1"] != "a" || m["s3-output 2"] != "b" {
		t.Fatalf("two matches should be numbered, got %v", m)
	}
}

func TestAugmentShowsTheCommandsError(t *testing.T) {
	a := augmentApp(`echo "AccessDenied: no s3:GetObject" >&2; exit 1`)
	a.augments.spawn = func(f func()) { f() }
	m := decodeObject(t, a.augmentIO(`{"output_s3_key":"k"}`, nil))
	if m["s3-output"] != "error: AccessDenied: no s3:GetObject" {
		t.Fatalf("got %v", m["s3-output"])
	}
}

func TestAugmentLeavesPayloadsWithoutMatchesAlone(t *testing.T) {
	a := augmentApp(`exit 1`)
	payload := `{"other": 1}`
	if got := a.augmentIO(payload, nil); got != payload {
		t.Fatalf("got %s", got)
	}
	enterOnly := &App{config: &config.Config{Triggers: []config.TriggerConfig{{Name: "x", Path: "output_s3_key", Cmd: "exit 1"}}}}
	if got := enterOnly.augmentIO(`{"output_s3_key":"k"}`, nil); got != `{"output_s3_key":"k"}` {
		t.Fatalf("io_enter triggers should not run on their own, got %s", got)
	}
}

// The tree shows "fetching…" until the command is done, then redraws with
// its result in place and the highlight still on the same row.
func TestTreeRedrawsWhenAugmentArrives(t *testing.T) {
	a := augmentApp(`printf '{"rows":3}'`)
	var work func()
	a.augments.spawn = func(f func()) { work = f }

	view := tview.NewTextView().SetDynamicColors(true)
	view.SetRect(0, 0, 80, 20)
	tree := newJSONTreeSelection(view)
	tree.augment = a.augmentIO
	if !tree.setContent(`{"output_s3_key":"k","status":"ok"}`, true) {
		t.Fatal("expected a tree")
	}
	if !strings.Contains(stripTreeTags(view.GetText(false)), "s3-output: fetching…") {
		t.Fatalf("a running command should read as fetching, got\n%s", stripTreeTags(view.GetText(false)))
	}
	statusRow, _ := tree.indexOfPath("status")
	tree.selectRow(statusRow)

	work()
	plain := stripTreeTags(view.GetText(false))
	if !strings.Contains(plain, "rows: 3") || strings.Contains(plain, "fetching") {
		t.Fatalf("the tree should redraw with the result, got\n%s", plain)
	}
	if row, _ := tree.selectedRow(); row.path != "status" {
		t.Fatalf("the highlight should stay on its row, got %q", row.path)
	}
	// A later render of the same payload reuses the result.
	work = nil
	tree.setContent(`{"output_s3_key":"k","status":"ok"}`, true)
	if work != nil {
		t.Fatal("a fetched result should not be fetched again")
	}
}
