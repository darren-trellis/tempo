package view

import (
	"testing"

	"github.com/atterpac/jig/layout"
)

func statusBarWithFixedSections() *layout.StatusBar {
	bar := layout.NewStatusBar()
	for i := 0; i < loadingSectionIndex; i++ {
		bar.AddSection(layout.StatusSection{Text: "fixed"})
	}
	return bar
}

func TestLoadingLabelCyclesFrames(t *testing.T) {
	first := loadingLabel(0)
	if first == loadingLabel(1) {
		t.Fatal("consecutive frames should differ")
	}
	if loadingLabel(len(loadingFrames)) != first {
		t.Fatal("frames should wrap around")
	}
}

func TestRenderLoadingAddsAndRemovesSection(t *testing.T) {
	a := &App{statusBar: statusBarWithFixedSections()}

	a.renderLoading(loadingLabel(0))
	if a.statusBar.SectionCount() != loadingSectionIndex+1 {
		t.Fatalf("spinner should append one section, got %d", a.statusBar.SectionCount())
	}

	a.renderLoading(loadingLabel(1))
	if a.statusBar.SectionCount() != loadingSectionIndex+1 {
		t.Fatal("next frame should update the section, not add another")
	}
	if a.statusBar.GetSection(loadingSectionIndex).Text != loadingLabel(1) {
		t.Fatal("spinner section should hold the current frame")
	}

	a.renderLoading("")
	if a.statusBar.SectionCount() != loadingSectionIndex {
		t.Fatalf("spinner should be removed, got %d sections", a.statusBar.SectionCount())
	}
	if a.statusBar.GetSection(0).Text != "fixed" {
		t.Fatal("removing the spinner should leave the fixed sections alone")
	}
}

func TestRenderLoadingWaitsForFixedSections(t *testing.T) {
	a := &App{statusBar: layout.NewStatusBar()}
	a.renderLoading(loadingLabel(0))
	if a.statusBar.SectionCount() != 0 {
		t.Fatal("spinner should not claim a fixed section's slot")
	}
}

func TestSetViewLoadingTracksViewsIndependently(t *testing.T) {
	a := &App{}

	a.SetViewLoading("workflows", true)
	a.SetViewLoading("workers", true)
	if a.loadingText() == "" {
		t.Fatal("spinner should run while a view is loading")
	}

	a.SetViewLoading("workflows", false)
	if a.loadingText() == "" {
		t.Fatal("spinner should keep running while another view is loading")
	}

	a.SetViewLoading("workers", false)
	if a.loadingText() != "" {
		t.Fatal("spinner should stop once every view is done")
	}
}

func TestSetViewLoadingIsIdempotent(t *testing.T) {
	a := &App{}
	a.SetViewLoading("workflows", true)
	a.SetViewLoading("workflows", true)
	a.SetViewLoading("workflows", false)
	if a.loadingText() != "" {
		t.Fatal("repeated starts should not need repeated stops")
	}
}
