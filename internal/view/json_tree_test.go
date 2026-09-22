package view

import (
	"strings"
	"testing"
)

func stripTreeTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '[':
			inTag = true
		case r == ']' && inTag:
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestFormatJSONTreeExpandsNestedObject(t *testing.T) {
	got := stripTreeTags(formatJSONTree(`{"annotations":{"url":"http://x","tags":["a","b"]}}`))
	if !strings.Contains(got, "▾ annotations") {
		t.Fatalf("root object key should fold, got %q", got)
	}
	if !strings.Contains(got, "url: \"http://x\"") {
		t.Fatalf("nested string should render, got %q", got)
	}
	if !strings.Contains(got, "▾ tags") {
		t.Fatalf("arrays should show a count, got %q", got)
	}
	if !strings.Contains(got, "0: \"a\"") || !strings.Contains(got, "1: \"b\"") {
		t.Fatalf("array items should be indexed, got %q", got)
	}
	if !strings.Contains(got, "├── ") || !strings.Contains(got, "└── ") {
		t.Fatalf("tree should use box-drawing branches, got %q", got)
	}
}

func TestFormatJSONTreeEmptyContainers(t *testing.T) {
	got := stripTreeTags(formatJSONTree(`{"empty_obj":{},"empty_arr":[],"nested":{"a":{},"b":[]}}`))
	if !strings.Contains(got, "empty_obj: {}") {
		t.Fatalf("empty object, got %q", got)
	}
	if !strings.Contains(got, "empty_arr: ［]") {
		t.Fatalf("empty array, got %q", got)
	}
	if !strings.Contains(got, "a: {}") || !strings.Contains(got, "b: ［]") {
		t.Fatalf("nested empty containers, got %q", got)
	}
}

func TestFormatJSONTreeCommaSeparatedPayloads(t *testing.T) {
	got := stripTreeTags(formatJSONTree(`{"order":1}, false`))
	if !strings.Contains(got, "▾") || !strings.Contains(got, "2") {
		t.Fatalf("multi-arg payloads should be a root array, got %q", got)
	}
	if !strings.Contains(got, "order: 1") {
		t.Fatalf("object payload should expand, got %q", got)
	}
	if !strings.Contains(got, "1: false") {
		t.Fatalf("boolean payload should stay, got %q", got)
	}
}

func TestFormatJSONTreeLeavesPlainText(t *testing.T) {
	raw := "not json at all"
	if got := formatJSONTree(raw); !strings.Contains(stripTreeTags(got), raw) {
		t.Fatalf("plain text should survive, got %q", got)
	}
}

func TestFormatIOContentUsesTreeWhenAsked(t *testing.T) {
	pretty := formatIOContent("Input", `{"a":1}`, false)
	if strings.Contains(stripTreeTags(pretty), "▾") {
		t.Fatalf("pretty mode should not draw a tree, got %q", pretty)
	}
	tree := formatIOContent("Input", `{"a":1}`, true)
	if !strings.Contains(stripTreeTags(tree), "a: 1") {
		t.Fatalf("tree mode should flatten the object, got %q", tree)
	}
}
