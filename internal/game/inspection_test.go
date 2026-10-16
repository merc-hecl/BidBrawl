package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"bidbrawl/internal/cards"
)

func TestInitialInspectionIsEqualSizedPrivateAndDurable(t *testing.T) {
	for _, cfg := range []Config{DefaultConfig(), geometryConfig(Dimensions{1, 1}, ItemLimits{1, 1})} {
		g, err := NewWithConfig(7, cfg, cards.Default())
		if err != nil {
			t.Fatal(err)
		}
		var tokens []string
		for i := range 7 {
			_, token, err := g.Join(fmt.Sprintf("Player %d", i), nil)
			if err != nil {
				t.Fatal(err)
			}
			tokens = append(tokens, token)
			if len(view(t, g, token).Self.InitialInspection) != 0 {
				t.Fatal("inspection revealed before start")
			}
		}
		if err := g.Start(); err != nil {
			t.Fatal(err)
		}
		originals := map[string]InspectionItem{}
		for _, item := range g.items {
			originals[item.ID] = InspectionItem{item.ID, item.Quality, item.TopLeft, item.BottomRight}
		}
		for _, token := range tokens {
			v := view(t, g, token)
			if len(v.Self.InitialInspection) != min(InitialInspectionCount, len(g.items)) || v.Rules.InitialInspectionCount != InitialInspectionCount {
				t.Fatal("players did not receive the published sample size")
			}
			if len(v.InitialInspections) != 0 {
				t.Fatal("player received others' inspections")
			}
			seen := map[string]bool{}
			for _, item := range v.Self.InitialInspection {
				if item != originals[item.ID] || seen[item.ID] {
					t.Fatal("invalid or repeated sampled item")
				}
				seen[item.ID] = true
			}
		}
		public, _ := g.Observe("")
		if bytes.Contains(public, []byte(`"initial_inspection"`)) || bytes.Contains(public, []byte(`"initial_inspections"`)) {
			t.Fatal("inspection leaked publicly")
		}
		observed, _ := g.ObserveObserver()
		var observer View
		if err := json.Unmarshal(observed, &observer); err != nil {
			t.Fatal(err)
		}
		if observer.Self != nil || len(observer.InitialInspections) != 7 {
			t.Fatal("invalid human observer inspection projection")
		}
		encoded, _ := json.Marshal(observer.InitialInspections)
		if bytes.Contains(encoded, []byte(`"price"`)) {
			t.Fatal("inspection contains prices")
		}
		snapshot, _ := g.Snapshot()
		restored, err := Restore(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range tokens {
			before, _ := g.Observe(token)
			after, _ := restored.Observe(token)
			if !bytes.Equal(before, after) {
				t.Fatal("restart changed private inspection")
			}
		}
	}
}

func TestObserverItemRevealsPreserveOnlyLearnedFields(t *testing.T) {
	for _, fields := range [][]string{{"shape"}, {"quality"}, {"price"}, {"shape", "quality", "price"}} {
		t.Run(fmt.Sprint(fields), func(t *testing.T) {
			cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{}, Cards: []cards.Definition{
				{ID: "scan", Name: "Scan", Description: "Test reveal", Effect: cards.Effect{Type: "reveal", Selector: "largest", Fields: fields}},
			}}
			g, err := NewWithCards(2, cfg)
			if err != nil {
				t.Fatal(err)
			}
			id, token, _ := g.Join("A", []string{"scan"})
			other, otherToken, _ := g.Join("B", nil)
			if err := g.Start(); err != nil {
				t.Fatal(err)
			}
			public, _ := g.Observe("")
			if err := g.UseCard(token, 1, "scan", "inspect the largest item"); err != nil {
				t.Fatal(err)
			}
			data, err := g.ObserveObserver()
			if err != nil {
				t.Fatal(err)
			}
			var observed View
			if err := json.Unmarshal(data, &observed); err != nil {
				t.Fatal(err)
			}
			if len(observed.ItemReveals[id]) != 1 || len(observed.ItemReveals[other]) != 0 {
				t.Fatal("wrong player projection")
			}
			reveal := observed.ItemReveals[id][0]
			hint := view(t, g, token).Self.Hints[0].Data
			if reveal.ItemID != hint["item_id"] || reveal.Round != 1 ||
				(reveal.Quality != "") != contains(fields, "quality") ||
				(reveal.Price != nil) != contains(fields, "price") ||
				reveal.TopLeft == nil || reveal.BottomRight == nil {
				t.Fatal("projection revealed an unlearned attribute")
			}
			if reveal.Price != nil && float64(*reveal.Price) != hint["price"] {
				t.Fatal("wrong learned price")
			}
			for _, item := range g.items {
				if item.ID == reveal.ItemID && (*reveal.TopLeft != item.TopLeft || *reveal.BottomRight != item.BottomRight) {
					t.Fatal("wrong target location")
				}
			}
			if !reflect.DeepEqual(view(t, g, token).Self.ItemReveals, observed.ItemReveals[id]) {
				t.Fatal("player and human observer have different knowledge")
			}
			after, _ := g.Observe("")
			if !bytes.Equal(public, after) || len(view(t, g, otherToken).ItemReveals) != 0 {
				t.Fatal("private vision leaked")
			}
			snapshot, _ := g.Snapshot()
			restored, err := Restore(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			restoredData, _ := restored.ObserveObserver()
			var restoredView View
			_ = json.Unmarshal(restoredData, &restoredView)
			if !reflect.DeepEqual(observed.ItemReveals, restoredView.ItemReveals) {
				t.Fatal("restart changed learned fields")
			}
		})
	}
}
