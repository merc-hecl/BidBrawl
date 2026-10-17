package game

import "maps"

// EconomyState is used only by the account service while committing a room.
// It is never included in a public room view.
type EconomyState struct {
	Used []string
}

func (g *Game) Economy() map[string]EconomyState {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]EconomyState{}
	for _, p := range g.players {
		out[p.id] = EconomyState{Used: append([]string{}, p.used...)}
	}
	return out
}
func (g *Game) SetCardPrices(prices map[string]map[string]int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range g.players {
		p.cardPrices = maps.Clone(prices[p.id])
	}
}

func (g *Game) UsedCardPrice(playerID, card string) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range g.players {
		if p.id != playerID || !contains(p.used, card) {
			continue
		}
		if price, ok := p.cardPrices[card]; ok {
			return price
		}
		for _, d := range g.cardConfig.Cards {
			if d.ID == card {
				return d.Price
			}
		}
	}
	return 0
}
