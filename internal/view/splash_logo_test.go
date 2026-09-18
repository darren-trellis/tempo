package view

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestSplashLogosAreOrderedNarrowestFirst(t *testing.T) {
	for i := 1; i < len(SplashLogos); i++ {
		if SplashLogos[i-1].Width() > SplashLogos[i].Width() {
			t.Fatalf("%s (%d cols) should not come before %s (%d cols)",
				SplashLogos[i-1].Name, SplashLogos[i-1].Width(),
				SplashLogos[i].Name, SplashLogos[i].Width())
		}
	}
}

func TestSplashLogoArtHasNoStrayPadding(t *testing.T) {
	for _, logo := range SplashLogos {
		lines := logo.Lines()
		if len(lines) < 5 {
			t.Fatalf("%s is only %d rows tall", logo.Name, len(lines))
		}
		if strings.TrimSpace(lines[0]) == "" {
			t.Fatalf("%s starts with a blank row, so the art carries stray padding", logo.Name)
		}
		if strings.TrimSpace(lines[len(lines)-1]) == "" {
			t.Fatalf("%s ends with a blank row, so the art carries stray padding", logo.Name)
		}
		for i, line := range lines {
			if got := len([]rune(line)); got > logo.Width() {
				t.Fatalf("%s row %d is %d cols, wider than the reported %d", logo.Name, i, got, logo.Width())
			}
		}
	}
}

func TestSplashLogosFittingRespectsTerminalSize(t *testing.T) {
	widest := SplashLogos[len(SplashLogos)-1]
	narrowest := SplashLogos[0]

	if got := splashLogosFitting(0, 0); len(got) != len(SplashLogos) {
		t.Fatalf("an unknown terminal size should not rule anything out, got %d", len(got))
	}

	tooNarrow := splashLogosFitting(widest.Width()-1, 0)
	for _, logo := range tooNarrow {
		if logo.Name == widest.Name {
			t.Fatalf("%s needs %d cols and should not fit in %d", widest.Name, widest.Width(), widest.Width()-1)
		}
	}

	tooShort := splashLogosFitting(0, widest.Height()+splashLogoChrome-1)
	for _, logo := range tooShort {
		if logo.Name == widest.Name {
			t.Fatalf("%s should not fit a terminal one row too short", widest.Name)
		}
	}

	if got := splashLogosFitting(narrowest.Width(), narrowest.Height()+splashLogoChrome); len(got) != 1 || got[0].Name != narrowest.Name {
		t.Fatalf("an exactly-sized terminal should fit only %s, got %v", narrowest.Name, got)
	}
}

func TestRandomSplashLogoStaysWithinTheTerminal(t *testing.T) {
	cols, rows := 100, 30
	for i := 0; i < 200; i++ {
		logo := RandomSplashLogo(cols, rows)
		if logo.Width() > cols || logo.Height()+splashLogoChrome > rows {
			t.Fatalf("%s (%dx%d) does not fit %dx%d", logo.Name, logo.Width(), logo.Height(), cols, rows)
		}
	}
}

func TestRandomSplashLogoEventuallyPicksEachFittingStyle(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		seen[RandomSplashLogo(0, 0).Name] = true
	}
	for _, logo := range SplashLogos {
		if !seen[logo.Name] {
			t.Fatalf("%s never came up in 500 picks", logo.Name)
		}
	}
}

func TestRandomSplashLogoFallsBackWhenNothingFits(t *testing.T) {
	logo := RandomSplashLogo(10, 5)
	if logo.Name != SplashLogos[0].Name {
		t.Fatalf("a tiny terminal should fall back to the narrowest logo, got %s", logo.Name)
	}
}

// The dev splash preview resizes its containers as it cycles, so each style
// has to come out whole rather than clipped to the one before it.
func TestSplashPreviewDrawsEveryLogoWhole(t *testing.T) {
	const cols, rows = 130, 34
	v := NewSplashTestView("tokyonight-night")
	v.SetRect(0, 0, cols, rows)

	for range SplashLogos {
		logo := SplashLogos[v.logoIndex]
		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatal(err)
		}
		screen.SetSize(cols, rows)
		v.Draw(screen)

		want := strings.TrimRight(logo.Lines()[0], " ")
		found := false
		for y := 0; y < rows; y++ {
			if strings.Contains(rowText(screen, y, cols), want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s logo did not render whole", logo.Name)
		}

		v.logoIndex = (v.logoIndex + 1) % len(SplashLogos)
		v.updateDisplay()
	}
}
