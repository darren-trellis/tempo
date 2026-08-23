package view

import (
	"os"
	"strings"
	"testing"
)

func TestEditorFileExt(t *testing.T) {
	if got := editorFileExt(`{"a":1}`); got != ".json" {
		t.Fatalf("json object: %q", got)
	}
	if got := editorFileExt("  [1, 2]"); got != ".json" {
		t.Fatalf("json array: %q", got)
	}
	if got := editorFileExt("plain text"); got != ".txt" {
		t.Fatalf("text: %q", got)
	}
}

func TestWriteEditorFilePrettyJSON(t *testing.T) {
	path, err := writeEditorFile("input", `{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if !strings.HasSuffix(path, ".json") {
		t.Fatalf("expected .json path, got %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\n") {
		t.Fatalf("expected pretty JSON, got %q", data)
	}
}

func TestResolveEditorUsesVISUAL(t *testing.T) {
	t.Setenv("VISUAL", "true")
	t.Setenv("EDITOR", "false")
	bin, args, err := resolveEditor()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(bin, "true") {
		t.Fatalf("expected true from VISUAL, got %q", bin)
	}
	if len(args) != 0 {
		t.Fatalf("unexpected args %v", args)
	}
}
