package game

import (
	"math"
	"strings"
	"testing"

	"bidbrawl/internal/cards"
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

func TestCatalogAppearanceProbabilitiesUseIndependentEggTierAndWarehouse(t *testing.T) {
	cfg := DefaultConfig()
	check := func(items []ItemTemplate, eggExpected float64) {
		t.Helper()
		total, eggs := 0.0, 0.0
		byQuality := map[string]float64{}
		for _, item := range items {
			if item.AppearanceProbability == nil || math.IsNaN(*item.AppearanceProbability) || *item.AppearanceProbability < 0 || *item.AppearanceProbability > 1 {
				t.Fatalf("invalid probability: %+v", item)
			}
			total += *item.AppearanceProbability
			byQuality[item.Quality] += *item.AppearanceProbability
			if item.Category == "easter_egg" {
				if item.Quality != "easter_egg" || item.Price != 17500000 {
					t.Fatal("egg still classified as ordinary quality")
				}
				eggs += *item.AppearanceProbability
			}
		}
		for _, item := range items {
			expected := byQuality[item.Quality]
			if item.Quality != "easter_egg" {
				expected /= 1 - eggExpected
			}
			if item.QualityAppearanceProbability == nil || math.Abs(*item.QualityAppearanceProbability-expected) > 1e-12 {
				t.Fatalf("ordinary quality probability must exclude eggs: %s", item.ID)
			}
		}
		if math.Abs(total-1) > 1e-12 || math.Abs(eggs-eggExpected) > 1e-12 {
			t.Fatalf("probabilities do not match draw: total=%g eggs=%g", total, eggs)
		}
	}
	check(Catalog(cfg), 2.0/100002)
	for _, item := range Catalog(cfg) {
		for _, q := range cfg.Qualities {
			if item.Quality == q.ID && *item.QualityAppearanceProbability != float64(q.Weight)/100 {
				t.Fatalf("quality probability differs from configured weight: %s", q.ID)
			}
		}
		if item.Quality == "easter_egg" && math.Abs(*item.AppearanceProbability-2.0/100002/8) > 1e-15 {
			t.Fatal("per-item probability must divide the egg pool")
		}
	}
	g, _ := New(2)
	if err := g.SetWarehouse("tech"); err != nil {
		t.Fatal(err)
	}
	check(g.Catalog(), 2.0/100002)
	var electronics, other float64
	for _, item := range g.Catalog() {
		if item.Quality == "common" {
			if item.Category == "electronics" {
				electronics = *item.AppearanceProbability
			} else {
				other = *item.AppearanceProbability
			}
		}
	}
	if math.Abs(electronics-3*other) > 1e-12 {
		t.Fatal("warehouse preference not reflected")
	}
	for n := range cfg.Qualities {
		if cfg.Qualities[n].ID == "legendary" {
			cfg.Qualities[n].Weight = 0
			cfg.Qualities[0].Weight += 2
		}
	}
	check(Catalog(cfg), 0)
	cfg = DefaultConfig()
	ordinary := []ItemTemplate{}
	for _, item := range cfg.Catalog {
		if item.Category != "easter_egg" {
			ordinary = append(ordinary, item)
		}
	}
	cfg.Catalog = ordinary
	check(Catalog(cfg), 0)
	cfg = DefaultConfig()
	cfg.Box = Dimensions{1, 1}
	check(Catalog(cfg), 2.0/100002)
	for _, item := range Catalog(cfg) {
		if (item.Width > 1 || item.Height > 1) && *item.AppearanceProbability != 0 {
			t.Fatal("unplaceable template has nonzero probability")
		}
	}
}

func TestScoutingEffectsExcludeEggs(t *testing.T) {
	g, _ := New(2)
	g.items = []Item{{ID: "legend", Quality: "legendary", Price: 20000}, {ID: "egg", Category: "easter_egg", Quality: "easter_egg", Price: 17500000}}
	counts := g.cardEffect(cards.Effect{Type: "count", GroupBy: "quality"})["counts"].(map[string]int)
	if counts["legendary"] != 1 || len(counts) != len(g.gameConfig.Qualities) {
		t.Fatal("quality census exposed eggs", counts)
	}
	if got := g.cardEffect(cards.Effect{Type: "value", Quality: "legendary"})["total_value"]; got != int64(20000) {
		t.Fatal("legendary card included egg value", got)
	}
	if got := g.cardEffect(cards.Effect{Type: "count", Quality: "easter_egg"})["count"]; got != 0 {
		t.Fatal("egg filter exposed eggs", got)
	}
	if got := g.cardEffect(cards.Effect{Type: "value"})["total_value"]; got != int64(20000) {
		t.Fatal("aggregate value exposed egg value", got)
	}
	bad := DefaultConfig()
	for n := range bad.Catalog {
		if bad.Catalog[n].Category == "easter_egg" {
			bad.Catalog[n].Quality = "legendary"
			break
		}
	}
	if bad.Validate() == nil {
		t.Fatal("legendary-labelled egg accepted")
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
