package game

import (
	"bytes"
	"testing"
	"testing/synctest"
	"time"
)

func TestRoundDeadlineSkipsOnlyMissingBidAndPlayerCanReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g, tokens := fixture(t, 3)
		deadline := view(t, g, "").RoundDeadline
		if deadline.Sub(time.Now()) != RoundTimeout {
			t.Fatal("opening round does not have five minutes")
		}
		for _, token := range tokens[:2] {
			if err := g.Submit(token, 1, 100, "same estimate"); err != nil {
				t.Fatal(err)
			}
		}
		if err := g.UseCard(tokens[2], 1, "item_count", "", "count before bidding"); err != nil {
			t.Fatal(err)
		}
		before, _ := g.Snapshot()
		if err := g.ExpireRound(deadline.Add(-time.Nanosecond)); err != nil {
			t.Fatal(err)
		}
		after, _ := g.Snapshot()
		if !bytes.Equal(before, after) {
			t.Fatal("expired before deadline")
		}
		time.Sleep(RoundTimeout)
		for _, err := range []error{
			g.Submit(tokens[2], 1, 90, "late bid"),
			g.UseCard(tokens[2], 1, "largest_quality", "", "late card"),
			g.Withdraw(tokens[2], 1, "late withdrawal"),
		} {
			if err == nil {
				t.Fatal("accepted action at deadline")
			}
		}
		if err := g.ExpireRound(time.Now()); err != nil {
			t.Fatal(err)
		}
		v := view(t, g, tokens[2])
		if v.Round != 2 || v.RoundDeadline.Sub(time.Now()) != RoundTimeout || len(v.History) != 1 || v.History[0].Bids[0].Amount != 100 || v.History[0].Bids[2].Amount != 0 {
			t.Fatal("timeout did not preserve bids and advance with a fresh deadline")
		}
		if v.Self.Withdrawn || v.Players[2].Withdrawn || v.Players[2].WithdrawRound != 0 || len(v.Self.Used) != 1 || v.Self.Bid != nil {
			t.Fatal("timeout changed withdrawal, card use or next-round availability")
		}
		if got := v.Self.Decisions[len(v.Self.Decisions)-1]; got.Action != "timeout" || got.Reason == "" {
			t.Fatal("missing private timeout record")
		}
		public, _ := g.Observe("")
		other, _ := g.Observe(tokens[0])
		if bytes.Contains(public, []byte("本轮报价超时")) || bytes.Contains(other, []byte("本轮报价超时")) {
			t.Fatal("timeout reason leaked")
		}
		snapshot, _ := g.Snapshot()
		g, err := Restore(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if err := g.UseCard(tokens[2], 2, "largest_quality", "", "return after missing one round"); err != nil {
			t.Fatal(err)
		}
		if err := g.Submit(tokens[2], 2, 90, "new estimate"); err != nil {
			t.Fatal(err)
		}
		for _, token := range tokens[:2] {
			if err := g.Submit(token, 2, 100, "same estimate"); err != nil {
				t.Fatal(err)
			}
		}
		if v := view(t, g, ""); v.Round != 3 || v.Players[2].Withdrawn || v.History[1].Bids[2].Amount != 90 {
			t.Fatal("timed-out player could not rejoin next round")
		}
	})
}

func TestTimeoutSettlementAndFinalRound(t *testing.T) {
	for _, scenario := range []string{"all missing", "one positive", "final tie"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				g, tokens := fixture(t, 3)
				round := 1
				if scenario == "final tie" {
					for ; round < Rounds; round++ {
						for _, token := range tokens {
							if err := g.Submit(token, round, 100, "equal offers"); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				count := 0
				if scenario == "one positive" {
					count = 1
				}
				if scenario == "final tie" {
					count = 2
				}
				for _, token := range tokens[:count] {
					if err := g.Submit(token, round, 100, "positive estimate"); err != nil {
						t.Fatal(err)
					}
				}
				time.Sleep(RoundTimeout)
				if err := g.ExpireRound(time.Now()); err != nil {
					t.Fatal(err)
				}
				v := view(t, g, "")
				if v.Phase != "settled" || !v.RoundDeadline.IsZero() {
					t.Fatal("timeout failed to settle")
				}
				if scenario == "all missing" && (v.Settlement.Method != "unsold" || v.Settlement.WinnerID != "" || v.Settlement.Paid != 0) {
					t.Fatal("all missing should be unsold")
				}
				if count > 0 && (v.Settlement.WinnerID == "p3" || v.Settlement.Paid != 100) {
					t.Fatal("zero bid won or incorrect price charged")
				}
				for _, p := range v.Players {
					if p.Withdrawn {
						t.Fatal("system withdrew a player")
					}
				}
				snapshot, _ := g.Snapshot()
				if _, err := Restore(snapshot); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
