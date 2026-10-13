package game

import (
	"bytes"
	"encoding/json"
	"testing"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/hints"
)

func TestHintPlanNeverCombinesPriceCluesAndHandlesTinyBoxes(t *testing.T) {
	c := hints.Default()
	c.Plugins = append(c.Plugins, hints.Plugin{ID: "average_alias", Builtin: "quality_average"}, hints.Plugin{ID: "price_alias", Builtin: "random_quality_price"})
	g, err := NewWithHints(2, DefaultConfig(), cards.Default(), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]Item{
		{{ID: "a", Quality: "common", Price: 20, BottomRight: Point{1, 1}}},
		{{ID: "a", Quality: "common", Price: 20, BottomRight: Point{1, 1}}, {ID: "b", Quality: "rare", Price: 600, TopLeft: Point{1, 0}, BottomRight: Point{2, 1}}},
	} {
		for range 80 {
			plan, err := g.planHints(items)
			if err != nil || len(plan) != 2 {
				t.Fatal("tiny box cannot start", err)
			}
			prices := 0
			for _, h := range plan {
				b, _ := json.Marshal(h.Data)
				c, err := hints.ParseClue(bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}
				if c.Fact.IsPrice() {
					prices++
				}
			}
			if prices > 1 || len(items) == 1 && prices != 0 {
				t.Fatal("whole value could be reconstructed from price clues")
			}
		}
	}
}

func TestHintFailureLeavesLobbyUntouched(t *testing.T) {
	c := hints.Config{SchemaVersion: 1, Plugins: []hints.Plugin{
		{ID: "first", Command: []string{"nonexistent-bidbrawl-hint-executable"}},
		{ID: "second", Command: []string{"nonexistent-bidbrawl-hint-executable"}},
	}}
	g, err := NewWithHints(2, DefaultConfig(), cards.Default(), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B"} {
		if _, _, err := g.Join(name, []string{}); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := g.Snapshot()
	if err := g.Start(); err == nil {
		t.Fatal("broken plugin did not fail start")
	}
	after, _ := g.Snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("plugin failure partially started game")
	}
}

func TestPlannedHintsStayPrivateAndRestoreWithoutPlugins(t *testing.T) {
	c := hints.Config{SchemaVersion: 1, Plugins: []hints.Plugin{
		{ID: "custom_first", Builtin: "random_quality"}, {ID: "custom_second", Builtin: "quality_count"},
	}}
	g, err := NewWithHints(2, DefaultConfig(), cards.Default(), c)
	if err != nil {
		t.Fatal(err)
	}
	var tokens []string
	for _, name := range []string{"A", "B"} {
		_, token, _ := g.Join(name, []string{})
		tokens = append(tokens, token)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	// Simulate executable removal after a successful start. Already saved clues
	// must not require executing a program during restore, reads or later rounds.
	for i := range g.hintConfig.Plugins {
		g.hintConfig.Plugins[i].Builtin = ""
		g.hintConfig.Plugins[i].Command = []string{"removed-private-plugin-path"}
	}
	for _, token := range append([]string{""}, tokens...) {
		b, err := g.Observe(token)
		if err != nil {
			t.Fatal(err)
		}
		var v View
		_ = json.Unmarshal(b, &v)
		if len(v.PublicHints) != 1 || v.PublicHints[0].Round != 1 {
			t.Fatal("future hint leaked", v.PublicHints)
		}
		for _, field := range []string{"planned_hints", "hint_config", "removed-private-plugin-path"} {
			if bytes.Contains(b, []byte(field)) {
				t.Fatal("private hint settings leaked", field)
			}
		}
	}
	snapshot, _ := g.Snapshot()
	var restored *Game
	if restored, err = Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	for round := 1; round <= 2; round++ {
		for _, token := range tokens {
			if err := restored.Submit(token, round, 100, "protocol test"); err != nil {
				t.Fatal(err)
			}
		}
	}
	got, _ := restored.Observe("")
	var v View
	_ = json.Unmarshal(got, &v)
	if len(v.PublicHints) != 2 || v.PublicHints[1].Round != 3 {
		t.Fatal("scheduled hint missing")
	}
	want, _ := json.Marshal(g.plannedHints[1])
	actual, _ := json.Marshal(v.PublicHints[1])
	if !bytes.Equal(want, actual) {
		t.Fatal("third round hint was rerolled")
	}
}
