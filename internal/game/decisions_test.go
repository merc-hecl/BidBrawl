package game

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestInstantSettlementThresholdsAndPersistence(t *testing.T) {
	for _, sample := range []struct {
		round        int
		high, second int64
		instant      bool
	}{
		{1, 199, 100, false}, {1, 200, 100, true}, {2, 169, 100, false}, {2, 170, 100, true},
		{3, 149, 100, false}, {3, 150, 100, true}, {4, 1000000, 1, false}, {5, 100, 100, false},
		{1, 1, 0, true}, {1, 0, 0, false},
	} {
		g, tokens := fixture(t, 2)
		for round := 1; round < sample.round; round++ {
			for _, token := range tokens {
				if err := g.Submit(token, round, 100, "balanced estimate"); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := g.Submit(tokens[0], sample.round, sample.high, "price and risk estimate"); err != nil {
			t.Fatal(err)
		}
		if view(t, g, "").Settlement != nil {
			t.Fatal("settled before everyone submitted")
		}
		if err := g.Submit(tokens[1], sample.round, sample.second, "independent estimate"); err != nil {
			t.Fatal(err)
		}
		v := view(t, g, "")
		settled := sample.instant || sample.round == Rounds
		if (v.Settlement != nil) != settled {
			t.Fatalf("round %d bids %d/%d: phase %s", sample.round, sample.high, sample.second, v.Phase)
		}
		if settled {
			method := "final"
			if sample.instant {
				method = "instant"
			}
			if v.Round != sample.round || v.Settlement.Round != sample.round || v.Settlement.Method != method || v.Settlement.Paid != sample.high || len(v.History) != sample.round {
				t.Fatal("wrong settlement stage")
			}
		}
		snapshot, _ := g.Snapshot()
		restored, err := Restore(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := g.ObserveObserver()
		after, _ := restored.ObserveObserver()
		if !bytes.Equal(before, after) {
			t.Fatal("restart changed decisions or outcome")
		}
	}
}

func TestDecisionReasonsPrivateAndRequired(t *testing.T) {
	g, tokens := fixture(t, 2)
	before, _ := g.Observe("")
	for _, reason := range []string{"", " \n", strings.Repeat("字", 2001)} {
		if g.UseCard(tokens[0], 1, "item_count", reason) == nil || g.Submit(tokens[0], 1, 100, reason) == nil {
			t.Fatal("invalid reason accepted")
		}
	}
	if err := g.UseCard(tokens[0], 1, "item_count", "先清点，缩小估值区间。"); err != nil {
		t.Fatal(err)
	}
	after, _ := g.Observe("")
	if !bytes.Equal(before, after) {
		t.Fatal("public view exposed private action")
	}
	if err := g.Submit(tokens[0], 1, 100, "I am allowing a margin for uncertainty."); err != nil {
		t.Fatal(err)
	}
	self := view(t, g, tokens[0])
	other := view(t, g, tokens[1])
	public := view(t, g, "")
	if len(self.Self.Decisions) != 2 || len(other.Self.Decisions) != 0 || len(other.Decisions) != 0 || len(public.Decisions) != 0 {
		t.Fatal("decision isolation failed")
	}
	data, _ := g.ObserveObserver()
	var observer View
	_ = json.Unmarshal(data, &observer)
	if len(observer.Decisions) != 2 || observer.Self != nil {
		t.Fatal("observer did not receive only decision records")
	}
}
