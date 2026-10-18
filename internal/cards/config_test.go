package cards

import (
	"strings"
	"testing"
)

func TestConfigurationValidation(t *testing.T) {
	if _, err := Parse(strings.NewReader(`{"schema_version":1,"default_deck":[],"cards":[]}`)); err != nil {
		t.Fatal("empty catalog should be supported", err)
	}
	for _, data := range []string{
		`null`,
		`{"schema_version":2,"default_deck":[],"cards":[]}`,
		`{"schema_version":1,"default_deck":null,"cards":[]}`,
		`{"schema_version":1,"default_deck":[],"cards":[],"typo":true}`,
		`{"schema_version":1,"default_deck":["missing"],"cards":[]}`,
		`{"schema_version":1,"default_deck":[],"cards":[]} {}`,
	} {
		if _, err := Parse(strings.NewReader(data)); err == nil {
			t.Fatal("invalid configuration accepted", data)
		}
	}
	for _, effect := range []Effect{
		{Type: "unknown"},
		{Type: "reveal", Selector: "largest"},
		{Type: "reveal", Selector: "anything", Fields: []string{"price"}},
		{Type: "reveal", Selector: "random", Fields: []string{"token"}},
		{Type: "reveal", Selector: "random", Fields: []string{"price", "price"}},
		{Type: "reveal", Selector: "random", Fields: []string{"price"}, GroupBy: "quality"},
		{Type: "count", GroupBy: "name"},
		{Type: "count", Selector: "largest"},
		{Type: "average"},
		{Type: "average", Measure: "price", Fields: []string{"price"}},
		{Type: "average", Measure: "price", SampleSize: -1},
		{Type: "value", Measure: "price"},
		{Type: "count", SampleSize: 2},
		{Type: "count", Quality: "rare", Qualities: []string{"rare"}},
		{Type: "count", Qualities: []string{"rare", "rare"}},
		{Type: "count", Qualities: []string{"unknown"}},
		{Type: "count", Area: -1},
		{Type: "count", Area: 16385},
		{Type: "count", Region: "center"},
		{Type: "count", Qualities: []string{""}},
		{Type: "count", UnknownOnly: true},
	} {
		cfg := Config{SchemaVersion: 1, DefaultDeck: []string{}, Cards: []Definition{{ID: "new_card", Name: "新卡", Description: "测试", Price: 1, Effect: effect}}}
		if err := cfg.Validate(); err == nil {
			t.Fatal("invalid effect accepted", effect)
		}
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Cards = append(c.Cards, c.Cards[0]) },
		func(c *Config) { c.Cards[0].ID = "bad id" },
		func(c *Config) { c.Cards[0].Name = "" },
		func(c *Config) { c.Cards[0].Price = 0 },
		func(c *Config) { c.Cards[0].MinRound = 6 },
		func(c *Config) { c.Cards[0].MinRound = -1 },
		func(c *Config) { c.Cards[0].Price = -1 },
		func(c *Config) { c.Cards[0].Price = 1_000_000_001 },
		func(c *Config) { c.DefaultDeck = []string{"largest_price", "largest_price"} },
	} {
		cfg := Default()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
}

func TestTargetDefinitionsRejectConflictsAndResolveExplicitChoices(t *testing.T) {
	for _, tc := range []struct {
		kind, target string
		effect       Effect
	}{
		{"category", "antiques", Effect{Type: "count"}},
		{"quality", "legendary", Effect{Type: "count"}},
		{"area", "6", Effect{Type: "average", Measure: "price"}},
		{"region", "southeast", Effect{Type: "value"}},
	} {
		d := Definition{ID: "target", Name: "Target", Description: "Target test", Price: 1, Target: tc.kind, Effect: tc.effect}
		c := Config{SchemaVersion: 1, DefaultDeck: []string{}, Cards: []Definition{d}}
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		resolved, err := d.ResolveTarget(tc.target)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.validate() != nil {
			t.Fatal("invalid target resolution")
		}
		if _, err := d.ResolveTarget(""); err == nil {
			t.Fatal("target silently defaulted", tc.kind)
		}
		if _, err := d.ResolveTarget("easter_egg"); err == nil {
			t.Fatal("target allowed hidden tier", tc.kind)
		}
		if d.Effect.Category != "" || d.Effect.Quality != "" || d.Effect.Area != 0 || d.Effect.Region != "" {
			t.Fatal("target mutated frozen definition")
		}
	}
	for _, d := range []Definition{
		{Target: "unknown", Effect: Effect{Type: "count"}},
		{Target: "category", Effect: Effect{Type: "count", Category: "art"}},
		{Target: "quality", Effect: Effect{Type: "count", Qualities: []string{"epic"}}},
		{Target: "region", Effect: Effect{Type: "count", Region: "northwest"}},
		{Target: "area", Effect: Effect{Type: "count", Area: 4}},
	} {
		d.ID, d.Name, d.Description, d.Price = "bad", "Bad", "Bad", 1
		c := Config{SchemaVersion: 1, DefaultDeck: []string{}, Cards: []Definition{d}}
		if c.Validate() == nil {
			t.Fatal("conflicting target accepted", d)
		}
	}
	if _, err := (Definition{Effect: Effect{Type: "count"}}).ResolveTarget("art"); err == nil {
		t.Fatal("fixed card accepted ignored target")
	}
}

func TestDefaultAndCloneIsolation(t *testing.T) {
	first := Default()
	clone := first.Clone()
	clone.DefaultDeck[0] = "changed"
	clone.Cards[0].Effect.Fields[0] = "price"
	if first.DefaultDeck[0] == "changed" || first.Cards[0].Effect.Fields[0] == "price" {
		t.Fatal("clone shares mutable configuration")
	}
	first.Cards[0].ID = "changed"
	if Default().Cards[0].ID == "changed" {
		t.Fatal("default configuration shares state between calls")
	}
}
