package game

import (
	"encoding/json"
	"os"
	"testing"
)

func releasedSpecialtyConfig(t *testing.T) Config {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Specialties = nil
	raw, err := os.ReadFile("testdata/released-specialties.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cfg.Specialties); err != nil {
		t.Fatal(err)
	}
	return cfg
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
