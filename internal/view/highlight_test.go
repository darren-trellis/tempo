package view

import (
	"strings"
	"testing"
)

func TestFormatEventDetailsNeutralizesPayloadBrackets(t *testing.T) {
	got := formatEventDetails("Input: [encrypted:payload")
	if strings.Contains(got, "[encrypted") {
		t.Fatalf("raw '[' left in payload text: %q", got)
	}
	if !strings.Contains(got, "［encrypted") {
		t.Fatalf("expected neutralized bracket in %q", got)
	}
}

func TestHighlightJSONLineNeutralizesKeyBrackets(t *testing.T) {
	got := highlightJSONLineWorkflow(`  "arr[0]": [1, 2]`)
	if strings.Contains(got, "[0]") {
		t.Fatalf("raw '[0]' left in %q", got)
	}
}
