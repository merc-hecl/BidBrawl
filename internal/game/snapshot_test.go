package game

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSnapshotPreservesPrivateStateAndSettlement(t *testing.T) {
	g, err := New(2)
	if err != nil {
		t.Fatal(err)
	}
	tokens := []string{}
	for _, name := range []string{"Atlas", "Echo"} {
		_, token, err := g.Join(name, []string{"largest_price", "random_shape"})
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	if err := g.UseCard(tokens[0], 1, "random_shape", "protocol test"); err != nil {
		t.Fatal(err)
	}
	if err := g.Submit(tokens[0], 1, 777777, "protocol test"); err != nil {
		t.Fatal(err)
	}
	compareRestore := func() *Game {
		t.Helper()
		snapshot, err := g.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		restored, err := Restore(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range append([]string{""}, tokens...) {
			before, _ := g.Observe(token)
			after, _ := restored.Observe(token)
			if !bytes.Equal(before, after) {
				t.Fatal("snapshot changed an observation")
			}
		}
		return restored
	}
	restored := compareRestore()
	if err := restored.Submit(tokens[0], 1, 1, "protocol test"); err == nil {
		t.Fatal("restoration forgot a sealed bid")
	}
	if err := restored.Submit(tokens[1], 1, 600000, "protocol test"); err != nil {
		t.Fatal(err)
	}
	before, _ := g.Observe("")
	var original View
	_ = json.Unmarshal(before, &original)
	if original.Round != 1 || len(original.History) != 0 {
		t.Fatal("restored game mutated original state")
	}
	g = restored
	for round := 2; round <= Rounds; round++ {
		for _, token := range tokens {
			if err := g.Submit(token, round, 100, "protocol test"); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Restoring a tied final result must retain the drawn winner.
	for range 3 {
		g = compareRestore()
	}
}

func TestRestoreRejectsInconsistentSnapshots(t *testing.T) {
	g, _ := New(2)
	tokens := []string{}
	for _, name := range []string{"A", "B"} {
		_, token, _ := g.Join(name, []string{"item_count"})
		tokens = append(tokens, token)
	}
	_ = g.Start()
	_ = g.UseCard(tokens[0], 1, "item_count", "protocol test")
	data, _ := g.Snapshot()
	for name, corrupt := range map[string]func(*gameSnapshot){
		"schema":   func(s *gameSnapshot) { s.SchemaVersion = 99 },
		"phase":    func(s *gameSnapshot) { s.Phase = "unknown" },
		"price":    func(s *gameSnapshot) { s.Items[0].Price = -1 },
		"identity": func(s *gameSnapshot) { s.Players[1].Token = s.Players[0].Token },
		"bid":      func(s *gameSnapshot) { bid := int64(-1); s.Players[0].Bid = &bid },
		"card":     func(s *gameSnapshot) { s.Players[0].Used[0] = "missing" },
		"history":  func(s *gameSnapshot) { s.History = []RoundResult{{Round: 1}} },
		"version":  func(s *gameSnapshot) { s.Version++ },
		"geometry": func(s *gameSnapshot) {
			s.Items[0].TopLeft.X = -1
		},
	} {
		t.Run(name, func(t *testing.T) {
			var snapshot gameSnapshot
			_ = json.Unmarshal(data, &snapshot)
			corrupt(&snapshot)
			bad, _ := json.Marshal(snapshot)
			if _, err := Restore(bad); err == nil {
				t.Fatal("inconsistent snapshot accepted")
			}
		})
	}
}
