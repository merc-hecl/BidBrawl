package game

import (
	"bytes"
	"reflect"
	"testing"

	"bidbrawl/internal/cards"
)

func TestCardRoundGateIsAtomicAndDurable(t *testing.T) {
	g, _ := New(2)
	_, a, _ := g.Join("A", []string{"quality_counts", "random_shape"})
	_, b, _ := g.Join("B", nil)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	before := mustSnapshot(t, g)
	if err := g.UseCard(a, 1, "quality_counts", "", "seek full value"); err == nil {
		t.Fatal("early audit accepted")
	}
	if !bytes.Equal(before, mustSnapshot(t, g)) {
		t.Fatal("rejected card changed costs, allowance or observations")
	}
	if err := g.UseCard(a, 1, "random_shape", "", "use an unlocked card instead"); err != nil {
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
	if err := g.UseCard(a, 5, "quality_counts", "", "value the final offer"); err != nil {
		t.Fatal(err)
	}
	after, _ := g.Observe("")
	if !bytes.Equal(public, after) || len(view(t, g, b).Self.Hints) != 0 {
		t.Fatal("audit leaked to public or peer")
	}
	v := view(t, g, a)
	if v.Self.Hints[1].Data["counts"] == nil || v.Self.CardCost != 12200 {
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
	d := g.effect(cards.Effect{Type: "average", Measure: "area", Area: 4}, nil)
	if d["average_area"] != float64(4) || len(d) != 1 {
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
	d = g.effect(cards.Effect{Type: "average", Measure: "area", Category: "medical"}, nil)
	if d["average_area"] != nil || len(d) != 1 {
		t.Fatal("empty mean invented a price", d)
	}
	for range 20 {
		d = g.effect(cards.Effect{Type: "average", Measure: "area", Area: 4, SampleSize: 1}, nil)
		if d["average_area"] != float64(4) || d["sample_count"] != 1 || d["sample_limit"] != 1 {
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

func TestChosenCardTargetIsPrivateAtomicAndSurvivesRestore(t *testing.T) {
	for _, tc := range []struct{ card, target, key string }{
		{"category_shapes", "art", "items"},
		{"target_quality_count", "legendary", "count"},
		{"region_census", "northwest", "counts"},
	} {
		t.Run(tc.card, func(t *testing.T) {
			g, _ := New(2)
			_, a, _ := g.Join("A", []string{tc.card})
			_, b, _ := g.Join("B", nil)
			if err := g.Start(); err != nil {
				t.Fatal(err)
			}
			for g.round < 3 {
				round := g.round
				for _, tok := range []string{a, b} {
					if err := g.Submit(tok, round, 100, "equal offers for target fixture"); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := mustSnapshot(t, g)
			for _, target := range []string{"", "invalid", "easter_egg"} {
				if g.UseCard(a, 3, tc.card, target, "test rejected target") == nil {
					t.Fatal("invalid target accepted", target)
				}
				if !bytes.Equal(before, mustSnapshot(t, g)) {
					t.Fatal("invalid target changed allowance, state or costs")
				}
			}
			public, _ := g.Observe("")
			if err := g.UseCard(a, 3, tc.card, tc.target, "follow my visible observations"); err != nil {
				t.Fatal(err)
			}
			d := view(t, g, a).Self.Hints[0].Data
			if d["target"] != tc.target {
				t.Fatal("chosen target not retained")
			}
			if _, ok := d[tc.key]; !ok {
				t.Fatal("missing targeted result", d)
			}
			after, _ := g.Observe("")
			if !bytes.Equal(public, after) || len(view(t, g, b).Self.Hints) > 0 {
				t.Fatal("target or result leaked")
			}
			restored, err := Restore(mustSnapshot(t, g))
			if err != nil {
				t.Fatal(err)
			}
			x, _ := g.Observe(a)
			y, _ := restored.Observe(a)
			if !bytes.Equal(x, y) {
				t.Fatal("restart changed target or result")
			}
		})
	}
}

func TestRegionalEffectsUseTopLeftCornerAndExcludeEggs(t *testing.T) {
	g, _ := New(2)
	g.width, g.height = 5, 5
	g.items = []Item{
		{ID: "nw", Quality: "rare", Price: 100, TopLeft: Point{0, 0}, BottomRight: Point{3, 3}},
		{ID: "ne", Quality: "rare", Price: 200, TopLeft: Point{2, 0}, BottomRight: Point{5, 2}},
		{ID: "sw", Quality: "rare", Price: 300, TopLeft: Point{0, 2}, BottomRight: Point{2, 5}},
		{ID: "se", Quality: "rare", Price: 400, TopLeft: Point{2, 2}, BottomRight: Point{5, 5}},
		{ID: "egg", Category: "easter_egg", Quality: "easter_egg", Price: 17500000, TopLeft: Point{2, 2}, BottomRight: Point{3, 3}},
	}
	for region, want := range map[string]int64{"northwest": 9, "northeast": 6, "southwest": 6, "southeast": 9} {
		d := g.effect(cards.Effect{Type: "area", Region: region}, nil)
		if d["area"] != want {
			t.Fatal("region overlaps, misses dividing line or reveals egg", region, d)
		}
	}
}

func TestFocusedAppraisalRequiresPersonalObservationAndPersists(t *testing.T) {
	cfg := geometryConfig(Dimensions{10, 1}, ItemLimits{1, 1})
	ordinary := []ItemTemplate{}
	for _, item := range cfg.Catalog {
		if item.Category != "easter_egg" {
			ordinary = append(ordinary, item)
		}
	}
	cfg.Catalog = ordinary
	g, err := NewWithConfig(2, cfg, cards.Default())
	if err != nil {
		t.Fatal(err)
	}
	_, a, _ := g.Join("A", []string{"focused_appraisal"})
	_, b, _ := g.Join("B", nil)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{a, b} {
		if err := g.Submit(tok, 1, 100, "equal offers for observed-target fixture"); err != nil {
			t.Fatal(err)
		}
	}
	p := g.find(a)
	known := g.knownItemFields(p)
	var hidden Item
	for _, i := range g.items {
		if known[i.ID] == nil {
			hidden = i
			break
		}
	}
	if hidden.ID == "" {
		t.Fatal("fixture has no hidden item")
	}
	before := mustSnapshot(t, g)
	for _, target := range []string{hidden.ID, "guessed-item-id", hidden.TemplateID} {
		if g.UseCard(a, 2, "focused_appraisal", target, "target not observed") == nil {
			t.Fatal("unobserved target accepted", target)
		}
		if !bytes.Equal(before, mustSnapshot(t, g)) {
			t.Fatal("unobserved target changed game")
		}
	}
	target := p.inspection[0].ID
	g.SetCardPrices(map[string]map[string]int64{p.id: {"focused_appraisal": cards.MinimumAppraisalPrice - 1}})
	before = mustSnapshot(t, g)
	if g.UseCard(a, 2, "focused_appraisal", target, "underpriced appraisal") == nil || !bytes.Equal(before, mustSnapshot(t, g)) {
		t.Fatal("underpriced stock bypassed appraisal cost or changed state")
	}
	g.SetCardPrices(map[string]map[string]int64{p.id: {"focused_appraisal": cards.MinimumAppraisalPrice}})
	public, _ := g.Observe("")
	if err := g.UseCard(a, 2, "focused_appraisal", target, "appraise a known candidate"); err != nil {
		t.Fatal(err)
	}
	v := view(t, g, a)
	if v.Self.CardCost != cards.MinimumAppraisalPrice {
		t.Fatal("appraisal cost not charged")
	}
	d := v.Self.Hints[0].Data
	if d["item_id"] != target || d["target"] != target || d["price"] == nil || d["template_id"] == nil {
		t.Fatal("chosen item was not fully appraised", d)
	}
	after, _ := g.Observe("")
	if !bytes.Equal(public, after) {
		t.Fatal("chosen item became public")
	}
	restored, err := Restore(mustSnapshot(t, g))
	if err != nil {
		t.Fatal(err)
	}
	x, _ := g.Observe(a)
	y, _ := restored.Observe(a)
	if !bytes.Equal(x, y) {
		t.Fatal("chosen appraisal changed on restart")
	}
	if len(view(t, g, b).Self.Hints) > 0 {
		t.Fatal("another player received appraisal")
	}
}

func TestUnknownExtremaAdvanceWithoutChangingFixedTargets(t *testing.T) {
	g, _ := New(2)
	g.items = []Item{
		{ID: "top", Quality: "legendary", Price: 50000, Name: "Top", TemplateID: "top", BottomRight: Point{2, 2}},
		{ID: "second", Quality: "epic", Price: 10000, Name: "Second", TemplateID: "second", BottomRight: Point{3, 3}},
		{ID: "low", Quality: "common", Price: 100, Name: "Low", TemplateID: "low", BottomRight: Point{4, 4}},
	}
	p := &player{inspection: []InspectionItem{{ID: "top", Quality: "legendary"}}}
	e := cards.Effect{Type: "reveal", Selector: "highest", Fields: []string{"quality", "price", "identity"}, UnknownOnly: true}
	for _, want := range []string{"top", "second", "low"} {
		d := g.effect(e, p)
		if d["item_id"] != want {
			t.Fatal("unknown extrema skipped an incomplete item or repeated a completed one", want, d)
		}
		p.signals = append(p.signals, Hint{Data: d})
	}
	if d := g.effect(e, p); d["count"] != 0 {
		t.Fatal("exhausted extrema repeated known information", d)
	}
	e.UnknownOnly = false
	if d := g.effect(e, p); d["item_id"] != "top" {
		t.Fatal("fixed extremum unexpectedly advanced", d)
	}
}

func TestConfiguredCardsAndSnapshot(t *testing.T) {
	cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{"audit", "census"}, Cards: []cards.Definition{
		{ID: "audit", Name: "组合全检", Description: "最大物品品质与轮廓", Price: 1, Effect: cards.Effect{Type: "reveal", Selector: "largest", Fields: []string{"quality", "shape"}}},
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
	if err := g.UseCard(a, 1, "audit", "", "protocol test"); err != nil {
		t.Fatal(err)
	}
	data := view(t, g, a).Self.Hints[0].Data
	if data["item_id"] != "a" || data["quality"] != "rare" || data["price"] != nil || len(data) != 4 {
		t.Fatal("configured combined reveal or tie selection incorrect", data)
	}
	if !reflect.DeepEqual(before, view(t, g, "")) || len(view(t, g, b).Self.Hints) != 0 {
		t.Fatal("configured card leaked private information")
	}
	_ = g.Submit(a, 1, 100, "protocol test")
	_ = g.Submit(b, 1, 100, "protocol test")
	if err := g.UseCard(a, 2, "census", "", "protocol test"); err != nil {
		t.Fatal(err)
	}
	data = view(t, g, a).Self.Hints[1].Data
	if data["count"] != float64(3) {
		t.Fatal("configured count ignored", data)
	}
	if err := g.UseCard(a, 2, "census", "", "protocol test"); err == nil {
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
