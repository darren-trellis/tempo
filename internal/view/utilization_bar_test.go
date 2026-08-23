package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/theme"
)

// barCells strips the tags, label and percentage, leaving the bar itself.
func barCells(bar string) string {
	var b strings.Builder
	for _, r := range bar {
		if r == '█' || r == '░' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestRenderUtilizationBar(t *testing.T) {
	// Width 19 leaves 10 cells once the label and percentage are reserved.
	if got := barCells(renderUtilizationBar("CPU", 0.5, true, 19)); got != "█████░░░░░" {
		t.Fatalf("half: %q", got)
	}
	if got := renderUtilizationBar("CPU", 0.5, true, 19); !strings.Contains(got, "CPU") || !strings.Contains(got, "50%") {
		t.Fatalf("half should keep its label and percentage: %q", got)
	}
	if got := barCells(renderUtilizationBar("MEM", 0, true, 19)); got != "░░░░░░░░░░" {
		t.Fatalf("idle: %q", got)
	}
	if got := barCells(renderUtilizationBar("MEM", 0.01, true, 19)); got != "█░░░░░░░░░" {
		t.Fatalf("a live worker should keep one filled cell: %q", got)
	}
	if got := barCells(renderUtilizationBar("CPU", 1.5, true, 19)); got != "██████████" {
		t.Fatalf("over-full should clamp: %q", got)
	}
	if got := renderUtilizationBar("CPU", 1.5, true, 19); !strings.Contains(got, "100%") {
		t.Fatalf("percentage should clamp too: %q", got)
	}

	// The bar widens with the pane.
	if got := barCells(renderUtilizationBar("CPU", 1, true, 49)); len(strings.Split(got, "")) != 40 {
		t.Fatalf("bar should fill the width, got %d cells", len(strings.Split(got, "")))
	}

	unknown := renderUtilizationBar("CPU", 0, false, 19)
	if barCells(unknown) != "" || !strings.Contains(unknown, "-") {
		t.Fatalf("a worker without host info should show a dash, not an empty bar: %q", unknown)
	}

	// Too narrow to draw: keep the numbers rather than a broken bar.
	if got := barCells(renderUtilizationBar("CPU", 0.5, true, 9)); got != "" {
		t.Fatalf("narrow: %q", got)
	}
}

func TestUtilizationTagThresholds(t *testing.T) {
	if utilizationTag(0.2) != theme.TagSuccess() {
		t.Fatal("low load should be green")
	}
	if utilizationTag(0.7) != theme.TagWarning() {
		t.Fatal("elevated load should be amber")
	}
	if utilizationTag(0.9) != theme.TagError() {
		t.Fatal("high load should be red")
	}
}
