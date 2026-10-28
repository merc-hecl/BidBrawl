package game

import (
	"testing"

	"bidbrawl/internal/cards"
)

func geometryConfig(box Dimensions, items ItemLimits) Config {
	cfg := DefaultConfig()
	cfg.Box, cfg.Items = box, items
	return cfg
}

func TestBidsUseEachPlayersAssetsIncludingZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TicketFee = 0
	g, _ := NewWithConfig(2, cfg, cards.Default())
	_, a, _ := g.Join("A", nil)
	_, b, _ := g.Join("B", nil)
	if err := g.StartWithAssets(map[string]int64{"p1": 0, "p2": 3000000}); err != nil {
		t.Fatal(err)
	}
	if view(t, g, a).Self.MaxBid != 0 || view(t, g, b).Self.MaxBid != 3000000 {
		t.Fatal("players share a fixed limit")
	}
	if err := g.Submit(a, 1, 1, "protocol fixture"); err == nil {
		t.Fatal("zero-assets player can overbid")
	}
	if err := g.Withdraw(a, 1, "protocol fixture"); err != nil {
		t.Fatal(err)
	}
	if err := g.Submit(b, 1, 3000001, "protocol fixture"); err == nil {
		t.Fatal("overbid accepted")
	}
	if err := g.Submit(b, 1, 3000000, "protocol fixture"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := g.Snapshot()
	if _, err := Restore(snapshot); err != nil {
		t.Fatal(err)
	}
}
