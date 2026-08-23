package update

import "testing"

func TestCanCompareVersion(t *testing.T) {
	cases := map[string]bool{
		"0.1.14":        true,
		"v0.1.14":       true,
		"6204fca-dirty": false,
		"6204fca":       false,
		"dev":           false,
		"":              false,
	}
	for version, want := range cases {
		if got := canCompareVersion(version); got != want {
			t.Fatalf("canCompareVersion(%q)=%v, want %v", version, got, want)
		}
	}
}
