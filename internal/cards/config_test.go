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
	} {
		cfg := Config{SchemaVersion: 1, DefaultDeck: []string{}, Cards: []Definition{{ID: "new_card", Name: "新卡", Description: "测试", Effect: effect}}}
		if err := cfg.Validate(); err == nil {
			t.Fatal("invalid effect accepted", effect)
		}
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Cards = append(c.Cards, c.Cards[0]) },
		func(c *Config) { c.Cards[0].ID = "bad id" },
		func(c *Config) { c.Cards[0].Name = "" },
		func(c *Config) { c.DefaultDeck = []string{"largest_price", "largest_price"} },
	} {
		cfg := Default()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatal("invalid catalog accepted")
		}
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
