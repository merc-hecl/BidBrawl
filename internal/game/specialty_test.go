package game

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"bidbrawl/internal/cards"
)

func styleGame(t *testing.T, style string, deck []string) (*Game, string, string) {
	t.Helper()
	g, err := New(2)
	if err != nil {
		t.Fatal(err)
	}
	_, a, _ := g.Join("A", deck)
	_, b, _ := g.Join("B", nil)
	if err := g.Configure(a, style, "", deck); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	return g, a, b
}

func styleNextRound(t *testing.T, g *Game, a, b string) {
	t.Helper()
	round := g.round
	for _, token := range []string{a, b} {
		if err := g.Submit(token, round, 100, "equal offers for deterministic rule coverage"); err != nil {
			t.Fatal(err)
		}
	}
}

func styleChoice(t *testing.T, g *Game, token, action string, chooseTopic bool) {
	t.Helper()
	topic := ""
	if chooseTopic {
		topic = view(t, g, token).Self.SpecialtyState.Questions[0].ID
	}
	if err := g.SpecialtyAction(token, g.round, action, topic, "compare the cost and timing of private information"); err != nil {
		t.Fatal(err)
	}
}

func assertStyleRestore(t *testing.T, g *Game) *Game {
	t.Helper()
	before := mustSnapshot(t, g)
	restored, err := Restore(before)
	if err != nil {
		t.Fatal(err)
	}
	if after := mustSnapshot(t, restored); !bytes.Equal(before, after) {
		t.Fatal("restart changed frozen questions, samples or resources")
	}
	return restored
}

func TestStyleTimingAndRealOpportunityCosts(t *testing.T) {
	t.Run("quick reserves a frozen second report", func(t *testing.T) {
		g, a, b := styleGame(t, "quick", nil)
		styleChoice(t, g, a, "reserve", false)
		if len(g.find(a).signals) != 1 || !view(t, g, a).Self.SpecialtyState.Reserved {
			t.Fatal("reservation delivered both reports")
		}
		plan := g.find(a).specialtyState.plan(1)
		expected := g.sampleReport(plan, plan.Topics[1], 12)
		g = assertStyleRestore(t, g)
		styleNextRound(t, g, a, b)
		styleChoice(t, g, a, "take", false)
		h := g.find(a).signals[1].Data
		for _, key := range []string{"type", "topic", "draws", "band", "sampling"} {
			if h[key] != expected[key] {
				t.Fatal("reserved report rerolled", key)
			}
		}
		styleNextRound(t, g, a, b)
		if len(view(t, g, a).Self.SpecialtyState.Actions) != 0 || len(g.find(a).signals) != 2 {
			t.Fatal("quick gained late reports")
		}
		assertStyleRestore(t, g)
	})
	t.Run("survey receives three reports without action", func(t *testing.T) {
		g, a, b := styleGame(t, "survey", nil)
		for round := 1; round <= Rounds; round++ {
			if len(g.find(a).signals) != min(round, 3) || len(view(t, g, a).Self.SpecialtyState.Actions) != 0 {
				t.Fatal("incorrect survey cadence")
			}
			g = assertStyleRestore(t, g)
			styleNextRound(t, g, a, b)
		}
	})
	t.Run("research commits instead of immediate information", func(t *testing.T) {
		g, a, b := styleGame(t, "research", nil)
		styleChoice(t, g, a, "start", true)
		topic := g.find(a).specialtyState.Research.Topic
		styleNextRound(t, g, a, b)
		styleChoice(t, g, a, "commit", false)
		if len(g.find(a).signals) != 1 {
			t.Fatal("commit also granted an immediate report")
		}
		g = assertStyleRestore(t, g)
		styleNextRound(t, g, a, b)
		h := g.find(a).signals[1].Data
		if len(g.find(a).signals) != 2 || h["draws"] != 12 || h["topic"] != topic || !view(t, g, a).Self.SpecialtyState.Research.Delivered {
			t.Fatal("committed research not delivered on locked topic")
		}
		if err := g.SpecialtyAction(a, 3, "take", topic, "attempt double reward"); err == nil {
			t.Fatal("committed research allowed another report")
		}
		assertStyleRestore(t, g)
	})
	t.Run("research can abandon commitment", func(t *testing.T) {
		g, a, b := styleGame(t, "research", nil)
		styleChoice(t, g, a, "start", true)
		styleNextRound(t, g, a, b)
		styleChoice(t, g, a, "take", true)
		styleNextRound(t, g, a, b)
		if len(g.find(a).signals) != 2 || view(t, g, a).Self.SpecialtyState.Research != nil {
			t.Fatal("abandoned research still delivered")
		}
		if len(view(t, g, a).Self.SpecialtyState.Actions) != 0 {
			t.Fatal("abandoned research gained a third free report")
		}
		assertStyleRestore(t, g)
	})
	t.Run("planner gives up a report and spends on an ordinary card", func(t *testing.T) {
		g, a, b := styleGame(t, "planner", []string{"item_count", "focused_appraisal"})
		styleChoice(t, g, a, "bank", false)
		if len(g.find(a).signals) != 0 || len(g.find(a).specialtyState.Preparation) != 1 {
			t.Fatal("bank granted information too")
		}
		styleNextRound(t, g, a, b)
		styleChoice(t, g, a, "boost", true)
		g = assertStyleRestore(t, g)
		before := mustSnapshot(t, g)
		if err := g.UseCard(a, 2, "focused_appraisal", g.find(a).inspection[0].ID, "attempt to subsidize appraisal"); err == nil {
			t.Fatal("boost enhanced exact appraisal")
		}
		if !bytes.Equal(before, mustSnapshot(t, g)) {
			t.Fatal("rejected appraisal spent preparation or stock")
		}
		if err := g.UseCard(a, 2, "item_count", "", "spend preparation on ordinary scouting"); err != nil {
			t.Fatal(err)
		}
		p := g.find(a)
		report := p.hints[0].Data["preparation_report"].(map[string]any)
		if report["draws"] != 12 || len(p.used) != 1 || p.cardCost != g.cardConfig.Cards[findCardIndex(g.cardConfig, "item_count")].Price || p.specialtyState.ArmedRound != 0 || len(p.specialtyState.Preparation) != 0 {
			t.Fatal("boost changed normal price, allowance or resource consumption")
		}
		assertStyleRestore(t, g)
	})
	t.Run("unused boost expires", func(t *testing.T) {
		g, a, b := styleGame(t, "planner", nil)
		styleChoice(t, g, a, "bank", false)
		styleNextRound(t, g, a, b)
		styleChoice(t, g, a, "boost", true)
		styleNextRound(t, g, a, b)
		if view(t, g, a).Self.SpecialtyState.BoostArmed || len(g.find(a).specialtyState.Preparation) != 0 {
			t.Fatal("unused boost persisted or refunded")
		}
		assertStyleRestore(t, g)
	})
	t.Run("adaptive chooses before seeing an answer", func(t *testing.T) {
		g, a, b := styleGame(t, "adaptive", nil)
		if len(view(t, g, a).Self.SpecialtyState.Actions) != 0 {
			t.Fatal("adaptive acts in round one")
		}
		styleNextRound(t, g, a, b)
		before := mustSnapshot(t, g)
		if err := g.SpecialtyAction(a, 2, "choose", "not-offered", "try another topic"); err == nil {
			t.Fatal("unoffered topic accepted")
		}
		if !bytes.Equal(before, mustSnapshot(t, g)) {
			t.Fatal("rejected choice rerolled questions")
		}
		styleChoice(t, g, a, "choose", true)
		if len(g.find(a).signals) != 1 {
			t.Fatal("unchosen answers revealed")
		}
		assertStyleRestore(t, g)
	})
}

