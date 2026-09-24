package view

import (
	"strings"
	"testing"
)

func TestHighlightJSONLineNeutralizesKeyBrackets(t *testing.T) {
	got := highlightJSONLineWorkflow(`  "arr[0]": [1, 2]`)
	if strings.Contains(got, "[0]") {
		t.Fatalf("raw '[0]' left in %q", got)
	}
}
