package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func filterNames(filters []SavedFilter) string {
	names := make([]string, len(filters))
	for i, f := range filters {
		names[i] = f.Profile + "/" + f.Name
	}
	return strings.Join(names, ",")
}

func scopedConfig() *Config {
	return &Config{
		SavedFilters: []SavedFilter{{Name: "shared"}, {Name: "shared2"}},
		ProfileFilters: map[string][]SavedFilter{
			"prod": {{Name: "p1"}, {Name: "p2"}},
			"beta": {{Name: "b1"}},
		},
	}
}

func TestSavedFiltersForListsProfileThenGlobal(t *testing.T) {
	cfg := scopedConfig()
	if got := filterNames(cfg.SavedFiltersFor("prod")); got != "prod/p1,prod/p2,/shared,/shared2" {
		t.Fatalf("prod sees %s", got)
	}
	if got := filterNames(cfg.SavedFiltersFor("beta")); got != "beta/b1,/shared,/shared2" {
		t.Fatalf("beta sees %s", got)
	}
	if got := filterNames(cfg.SavedFiltersFor("")); got != "/shared,/shared2" {
		t.Fatalf("no profile sees %s", got)
	}
	if _, ok := cfg.SavedFilterFor("beta", "p1"); ok {
		t.Fatal("beta should not see prod's filters")
	}
}

func TestSaveFilterForReplacesInPlaceOrAddsToProfile(t *testing.T) {
	cfg := scopedConfig()
	cfg.SaveFilterFor("prod", SavedFilter{Name: "shared", Query: "q", Profile: "prod"})
	if cfg.SavedFilters[0].Query != "q" || len(cfg.ProfileFilters["prod"]) != 2 {
		t.Fatalf("editing a global filter should stay global, got %+v / %+v", cfg.SavedFilters, cfg.ProfileFilters)
	}
	if cfg.SavedFilters[0].Profile != "" {
		t.Fatal("Profile is a listing detail and should not be stored")
	}
	cfg.SaveFilterFor("beta", SavedFilter{Name: "new"})
	if got := filterNames(cfg.SavedFiltersFor("beta")); got != "beta/b1,beta/new,/shared,/shared2" {
		t.Fatalf("new filters belong to the profile, got %s", got)
	}
	cfg.SaveFilterFor("fresh", SavedFilter{Name: "first"})
	if got := filterNames(cfg.SavedFiltersFor("fresh")); got != "fresh/first,/shared,/shared2" {
		t.Fatalf("a profile without filters should get its own list, got %s", got)
	}
}

func TestRenameAndDeleteFilterForStayInScope(t *testing.T) {
	cfg := scopedConfig()
	if err := cfg.RenameFilterFor("prod", "p2", "shared"); err == nil {
		t.Fatal("a profile filter cannot take a visible global filter's name")
	}
	if err := cfg.RenameFilterFor("beta", "b1", "p1"); err != nil {
		t.Fatalf("names only need to be unique among visible filters: %v", err)
	}
	if err := cfg.DeleteFilterFor("beta", "p1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.ProfileFilters["beta"]; ok {
		t.Fatal("an emptied profile list should be dropped")
	}
	if got := filterNames(cfg.SavedFiltersFor("prod")); got != "prod/p1,prod/p2,/shared,/shared2" {
		t.Fatalf("prod should be untouched, got %s", got)
	}
	if err := cfg.DeleteFilterFor("beta", "p2"); err == nil {
		t.Fatal("beta cannot delete a filter it does not see")
	}
}

func TestMoveSavedFilterForStaysWithinScope(t *testing.T) {
	cfg := scopedConfig()
	cfg.MoveSavedFilterFor("prod", 1, 2)
	if got := filterNames(cfg.SavedFiltersFor("prod")); got != "prod/p1,prod/p2,/shared,/shared2" {
		t.Fatalf("moving across scopes should do nothing, got %s", got)
	}
	cfg.MoveSavedFilterFor("prod", 3, 2)
	if got := filterNames(cfg.SavedFiltersFor("prod")); got != "prod/p1,prod/p2,/shared2,/shared" {
		t.Fatalf("global filters reorder by visible index, got %s", got)
	}
	cfg.MoveSavedFilterFor("prod", 0, 1)
	if got := filterNames(cfg.SavedFiltersFor("prod")); got != "prod/p2,prod/p1,/shared2,/shared" {
		t.Fatalf("got %s", got)
	}
}

func TestSetFilterScopeMovesBetweenLists(t *testing.T) {
	cfg := scopedConfig()
	if err := cfg.SetFilterScope("prod", "p1", true); err != nil {
		t.Fatal(err)
	}
	if got := filterNames(cfg.SavedFiltersFor("beta")); got != "beta/b1,/shared,/shared2,/p1" {
		t.Fatalf("a global filter should reach every profile, got %s", got)
	}
	if err := cfg.SetFilterScope("beta", "shared", false); err != nil {
		t.Fatal(err)
	}
	if got := filterNames(cfg.SavedFiltersFor("prod")); got != "prod/p2,/shared2,/p1" {
		t.Fatalf("a filter taken by beta should leave prod, got %s", got)
	}
	if got := filterNames(cfg.SavedFiltersFor("beta")); got != "beta/b1,beta/shared,/shared2,/p1" {
		t.Fatalf("got %s", got)
	}
}

func TestInsertFilterPlacesWithinScope(t *testing.T) {
	cfg := scopedConfig()
	cfg.InsertFilter("prod", 1, SavedFilter{Name: "mid"})
	cfg.InsertFilter("", 99, SavedFilter{Name: "end"})
	if got := filterNames(cfg.SavedFiltersFor("prod")); got != "prod/p1,prod/mid,prod/p2,/shared,/shared2,/end" {
		t.Fatalf("got %s", got)
	}
}

func TestEnsureSavedFiltersSkipsConfigsWithProfileFilters(t *testing.T) {
	cfg := &Config{ProfileFilters: map[string][]SavedFilter{"prod": {{Name: "p"}}}}
	cfg.EnsureSavedFilters()
	if len(cfg.SavedFilters) != 0 {
		t.Fatalf("profile filters count as filters, so nothing should be seeded, got %+v", cfg.SavedFilters)
	}
}

func TestEmptiedGlobalFiltersSurviveARoundTrip(t *testing.T) {
	cfg := &Config{SavedFilters: []SavedFilter{{Name: "only"}}}
	if err := cfg.DeleteFilterFor("", "only"); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := ParseConfigFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SavedFilters) != 0 {
		t.Fatalf("deleting every filter should not bring the defaults back, got %d", len(loaded.SavedFilters))
	}
}

func TestDeleteProfileDropsItsFilters(t *testing.T) {
	cfg := scopedConfig()
	cfg.Profiles = map[string]ConnectionConfig{"prod": {}, "beta": {}}
	cfg.ActiveProfile = "beta"
	if err := cfg.DeleteProfile("prod"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.ProfileFilters["prod"]; ok {
		t.Fatal("a deleted profile's filters should go with it")
	}
}
