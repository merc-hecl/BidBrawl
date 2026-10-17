package game

import (
	"strings"
	"testing"
)

func TestBuiltinCatalogHasDistinctBilingualIdentities(t *testing.T) {
	names, english, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, item := range Catalog(DefaultConfig()) {
		translated := strings.ToLower(strings.TrimSpace(item.Translations["en"]))
		if ids[item.ID] || names[item.Name] || translated == "" || english[translated] {
			t.Fatalf("duplicate or missing item identity: %s", item.ID)
		}
		ids[item.ID], names[item.Name], english[translated] = true, true, true
	}
	// The old directory multiplied every base item by five generic tier prefixes.
	// Distinct materials, models and provenance may still describe related items.
	families := map[string]int{}
	for name := range names {
		for _, prefix := range []string{"旧藏", "精制", "典藏", "大师", "传世"} {
			if strings.HasPrefix(name, prefix) {
				families[strings.TrimPrefix(name, prefix)]++
			}
		}
	}
	for base, count := range families {
		if count == 5 {
			t.Fatalf("five-tier prefix copies returned: %s", base)
		}
	}
}

func TestBuiltinCatalogRetainsUsefulUncertaintyAndClockCoverage(t *testing.T) {
	shapes := map[Dimensions]map[string]bool{}
	clockCategories, clockTiers := map[string]bool{}, map[string]bool{}
	for _, item := range Catalog(DefaultConfig()) {
		if item.Category == "easter_egg" {
			continue
		}
		shape := Dimensions{item.Width, item.Height}
		if shapes[shape] == nil {
			shapes[shape] = map[string]bool{}
		}
		shapes[shape][item.Quality] = true
		for _, tag := range item.Tags {
			if tag == "clock" {
				clockCategories[item.Category], clockTiers[item.Quality] = true, true
			}
		}
	}
	// Common silhouettes keep multiple tiers; category + size may legitimately
	// narrow further, and distinctive shapes need not be padded with fake copies.
	for _, shape := range []Dimensions{{1, 1}, {1, 2}, {2, 1}, {2, 2}} {
		if len(shapes[shape]) < 3 {
			t.Fatalf("common silhouette gives away quality: %v", shape)
		}
	}
	if len(clockCategories) < 3 || len(clockTiers) < 3 {
		t.Fatal("clock specialty lost its cross-category or cross-tier candidates")
	}
}