func findCardIndex(cfg cards.Config, id string) int {
	for i, c := range cfg.Cards {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func TestStylePrivacyAtomicityAndFixedSampling(t *testing.T) {
	for _, style := range specialtyStyles {
		t.Run(style, func(t *testing.T) {
			g, a, b := styleGame(t, style, nil)
			for round := 1; round <= 3; round++ {
				before := mustSnapshot(t, g)
				public, _ := g.Observe("")
				other, _ := g.Observe(b)
				self := view(t, g, a).Self
				encoded, _ := json.Marshal(self.SpecialtyState)
				for _, key := range []string{`"samples"`, `"plans"`, `"band"`, `"item_id"`, `"price"`} {
					if bytes.Contains(encoded, []byte(key)) {
						t.Fatal("pending question exposed answer or hidden samples", key)
					}
				}
				if err := g.SpecialtyAction(a, round, "bank", "", ""); err == nil || !bytes.Equal(before, mustSnapshot(t, g)) {
					t.Fatal("missing reason accepted or mutated state")
				}
				status := self.SpecialtyState
				if len(status.Actions) > 0 {
					action := status.Actions[0]
					choose := style != "quick" && action != "commit" && action != "bank"
					styleChoice(t, g, a, action, choose)
					afterPublic, _ := g.Observe("")
					afterOther, _ := g.Observe(b)
					if !bytes.Equal(public, afterPublic) || !bytes.Equal(other, afterOther) {
						t.Fatal("private action changed public or another player's view")
					}
					before = mustSnapshot(t, g)
					if err := g.SpecialtyAction(a, round, action, "", "attempt to repeat choice"); err == nil || !bytes.Equal(before, mustSnapshot(t, g)) {
						t.Fatal("repeated choice accepted or changed samples")
					}
				}
				v := view(t, g, a)
				for _, h := range v.Self.Intelligence {
					raw, _ := json.Marshal(h)
					for _, key := range []string{`"item_id"`, `"items"`, `"price"`, `"count"`, `"top_left"`, `"category"`, `"template_id"`} {
						if bytes.Contains(raw, []byte(key)) {
							t.Fatal("anonymous report contains joinable facts", key)
						}
					}
				}
				if len(v.Self.ItemReveals) != 0 {
					t.Fatal("anonymous report was automatically located")
				}
				before = mustSnapshot(t, g)
				g.Observe(a)
				g.ObserveObserver()
				g.Observe("")
				if !bytes.Equal(before, mustSnapshot(t, g)) {
					t.Fatal("read rerolled private sample")
				}
				g = assertStyleRestore(t, g)
				styleNextRound(t, g, a, b)
			}
		})
	}
}

func TestStyleChoiceDeadlineAndWithdrawal(t *testing.T) {
	for _, mode := range []string{"bid", "withdraw", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			g, a, _ := styleGame(t, "quick", nil)
			switch mode {
			case "bid":
				g.Submit(a, 1, 100, "already committed offer")
			case "withdraw":
				g.Withdraw(a, 1, "leave auction")
			case "deadline":
				g.roundDeadline = time.Now().Add(-time.Second)
			}
			before := mustSnapshot(t, g)
			if err := g.SpecialtyAction(a, 1, "take", "", "late private action"); err == nil || !bytes.Equal(before, mustSnapshot(t, g)) {
				t.Fatal("late choice changed state")
			}
		})
	}
}

