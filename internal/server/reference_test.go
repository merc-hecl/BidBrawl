package server

import (
	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	"bidbrawl/internal/hints"
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicReferenceUsesConfiguredCatalogAndSafeClueDefinitions(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Catalog = []game.ItemTemplate{{ID: "custom", Name: "Host item", Category: "tools", Quality: "common", Width: 1, Height: 1, ValuePercent: 50, Translations: map[string]string{"en": "Host translation"}}}
	cfg.Qualities[0].Min = 1000
	cfg.Qualities[0].Max = 2000
	hc := hints.Config{SchemaVersion: 1, Plugins: []hints.Plugin{{ID: "alias", Builtin: "quality_count"}, {ID: "external", Command: []string{"PRIVATE_EXECUTABLE", "PRIVATE_ARGUMENT"}}}}
	s := NewWithHints(cfg, cards.Default(), hc)
	defer s.Close()
	code, body := request(t, s, "GET", "/api/config", "", "")
	if code != 200 {
		t.Fatalf("config status: %d", code)
	}
	var got struct {
		Catalog []game.ItemTemplate `json:"catalog"`
		Hints   []hints.Definition  `json:"public_hint_definitions"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Catalog) != 1 || got.Catalog[0].Price != 1500 || got.Catalog[0].Translations["en"] != "Host translation" {
		t.Fatalf("configured reference lost: %+v", got.Catalog)
	}
	if len(got.Hints) != 2 || got.Hints[0].Builtin != "quality_count" || got.Hints[1].Builtin != "" {
		t.Fatalf("clue capabilities lost: %+v", got.Hints)
	}
	for _, secret := range []string{"PRIVATE_EXECUTABLE", "PRIVATE_ARGUMENT", s.HostToken, s.ObserverToken} {
		if strings.Contains(string(body), secret) {
			t.Fatal("public config exposed a private value")
		}
	}
}
