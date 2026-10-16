package game

import (
	"reflect"
	"testing"

	"bidbrawl/internal/cards"
)

func TestConfiguredCardsAndSnapshot(t *testing.T) {
	cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{"audit", "census"}, Cards: []cards.Definition{
		{ID: "audit", Name: "组合全检", Description: "最大物品全部信息", Effect: cards.Effect{Type: "reveal", Selector: "largest", Fields: []string{"quality", "price", "shape"}}},
		{ID: "census", Name: "清点", Description: "物品总数", Effect: cards.Effect{Type: "count"}},
	}}
	g, err := NewWithCards(2, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Cards[0].Effect.Fields[0] = "invalid"
	cfg.DefaultDeck[0] = "missing"
	copy := g.CardConfig()
	copy.Cards[0].ID = "also_missing"
	if _, _, err := g.Join("invalid", []string{"largest_price"}); err == nil {
		t.Fatal("removed built-in card accepted")
	}
	_, a, _ := g.Join("A", []string{"audit", "census"})
	_, b, _ := g.Join("B", nil)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	g.items = []Item{
		{ID: "b", Quality: "common", Price: 100, TopLeft: Point{0, 0}, BottomRight: Point{2, 2}},
		{ID: "a", Quality: "rare", Price: 500, TopLeft: Point{2, 0}, BottomRight: Point{4, 2}},
		{ID: "c", Quality: "common", Price: 40, TopLeft: Point{4, 0}, BottomRight: Point{5, 1}},
	}
	before := view(t, g, "")
	if err := g.UseCard(a, 1, "audit", "protocol test"); err != nil {
		t.Fatal(err)
	}
	data := view(t, g, a).Self.Hints[0].Data
	if data["item_id"] != "a" || data["quality"] != "rare" || data["price"] != float64(500) || len(data) != 5 {
		t.Fatal("configured combined reveal or tie selection incorrect", data)
	}
	if !reflect.DeepEqual(before, view(t, g, "")) || len(view(t, g, b).Self.Hints) != 0 {
		t.Fatal("configured card leaked private information")
	}
	_ = g.Submit(a, 1, 0, "protocol test")
	_ = g.Submit(b, 1, 0, "protocol test")
	if err := g.UseCard(a, 2, "census", "protocol test"); err != nil {
		t.Fatal(err)
	}
	data = view(t, g, a).Self.Hints[1].Data
	if data["count"] != float64(3) {
		t.Fatal("configured count ignored", data)
	}
	if err := g.UseCard(a, 2, "census", "protocol test"); err == nil {
		t.Fatal("configured card reused")
	}
	if !reflect.DeepEqual(view(t, g, "").Rules.DefaultDeck, []string{"audit", "census"}) {
		t.Fatal("caller changed configuration snapshot")
	}
}

func TestEffectPrimitives(t *testing.T) {
	g, _ := New(2)
	g.items = []Item{
		{ID: "b", Quality: "common", Price: 17, TopLeft: Point{0, 0}, BottomRight: Point{1, 1}},
		{ID: "a", Quality: "rare", Price: 500, TopLeft: Point{1, 0}, BottomRight: Point{2, 1}},
		{ID: "c", Quality: "common", Price: 75, TopLeft: Point{2, 0}, BottomRight: Point{4, 2}},
	}
	data := g.cardEffect(cards.Effect{Type: "reveal", Selector: "smallest", Fields: []string{"price"}})
	if data["item_id"] != "a" || data["price"] != int64(500) || len(data) != 2 {
		t.Fatal("smallest selection or field projection incorrect", data)
	}
	data = g.cardEffect(cards.Effect{Type: "count", GroupBy: "quality"})
	counts := data["counts"].(map[string]int)
	if counts["common"] != 2 || counts["rare"] != 1 || counts["epic"] != 0 || len(counts) != len(g.gameConfig.Qualities) {
		t.Fatal("incorrect configured grouped count", counts)
	}
	if g.cardEffect(cards.Effect{Type: "count"})["count"] != 3 {
		t.Fatal("incorrect count")
	}
	g.items = g.items[:1]
	if data := g.cardEffect(cards.Effect{Type: "reveal", Selector: "random", Fields: []string{"quality"}}); data["item_id"] != "b" || len(data) != 2 {
		t.Fatal("random selection or projection incorrect", data)
	}
}