func TestWithdrawnPlannerDoesNotBlockLaterRoundPersistence(t *testing.T) {
	g, err := New(3)
	if err != nil {
		t.Fatal(err)
	}
	_, a, _ := g.Join("A", nil)
	_, b, _ := g.Join("B", nil)
	_, c, _ := g.Join("C", nil)
	if err := g.Configure(a, "planner", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	styleChoice(t, g, a, "bank", false)
	for _, token := range []string{a, b, c} {
		if err := g.Submit(token, 1, 100, "keep competing from current estimate"); err != nil {
			t.Fatal(err)
		}
	}
	styleChoice(t, g, a, "boost", true)
	if err := g.Withdraw(a, 2, "leave instead of spending further"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{b, c} {
		if err := g.Submit(token, 2, 100, "continue despite another player's withdrawal"); err != nil {
			t.Fatal(err)
		}
	}
	if g.round != 3 || len(g.find(a).specialtyState.Plans) != 2 {
		t.Fatal("withdrawn player gained a new investigation")
	}
	assertStyleRestore(t, g)
}

func TestReleasedSnapshotKeepsFrozenSpecialtiesAndDeadline(t *testing.T) {
	for _, schema := range []int{10, 11} {
		g, _ := NewWithConfig(2, releasedSpecialtyConfig(t), cards.Default())
		g.Join("A", nil)
		g.Join("B", nil)
		g.Start()
		before := view(t, g, g.players[0].token)
		var snapshot map[string]any
		json.Unmarshal(mustSnapshot(t, g), &snapshot)
		snapshot["schema_version"] = schema
		if schema == 10 {
			delete(snapshot, "round_deadline")
		}
		data, _ := json.Marshal(snapshot)
		now := time.Now().UTC().Add(time.Minute)
		data, err := UpgradeSnapshot(data, now)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := Restore(data)
		if err != nil {
			t.Fatal(err)
		}
		after := view(t, restored, g.players[0].token)
		if !reflect.DeepEqual(before.Rules.Specialties, after.Rules.Specialties) || !reflect.DeepEqual(before.Self.Intelligence, after.Self.Intelligence) || after.Self.SpecialtyState != nil {
			t.Fatal("migration replaced frozen rules or regenerated intelligence")
		}
		expected := before.RoundDeadline
		if schema == 10 {
			expected = now.Add(RoundTimeout)
		}
		if !after.RoundDeadline.Equal(expected) {
			t.Fatal("migration reset an existing deadline")
		}
	}
}

func TestStyleSnapshotRejectsAlteredResourcesAndReports(t *testing.T) {
	g, a, _ := styleGame(t, "quick", nil)
	styleChoice(t, g, a, "reserve", false)
	for _, mutate := range []func(*gameSnapshot){
		func(s *gameSnapshot) { s.Players[0].SpecialtyState.Reserved = false },
		func(s *gameSnapshot) { s.Players[0].SpecialtyState.Plans[0].Samples[0][0] = "missing-item" },
		func(s *gameSnapshot) { s.Players[0].Signals[0].Data["price"] = 123 },
		func(s *gameSnapshot) { s.Players[0].Signals[0].Data["band"] = "made-up" },
		func(s *gameSnapshot) { s.Players[0].SpecialtyState.Choices[0].Topic = "quality_mix" },
	} {
		s, err := decodeSnapshot(mustSnapshot(t, g))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&s)
		raw, _ := json.Marshal(s)
		if _, err := Restore(raw); err == nil {
			t.Fatal("inconsistent specialty snapshot accepted")
		}
	}
}
