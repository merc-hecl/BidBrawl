package game

import (
	"bidbrawl/internal/cards"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRandomCardsRespectLearnedLocationsAndPublicFacts(t *testing.T) {
	g, _ := New(2)
	g.items = []Item{{ID: "a", Quality: "common", TopLeft: Point{0, 0}, BottomRight: Point{1, 1}}, {ID: "b", Quality: "rare", TopLeft: Point{1, 0}, BottomRight: Point{2, 1}}}
	p := &player{hints: []Hint{{Data: map[string]any{"item_id": "a", "quality": "common"}}}}
	result := g.effect(cards.Effect{Type: "reveal", Selector: "random", Fields: []string{"shape"}}, p)
	if result["item_id"] != "b" {
		t.Fatal("paid scan repeated a location already learned through appraisal")
	}
	g.hints = []Hint{{Data: map[string]any{"fact": map[string]any{"item_id": "b", "type": "random_shape", "top_left": Point{1, 0}, "bottom_right": Point{2, 1}}}}}
	result = g.effect(cards.Effect{Type: "reveal", Selector: "random", Fields: []string{"shape"}}, p)
	if result["count"] != 0 {
		t.Fatal("exhausted scan rediscovered a public shape")
	}
}

func TestSpecialtiesSurviveRestartAndNeverReachPublicView(t *testing.T) {
	g, _ := New(4)
	tokens := []string{}
	for i, id := range []string{"survey", "category", "analysis", "late"} {
		_, token, _ := g.Join(id, nil)
		category := ""
		if id == "category" {
			category = "household"
		}
		if err := g.Configure(token, id, category, nil); err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
		_ = i
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	for round := 1; round <= 5; round++ {
		public, _ := g.Observe("")
		if bytes.Contains(public, []byte(`"intelligence"`)) || bytes.Contains(public, []byte(`"self"`)) {
			t.Fatal("private intel leaked")
		}
		for _, token := range tokens {
			v := view(t, g, token)
			expected := 0
			for _, step := range findSpecialty(Specialties, v.Self.Specialty).Steps {
				if step.Round <= round {
					expected++
				}
			}
			if len(v.Self.Intelligence) != expected {
				t.Fatal("missing round intelligence")
			}
		}
		snapshot, _ := g.Snapshot()
		restored, err := Restore(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := g.ObserveObserver()
		b, _ := restored.ObserveObserver()
		if !bytes.Equal(a, b) {
			t.Fatal("restart rerolled intelligence")
		}
		g = restored
		for _, token := range tokens {
			if err := g.Submit(token, round, 100, "keep auction open for protocol coverage"); err != nil {
				t.Fatal(err)
			}
		}
	}
	late := view(t, g, tokens[3]).Self
	if len(late.Intelligence) != 3 {
		t.Fatal("late specialty timing changed")
	}
}

func TestWithdrawalSkipsFutureRoundsAndStopsPrivateGrowth(t *testing.T) {
	g, tokens := fixture(t, 3)
	if err := g.Withdraw(tokens[0], 1, "withdraw"); err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens[1:] {
		if err := g.Submit(token, 1, 100, "continue"); err != nil {
			t.Fatal(err)
		}
	}
	v := view(t, g, tokens[0])
	if !v.Self.Withdrawn || v.Self.Bid == nil || *v.Self.Bid != 0 || len(v.Self.Intelligence) != 1 {
		t.Fatal("withdrawal did not freeze future participation")
	}
	if g.UseCard(tokens[0], 2, "item_count", "", "no longer playing") == nil {
		t.Fatal("withdrawn player used card")
	}
	if _, err := Restore(mustSnapshot(t, g)); err != nil {
		t.Fatal(err)
	}
}
func mustSnapshot(t *testing.T, g *Game) []byte {
	t.Helper()
	b, err := g.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCatalogIsDataDrivenAndInvalidFilesFail(t *testing.T) {
	cfg := DefaultConfig()
	catalog := Catalog(cfg)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(catalog) != len(builtinCatalog.Items) {
		t.Fatal("default limits exclude built-in templates")
	}
	g, _ := New(2)
	items := g.generate()
	byID := map[string]ItemTemplate{}
	for _, v := range catalog {
		byID[v.ID] = v
	}
	for _, i := range items {
		v, ok := byID[i.TemplateID]
		if !ok || i.Name != v.Name || i.Category != v.Category || i.Price != v.Price || i.area() != v.Width*v.Height {
			t.Fatal("generated item differs from catalog")
		}
	}
	custom := cfg.Clone()
	custom.Catalog[0].Name = "数据驱动测试物品"
	if reflect.DeepEqual(custom.Catalog, cfg.Catalog) {
		t.Fatal("catalog clone aliases original")
	}
	path := filepath.Join(t.TempDir(), "items.json")
	raw, _ := json.Marshal(ItemCatalog{Items: custom.Catalog})
	_ = os.WriteFile(path, raw, 0600)
	loaded, err := LoadCatalog(path)
	if err != nil || loaded[0].Name != custom.Catalog[0].Name {
		t.Fatal("custom JSON not loaded")
	}
	_ = os.WriteFile(path, []byte(`{"items":[],"typo":true}`), 0600)
	if _, err := LoadCatalog(path); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestAllSpecialtiesSchedulesAndPersistedScopes(t *testing.T) {
	if len(Specialties) != 26 {
		t.Fatal("specialty pool incomplete")
	}
	for _, spec := range Specialties {
		t.Run(spec.ID, func(t *testing.T) {
			g, _ := New(2)
			_, a, _ := g.Join("A", nil)
			_, b, _ := g.Join("B", nil)
			if err := g.Configure(a, spec.ID, "", nil); err != nil {
				t.Fatal(err)
			}
			if err := g.Start(); err != nil {
				t.Fatal(err)
			}
			for round := 1; round <= 5; round++ {
				v := view(t, g, a)
				expected := 0
				for _, step := range spec.Steps {
					if step.Round <= round && step.Condition == "" {
						expected++
					}
				}
				if len(v.Self.Intelligence) != expected {
					t.Fatal("schedule mismatch", round)
				}
				for _, h := range v.Self.Intelligence {
					for _, id := range signalItemIDs(h.Data) {
						for _, i := range g.items {
							if i.ID == id && len(spec.Categories) > 0 {
								found := false
								for _, category := range spec.Categories {
									found = found || i.Category == category
								}
								if !found {
									t.Fatal("cross-category disclosure")
								}
							}
						}
					}
				}
				restored, err := Restore(mustSnapshot(t, g))
				if err != nil {
					t.Fatal(err)
				}
				before, _ := g.ObserveObserver()
				after, _ := restored.ObserveObserver()
				if !bytes.Equal(before, after) {
					t.Fatal("restart changed specialty result")
				}
				g = restored
				for _, tok := range []string{a, b} {
					if err := g.Submit(tok, round, 100, "equal offers for schedule fixture"); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestEggProbability(t *testing.T) {
	cfg := DefaultConfig()
	counts := map[string]int{}
	for roll := 0; roll < 100002; roll++ {
		quality, egg := catalogTier(cfg.Qualities, 2, roll)
		if egg {
			if quality != "easter_egg" {
				t.Fatal("egg draw returned an ordinary quality")
			}
			quality = "egg"
		}
		counts[quality]++
	}
	if counts["egg"] != 2 || counts["legendary"] != 2000 || counts["common"] != 45000 {
		t.Fatal("egg probability must be highest tier / 1000", counts)
	}
	counts = map[string]int{}
	for roll := 0; roll < 100000; roll++ {
		quality, egg := catalogTier(cfg.Qualities, 0, roll)
		if egg {
			t.Fatal("missing egg pool was sampled")
		}
		counts[quality]++
	}
	if counts["legendary"] != 2000 || counts["common"] != 45000 {
		t.Fatal("ordinary catalog weights changed")
	}
}

func TestScoutingResultsAreUnaffectedByEggs(t *testing.T) {
	g, _ := New(2)
	ordinary := []Item{{ID: "ordinary", TemplateID: "test", Name: "Ordinary", Category: "household", Quality: "common", Price: 100, BottomRight: Point{1, 1}}}
	egg := Item{ID: "egg", Category: "easter_egg", Quality: "easter_egg", Price: 17500000, TopLeft: Point{1, 0}, BottomRight: Point{2, 1}}
	for _, lot := range [][]Item{ordinary, nil} {
		withEgg := append(append([]Item{}, lot...), egg)
		for _, card := range cards.Default().Cards {
			g.items = lot
			before := g.effect(card.Effect, nil)
			g.items = withEgg
			if after := g.effect(card.Effect, nil); !reflect.DeepEqual(before, after) {
				t.Fatalf("card %s exposed egg: before=%v after=%v", card.ID, before, after)
			}
		}
		for _, spec := range g.gameConfig.Specialties {
			before, after := &player{specialty: spec.ID}, &player{specialty: spec.ID}
			for g.round = 1; g.round <= Rounds; g.round++ {
				g.items = lot
				g.giveIntelligence(before)
				g.items = withEgg
				g.giveIntelligence(after)
			}
			if !reflect.DeepEqual(before.signals, after.signals) {
				t.Fatalf("specialty %s exposed egg: before=%v after=%v", spec.ID, before.signals, after.signals)
			}
		}
	}
}

func TestAverageIntelDoesNotGiveFreeTotalAndSampleAverageIsDistinct(t *testing.T) {
	g, _ := New(2)
	g.items = []Item{{ID: "a", Category: "household", Quality: "epic", Price: 100}, {ID: "b", Category: "household", Quality: "epic", Price: 100}, {ID: "c", Category: "household", Quality: "epic", Price: 100}, {ID: "d", Category: "household", Quality: "epic", Price: 1000}}
	p := &player{specialty: "appraisal"}
	g.round = 2
	g.giveIntelligence(p)
	d := p.signals[0].Data
	if _, ok := d["count"]; ok {
		t.Fatal("average exposed total candidate count")
	}
	if (d["average_price"] != float64(100) && d["average_price"] != float64(400)) || d["sample_count"] != 3 || d["sample_limit"] != 3 {
		t.Fatal("early appraisal gave full population average or incorrect sample", d)
	}
	g.round = 4
	g.giveIntelligence(p)
	if p.signals[1].Data["total_value"] != int64(1300) {
		t.Fatal("late appraisal no longer confirms the scoped total")
	}
	p = &player{specialty: "category"}
	g.round = 3
	g.giveIntelligence(p)
	d = p.signals[0].Data
	if d["sample_count"] != 3 || d["sample_limit"] != 3 || d["average_price"] == float64(325) {
		t.Fatal("sample average silently returned full population average")
	}
	g.round = 5
	g.giveIntelligence(p)
	if p.signals[1].Data["total_value"] != int64(1300) {
		t.Fatal("later full value changed")
	}
	g.items = nil
	g.round = 2
	p = &player{specialty: "appraisal"}
	g.giveIntelligence(p)
	if p.signals[0].Data["average_price"] != nil {
		t.Fatal("empty average invented a value")
	}
}

func TestBidConditionedIntelUsesOnlyRevealedHistory(t *testing.T) {
	g, _ := New(2)
	_, a, _ := g.Join("A", nil)
	_, b, _ := g.Join("B", nil)
	if err := g.Configure(a, "momentum", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := g.Configure(b, "contrarian", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		a, b    int64
		signals int
	}{{100, 90, 2}, {100, 100, 2}, {110, 100, 3}} {
		round := g.round
		before := view(t, g, a).Self.Intelligence
		if err := g.Submit(a, round, r.a, "test an eligible public bid position"); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, view(t, g, a).Self.Intelligence) {
			t.Fatal("sealed bid triggered intelligence")
		}
		if err := g.Submit(b, round, r.b, "test the previous revealed-round condition"); err != nil {
			t.Fatal(err)
		}
		for _, tok := range []string{a, b} {
			if len(view(t, g, tok).Self.Intelligence) != r.signals {
				t.Fatal("wrong conditional trigger", round, view(t, g, tok).Self.Intelligence)
			}
		}
		public, _ := g.Observe("")
		if bytes.Contains(public, []byte(`"intelligence"`)) {
			t.Fatal("conditional private information leaked")
		}
		restored, err := Restore(mustSnapshot(t, g))
		if err != nil {
			t.Fatal(err)
		}
		x, _ := g.Observe(a)
		y, _ := restored.Observe(a)
		if !bytes.Equal(x, y) {
			t.Fatal("restart rerolled conditional rewards")
		}
		g = restored
	}
	if err := g.Submit(a, 4, 130, "close at the existing fourth-round multiplier"); err != nil {
		t.Fatal(err)
	}
	if err := g.Submit(b, 4, 100, "finalize multiplier fixture"); err != nil {
		t.Fatal(err)
	}
	if g.phase != "settled" || g.settlement.Round != 4 || len(view(t, g, a).Self.Intelligence) != 3 {
		t.Fatal("early settlement granted future rewards or changed")
	}
}

func TestSpecialtyConditionsIgnoreTiesZeroAndMissingHistory(t *testing.T) {
	g, _ := New(2)
	p := &player{id: "a"}
	g.round = 2
	for _, bids := range [][]Bid{nil, {{"a", 0}, {"b", 0}}, {{"a", 100}, {"b", 100}}, {{"a", 0}, {"b", 100}}, {{"b", 100}}} {
		g.history = []RoundResult{{Round: 1, Bids: bids}}
		if g.specialtyCondition(p, "previous_unique_highest") || g.specialtyCondition(p, "previous_below_highest") {
			t.Fatal("ineligible bid qualified", bids)
		}
	}
	g.history = []RoundResult{{Round: 0, Bids: []Bid{{"a", 100}, {"b", 90}}}}
	if g.specialtyCondition(p, "previous_unique_highest") {
		t.Fatal("stale history qualified")
	}
}

func TestSpecialtyConfigRejectsInvalidTriggersAndSamples(t *testing.T) {
	for _, step := range []SpecialtyStep{
		{Round: 1, Condition: "previous_unique_highest", Effect: cards.Effect{Type: "count"}},
		{Round: 2, Condition: "current_highest", Effect: cards.Effect{Type: "count"}},
		{Round: 2, FromSample: "missing", Effect: cards.Effect{Type: "count"}},
		{Round: 2, Remember: "x", Effect: cards.Effect{Type: "count"}},
		{Round: 2, Qualities: []string{"easter_egg"}, Effect: cards.Effect{Type: "count"}},
	} {
		cfg := DefaultConfig()
		cfg.Specialties = []Specialty{{ID: "bad", Name: "Bad", Steps: []SpecialtyStep{step}}}
		if cfg.Validate() == nil {
			t.Fatal("invalid specialty accepted", step)
		}
	}
}
