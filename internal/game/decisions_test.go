package game

import (
	"bytes"
	"math"
	"testing"
)

func TestEarlySettlementWithLargeAssetBalances(t *testing.T) {
	for _, test := range []struct {
		high, second int64
		want         bool
	}{
		{math.MaxInt64, math.MaxInt64 - 1, false},
		{math.MaxInt64, math.MaxInt64 / 2, true},
		{math.MaxInt64 - 2, math.MaxInt64 / 2, false},
	} {
		got := EarlySettlement(RoundResult{Round: 1, Bids: []Bid{{"p1", test.high}, {"p2", test.second}}})
		if got != test.want {
			t.Fatalf("large asset threshold: %d/%d got %v", test.high, test.second, got)
		}
	}
}

func TestInstantSettlementThresholdsAndPersistence(t *testing.T) {
	for _, sample := range []struct {
		round        int
		high, second int64
		instant      bool
	}{
		{1, 199, 100, false}, {1, 200, 100, true}, {2, 159, 100, false}, {2, 160, 100, true},
		{3, 129, 100, false}, {3, 130, 100, true}, {4, 109, 100, false}, {4, 110, 100, true}, {5, 100, 100, false},
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
		submit := func(token string, amount int64) error {
			if amount == 0 {
				return g.Withdraw(token, sample.round, "exit based on risk")
			}
			return g.Submit(token, sample.round, amount, "price and risk estimate")
		}
		if err := submit(tokens[0], sample.high); err != nil {
			t.Fatal(err)
		}
		if view(t, g, "").Settlement != nil {
			t.Fatal("settled before everyone submitted")
		}
		if err := submit(tokens[1], sample.second); err != nil {
			t.Fatal(err)
		}
		v := view(t, g, "")
		settled := sample.instant || sample.round == Rounds || sample.high == 0 && sample.second == 0
		if (v.Settlement != nil) != settled {
			t.Fatalf("round %d bids %d/%d: phase %s", sample.round, sample.high, sample.second, v.Phase)
		}
		if settled {
			method := "final"
			if sample.instant {
				method = "instant"
			}
			if sample.high == 0 && sample.second == 0 {
				method = "unsold"
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
