package game

import (
	"bytes"
	"reflect"
	"testing"

	"bidbrawl/internal/cards"
)

func TestCardRoundGateIsAtomicAndDurable(t *testing.T) {
	g, _ := New(2)
	_, a, _ := g.Join("A", []string{"audit", "random_shape"})
	_, b, _ := g.Join("B", nil)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	before := mustSnapshot(t, g)
	if err := g.UseCard(a, 1, "audit", "seek full value"); err == nil {
		t.Fatal("early audit accepted")
	}
	if !bytes.Equal(before, mustSnapshot(t, g)) {
		t.Fatal("rejected card changed costs, allowance or observations")
	}
	if err := g.UseCard(a, 1, "random_shape", "use an unlocked card instead"); err != nil {
		t.Fatal(err)
	}
	for round := 1; round < 5; round++ {
		for _, tok := range []string{a, b} {
			if err := g.Submit(tok, round, 100, "equal offers for gate fixture"); err != nil {
				t.Fatal(err)
			}
		}
	}
	g, _ = Restore(mustSnapshot(t, g))
	public, _ := g.Observe("")
	if err := g.UseCard(a, 5, "audit", "value the final offer"); err != nil {
		t.Fatal(err)
	}
	after, _ := g.Observe("")
	if !bytes.Equal(public, after) || len(view(t, g, b).Self.Hints) != 0 {
		t.Fatal("audit leaked to public or peer")
	}
	var expected int64
	for _, i := range scoutableItems(g.items) {
		expected += i.Price
	}
	v := view(t, g, a)
	if v.Self.Hints[1].Data["total_value"] != float64(expected) || v.Self.CardCost != 35200 {
		t.Fatal("unlocked audit result or cost incorrect")
	}
	restored, err := Restore(mustSnapshot(t, g))
	if err != nil {
		t.Fatal(err)
	}
	x, _ := g.Observe(a)
	y, _ := restored.Observe(a)
	if !bytes.Equal(x, y) {
		t.Fatal("restart changed learned audit")
	}
}

func TestFilteredMeansAndCombinedTiers(t *testing.T) {
	g, _ := New(2)
	g.items = []Item{
		{ID: "a", Category: "antiques", Quality: "rare", Price: 100, BottomRight: Point{2, 2}},
		{ID: "b", Category: "antiques", Quality: "epic", Price: 900, BottomRight: Point{1, 4}},
		{ID: "c", Category: "tools", Quality: "legendary", Price: 5000, BottomRight: Point{1, 1}},
		{ID: "egg", Category: "easter_egg", Quality: "easter_egg", Price: 17500000, BottomRight: Point{1, 1}},
	}
	d := g.effect(cards.Effect{Type: "average", Measure: "price", Area: 4}, nil)
	if d["average_price"] != float64(500) || len(d) != 1 {
		t.Fatal("area mean or count privacy incorrect", d)
	}
	d = g.effect(cards.Effect{Type: "average", Measure: "area", Qualities: []string{"epic", "legendary"}}, nil)
	if d["average_area"] != float64(2.5) || len(d) != 1 {
		t.Fatal("combined mean incorrect", d)
	}
	d = g.effect(cards.Effect{Type: "count", Qualities: []string{"epic", "legendary"}}, nil)
	if d["count"] != 2 || len(d) != 1 {
		t.Fatal("combined tiers revealed split counts", d)
	}
	d = g.effect(cards.Effect{Type: "count", GroupBy: "quality", Qualities: []string{"common", "epic"}}, nil)
	if !reflect.DeepEqual(d["counts"], map[string]int{"common": 0, "epic": 1}) {
		t.Fatal("group counts escaped filter", d)
	}
	d = g.effect(cards.Effect{Type: "average", Measure: "price", Category: "medical"}, nil)
	if d["average_price"] != nil || len(d) != 1 {
		t.Fatal("empty mean invented a price", d)
	}
	for range 20 {
		d = g.effect(cards.Effect{Type: "average", Measure: "price", Area: 4, SampleSize: 1}, nil)
		if (d["average_price"] != float64(100) && d["average_price"] != float64(900)) || d["sample_count"] != 1 || d["sample_limit"] != 1 {
			t.Fatal("sample leaked population mean", d)
		}
	}
	if !cards.ValidCategory("antiques") {
		t.Fatal("catalog category rejected by cards")
	}
	for _, c := range Categories {
		if !cards.ValidCategory(c.ID) {
			t.Fatal("card and game categories diverged", c.ID)
		}
	}
}

func TestConfiguredCardsAndSnapshot(t *testing.T) {
	cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{"audit", "census"}, Cards: []cards.Definition{
		{ID: "audit", Name: "组合全检", Description: "最大物品全部信息", Price: 1, Effect: cards.Effect{Type: "reveal", Selector: "largest", Fields: []string{"quality", "price", "shape"}}},
		{ID: "census", Name: "清点", Description: "物品总数", Price: 1, Effect: cards.Effect{Type: "count"}},
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
	_ = g.Submit(a, 1, 100, "protocol test")
	_ = g.Submit(b, 1, 100, "protocol test")
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
