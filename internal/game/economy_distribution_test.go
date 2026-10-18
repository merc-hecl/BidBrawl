package game

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// These are generated-lot economics checks, not bidding or Agent E2E evidence.
func TestDefaultLotValueDistribution(t *testing.T) {
	for _, warehouse := range Warehouses {
		t.Run(warehouse.ID, func(t *testing.T) {
			g, _ := New(2)
			g.warehouse = warehouse.ID
			rng := rand.New(rand.NewPCG(20261018, 1))
			values := []int64{}
			var total, legendary int64
			overBudget := 0
			for range 400 {
				var value int64
				for _, i := range g.generateCatalogWith(rng.IntN) {
					if isEasterEgg(i) {
						continue
					}
					value += i.Price
					if i.Quality == "legendary" {
						legendary += i.Price
					}
				}
				values = append(values, value)
				total += value
				if value > g.gameConfig.InitialAssets-g.gameConfig.TicketFee {
					overBudget++
				}
			}
			slices.Sort(values)
			cash := g.gameConfig.InitialAssets - g.gameConfig.TicketFee
			if overBudget > len(values)/10 || values[360] > cash {
				t.Fatal("ordinary lots frequently exceed starting bid cash", overBudget, values[360], cash)
			}
			if values[200] < cash/4 || values[200] > cash*3/4 {
				t.Fatal("median lot and starting cash no longer leave a useful bidding range", values[200], cash)
			}
			if legendary*2 > total {
				t.Fatal("highest ordinary tier dominates the default economy")
			}
			t.Logf("ordinary lot value: p10=%d p50=%d p90=%d p95=%d; above starting bid cash=%d/400; legendary share=%.1f%%", values[40], values[200], values[360], values[380], overBudget, 100*float64(legendary)/float64(total))
		})
	}
}
