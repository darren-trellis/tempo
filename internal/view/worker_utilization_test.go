package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// drawWorkerView renders the workers pane onto a simulation screen.
func drawWorkerView(t *testing.T, width, height int) (tcell.SimulationScreen, *WorkerView) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(width, height)

	wv := NewWorkerView(&App{})
	wv.loadMockData()
	wv.table.SelectRow(1) // first instance under host-001
	wv.updatePreview()
	wv.detailFlex.SetRect(0, 0, width, height)
	wv.detailFlex.Draw(screen)
	return screen, wv
}

// barRunLength counts the meter cells on a row.
func barRunLength(screen tcell.SimulationScreen, row, width int) int {
	count := 0
	for col := 0; col < width; col++ {
		main, _, _, _ := screen.GetContent(col, row)
		if main == '█' || main == '░' {
			count++
		}
	}
	return count
}

func TestWorkerUtilizationPanelFillsWidth(t *testing.T) {
	const width, height = 60, 24
	screen, wv := drawWorkerView(t, width, height)

	if wv.detailFlex.GetItemCount() != 2 {
		t.Fatalf("workers detail should stack the detail table and utilization panel, got %d", wv.detailFlex.GetItemCount())
	}

	// The utilization panel sits at the bottom, inside its own border.
	found := 0
	for row := height - workerUtilizationHeight; row < height; row++ {
		if cells := barRunLength(screen, row, width); cells > 0 {
			found++
			// Full width less the panel border, label and percentage.
			if want := width - 2 - utilizationLabelWidth - utilizationPercentWidth; cells != want {
				t.Fatalf("row %d drew %d bar cells, want %d", row, cells, want)
			}
		}
	}
	if found != 2 {
		t.Fatalf("expected a CPU and a memory bar, found %d", found)
	}
}

func TestWorkerUtilizationTracksSelection(t *testing.T) {
	_, wv := drawWorkerView(t, 60, 24)
	if wv.cpuBar.value != 0.18 || !wv.cpuBar.known {
		t.Fatalf("cpu bar should follow the selected instance, got %v known=%v", wv.cpuBar.value, wv.cpuBar.known)
	}
	if wv.memBar.value != 0.41 {
		t.Fatalf("memory bar should follow the selected instance, got %v", wv.memBar.value)
	}

	// A host row shows the host aggregate.
	wv.table.SelectRow(0)
	wv.updatePreview()
	if !wv.cpuBar.known || wv.cpuBar.value != wv.groups[0].CPU {
		t.Fatalf("host row should show the host aggregate, got %v", wv.cpuBar.value)
	}
	if infoRowValue(wv.detailRows, "host") != "host-001" {
		t.Fatalf("host row should load host detail rows: %+v", wv.detailRows)
	}
}
