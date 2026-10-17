package game

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"bidbrawl/internal/cards"
)

func geometryConfig(box Dimensions, items ItemLimits) Config {
	cfg := DefaultConfig()
	cfg.Box, cfg.Items = box, items
	return cfg
}

func TestGameConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Box != (Dimensions{20, 30}) || cfg.Items != (ItemLimits{4, 4}) {
		t.Fatal("incorrect default geometry", cfg)
	}
	rawDefault := string(defaultConfigJSON)
	for _, raw := range []string{
		`null`,
		strings.Replace(rawDefault, `"width": 20`, `"width": 0`, 1),
		strings.Replace(rawDefault, `"max_height": 4`, `"max_height": 0`, 1),
		strings.Replace(rawDefault, `"max_height": 4`, `"max_height": 4,"typo":1`, 1),
		strings.Replace(rawDefault, `"width": 20`, `"width": 20.5`, 1),
		strings.Replace(rawDefault, `"schema_version": 2`, `"schema_version": 1`, 1),
		strings.Replace(strings.Replace(rawDefault, `"width": 20`, `"width": 128`, 1), `"height": 30`, `"height": 128`, 1),
		rawDefault + " {}",
	} {
		if _, err := ParseConfig(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid game configuration accepted", raw)
		}
	}
	g, err := NewWithConfig(2, cfg, cards.Default())
	if err != nil {
		t.Fatal(err)
	}
	rules := view(t, g, "").Rules
	if !reflect.DeepEqual(rules.GameConfig, cfg) || rules.CoordinateUnit != "cell" {
		t.Fatal("public rules do not describe the configured cell grid", rules)
	}
}

func TestEconomyConfigValidation(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"missing tiers":         func(c *Config) { c.Qualities = nil },
		"duplicate tier":        func(c *Config) { c.Qualities[1].ID = c.Qualities[0].ID },
		"unknown tier":          func(c *Config) { c.Qualities[0].ID = "mythic" },
		"empty name":            func(c *Config) { c.Qualities[0].Name = " " },
		"negative price":        func(c *Config) { c.Qualities[0].Min = -1 },
		"reversed range":        func(c *Config) { c.Qualities[0].Max = c.Qualities[0].Min - 1 },
		"unsafe price":          func(c *Config) { c.Qualities[0].Max = 1_000_000_001 },
		"negative weight":       func(c *Config) { c.Qualities[0].Weight = -1 },
		"weight total":          func(c *Config) { c.Qualities[0].Weight++ },
		"zero initial assets":   func(c *Config) { c.InitialAssets = 0 },
		"unsafe initial assets": func(c *Config) { c.InitialAssets = 1_000_000_000_001 },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			change(&cfg)
			raw, _ := json.Marshal(cfg)
			if _, err := ParseConfig(bytes.NewReader(raw)); err == nil {
				t.Fatal("invalid economy accepted")
			}
		})
	}
}

func TestConfiguredPricesWeightsBidsAndRestore(t *testing.T) {
	for _, chosen := range DefaultConfig().Qualities {
		t.Run(chosen.ID, func(t *testing.T) {
			cfg := geometryConfig(Dimensions{4, 4}, ItemLimits{1, 1})
			cfg.InitialAssets = 8_000_000
			cfg.Catalog = []ItemTemplate{}
			for _, item := range builtinCatalog.Items {
				if item.Category != "easter_egg" {
					cfg.Catalog = append(cfg.Catalog, item)
				}
			}
			for i := range cfg.Qualities {
				q := &cfg.Qualities[i]
				q.Weight = 0
				if q.ID == chosen.ID {
					q.Min, q.Max, q.Weight = 1_234_567, 1_234_567, 100
				}
			}
			raw, _ := json.Marshal(cfg)
			loaded, err := ParseConfig(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			g, err := NewWithConfig(2, loaded, cards.Default())
			if err != nil {
				t.Fatal(err)
			}
			// Input and returned configuration must not alias a live room.
			loaded.Qualities[0].Max = 99
			returned := g.GameConfig()
			returned.Qualities[0].Max = 88
			if !reflect.DeepEqual(g.GameConfig(), cfg) {
				t.Fatal("caller changed room configuration")
			}
			_, a, _ := g.Join("A", []string{"largest_price", "quality_counts"})
			_, b, _ := g.Join("B", nil)
			if err = g.Start(); err != nil {
				t.Fatal(err)
			}
			for _, item := range g.items {
				if item.Quality != chosen.ID || item.Price != 1_234_567 {
					t.Fatal("generator ignored configured prices/weights")
				}
			}
			if err = g.UseCard(a, 1, "largest_price", "protocol fixture"); err != nil {
				t.Fatal(err)
			}
			if got := view(t, g, a).Self.Hints[0].Data["price"]; got != float64(1_234_567) {
				t.Fatal("skill used stale prices", got)
			}
			if err = g.Submit(a, 1, cfg.InitialAssets+1, "protocol fixture"); err == nil {
				t.Fatal("custom bid cap ignored")
			}
			if err = g.Submit(a, 1, cfg.InitialAssets-cfg.TicketFee, "protocol fixture"); err != nil {
				t.Fatal(err)
			}
			before, _ := g.Observe(a)
			snapshot, err := g.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Restore(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := restored.Observe(a)
			if !bytes.Equal(before, after) {
				t.Fatal("restore changed prices, private result or bid limit")
			}
			if err = restored.Withdraw(b, 1, "protocol fixture"); err != nil {
				t.Fatal(err)
			}
			settled := view(t, restored, "")
			if settled.Settlement == nil || settled.Settlement.Total != int64(len(g.items))*1_234_567 {
				t.Fatal("incorrect configured settlement")
			}
			if !reflect.DeepEqual(settled.Rules.Qualities, cfg.Qualities) {
				t.Fatal("public rules use stale economy")
			}
			var invalid gameSnapshot
			_ = json.Unmarshal(snapshot, &invalid)
			for i := range invalid.Config.Qualities {
				if invalid.Config.Qualities[i].ID == chosen.ID {
					invalid.Config.Qualities[i].Min = 1_234_568
					invalid.Config.Qualities[i].Max = 1_234_568
				}
			}
			corrupt, _ := json.Marshal(invalid)
			if _, err = Restore(corrupt); err == nil {
				t.Fatal("snapshot price outside saved configuration accepted")
			}
		})
	}
}

func TestBidsUseEachPlayersAssetsIncludingZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TicketFee = 0
	g, _ := NewWithConfig(2, cfg, cards.Default())
	_, a, _ := g.Join("A", nil)
	_, b, _ := g.Join("B", nil)
	if err := g.StartWithAssets(map[string]int64{"p1": 0, "p2": 3000000}); err != nil {
		t.Fatal(err)
	}
	if view(t, g, a).Self.MaxBid != 0 || view(t, g, b).Self.MaxBid != 3000000 {
		t.Fatal("players share a fixed limit")
	}
	if err := g.Submit(a, 1, 1, "protocol fixture"); err == nil {
		t.Fatal("zero-assets player can overbid")
	}
	if err := g.Withdraw(a, 1, "protocol fixture"); err != nil {
		t.Fatal(err)
	}
	if err := g.Submit(b, 1, 3000001, "protocol fixture"); err == nil {
		t.Fatal("overbid accepted")
	}
	if err := g.Submit(b, 1, 3000000, "protocol fixture"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := g.Snapshot()
	if _, err := Restore(snapshot); err != nil {
		t.Fatal(err)
	}
}
