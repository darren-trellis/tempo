package view

import (
	"testing"

	"github.com/atterpac/jig/layout"
)

func statusBarWithFixedSections() *layout.StatusBar {
	bar := layout.NewStatusBar()
	bar.AddSection(layout.StatusSection{Text: "profile"})
	bar.AddSection(layout.StatusSection{Text: "namespace"})
	bar.AddSection(layout.StatusSection{Text: "connected"})
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

func TestRenderLoadingReplacesConnected(t *testing.T) {
	a := &App{statusBar: statusBarWithFixedSections(), connected: true}

	a.renderLoading(loadingLabel(0))
	if a.statusBar.SectionCount() != 3 {
		t.Fatalf("spinner should reuse the connected slot, got %d sections", a.statusBar.SectionCount())
	}
	if a.statusBar.GetSection(connectionSectionIndex).Text != loadingLabel(0) {
		t.Fatalf("connected slot should show the spinner, got %q", a.statusBar.GetSection(connectionSectionIndex).Text)
	}

	a.renderLoading(loadingLabel(1))
	if a.statusBar.SectionCount() != 3 {
		t.Fatal("next frame should update the connected slot, not add a section")
	}
	if a.statusBar.GetSection(connectionSectionIndex).Text != loadingLabel(1) {
		t.Fatal("connected slot should hold the current frame")
	}

	a.renderLoading("")
	if a.statusBar.SectionCount() != 3 {
		t.Fatalf("restoring connected should keep three sections, got %d", a.statusBar.SectionCount())
	}
	if a.statusBar.GetSection(connectionSectionIndex).Text != "connected" {
		t.Fatalf("spinner should restore connected, got %q", a.statusBar.GetSection(connectionSectionIndex).Text)
	}
	if a.statusBar.GetSection(0).Text != "profile" {
		t.Fatal("spinner should leave the profile section alone")
	}
}

func TestRenderLoadingWaitsForFixedSections(t *testing.T) {
	a := &App{statusBar: layout.NewStatusBar()}
	a.renderLoading(loadingLabel(0))
	if a.statusBar.SectionCount() != 0 {
		t.Fatal("spinner should not claim a profile or namespace slot")
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

func TestSetConnectedKeepsSpinnerThenRestores(t *testing.T) {
	a := &App{statusBar: statusBarWithFixedSections(), connected: true}
	a.SetViewLoading("workflows", true)
	a.renderLoading(a.loadingText())
	if a.statusBar.GetSection(connectionSectionIndex).Text == "connected" {
		t.Fatal("loading should replace connected")
	}

	a.setConnected(true)
	if a.statusBar.GetSection(connectionSectionIndex).Text == "connected" {
		t.Fatal("a connection update should not hide the spinner")
	}

	a.SetViewLoading("workflows", false)
	a.renderLoading(a.loadingText())
	if a.statusBar.GetSection(connectionSectionIndex).Text != "connected" {
		t.Fatalf("finished load should restore connected, got %q", a.statusBar.GetSection(connectionSectionIndex).Text)
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
