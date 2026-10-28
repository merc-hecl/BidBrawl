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
