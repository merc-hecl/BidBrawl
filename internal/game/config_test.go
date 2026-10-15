package game

import (
	"strings"
	"testing"

	"bidbrawl/internal/cards"
)

func TestGameConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Box != (Dimensions{20, 30}) || cfg.Items != (ItemLimits{4, 4}) {
		t.Fatal("incorrect default geometry", cfg)
	}
	for _, raw := range []string{
		`null`,
		`{"schema_version":1,"box":{"width":0,"height":30},"items":{"max_width":4,"max_height":4}}`,
		`{"schema_version":1,"box":{"width":20,"height":30},"items":{"max_width":4,"max_height":0}}`,
		`{"schema_version":1,"box":{"width":20,"height":30},"items":{"max_width":4,"max_height":4,"typo":1}}`,
		`{"schema_version":1,"box":{"width":20.5,"height":30},"items":{"max_width":4,"max_height":4}}`,
		`{"schema_version":2,"box":{"width":20,"height":30},"items":{"max_width":4,"max_height":4}}`,
		`{"schema_version":1,"box":{"width":128,"height":128},"items":{"max_width":4,"max_height":4}}`,
		`{"schema_version":1,"box":{"width":20,"height":30},"items":{"max_width":4,"max_height":4}} {}`,
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
	if rules.GameConfig != cfg || rules.CoordinateUnit != "cell" {
		t.Fatal("public rules do not describe the configured cell grid", rules)
	}
}
