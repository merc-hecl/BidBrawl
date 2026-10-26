package server

import (
	"bidbrawl/internal/game"
	"errors"
	"math"
	"net/http"
	"time"
)

type storedItem struct {
	ID     string    `json:"id"`
	RoomID string    `json:"room_id"`
	Item   game.Item `json:"item"`
}

func cloneAccount(a *account) account {
	b := *a
	b.Warehouse = append([]storedItem{}, a.Warehouse...)
	b.CardStock = map[string][]int64{}
	for id, lots := range a.CardStock {
		b.CardStock[id] = append([]int64{}, lots...)
	}
	return b
}
func accountWealth(a *account) (items, cards int64) {
	for _, i := range a.Warehouse {
		items += i.Item.Price
	}
	for _, lots := range a.CardStock {
		for _, cost := range lots {
			cards += cost
		}
	}
	return
}
func (s *Server) inAuction(a *account) bool {
	if s.recoveryLocked(a.ID) {
		return true
	}
	for _, rm := range s.rooms {
		if rm.members[a.ID] == "" {
			continue
		}
		v, _ := publicView(rm.game)
		if v.Phase == "bidding" {
			return true
		}
	}
	return false
}
func (s *Server) checkDeck(a *account, deck []string) error {
	seen := map[string]bool{}
	if len(deck) > 5 {
		return errors.New("at most 5 different cards")
	}
	for _, id := range deck {
		if seen[id] || len(a.CardStock[id]) == 0 {
			return errors.New("card not owned or duplicate: " + id)
		}
		seen[id] = true
	}
	return nil
}
func (s *Server) inventory(w http.ResponseWriter, r *http.Request) {
	a := s.requireActor(w, r)
	if a == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[token(r)] != a {
		fail(w, 401, errors.New("login session required"))
		return
	}
	s.inventoryReply(w, a)
}
func (s *Server) inventoryReply(w http.ResponseWriter, a *account) {
	items, cards := accountWealth(a)
	reply(w, 200, map[string]any{"cash": a.Assets, "items_value": items, "cards_value": cards, "total_assets": a.Assets + items + cards, "items": a.Warehouse, "cards": a.CardStock, "in_auction": s.inAuction(a), "recovery_blocked": s.recoveryLocked(a.ID), "relief_claimed_at": a.ReliefAt})
}
func (s *Server) buy(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Card     string `json:"card"`
		Quantity int    `json:"quantity"`
	}
	if !decode(w, r, &v) {
		return
	}
	if v.Quantity < 1 || v.Quantity > 99 {
		fail(w, 400, errors.New("quantity must be 1–99"))
		return
	}
	s.economyAction(w, r, func(a *account) error {
		var price int64 = -1
		for _, d := range s.cardConfig.Cards {
			if d.ID == v.Card {
				price = d.Price
			}
		}
		if price < 0 {
			return errors.New("unknown card")
		}
		cost := price * int64(v.Quantity)
		if a.Assets < cost {
			return errors.New("insufficient cash")
		}
		if len(a.CardStock[v.Card])+v.Quantity > 999 {
			return errors.New("card stock limit is 999")
		}
		a.Assets -= cost
		for range v.Quantity {
			a.CardStock[v.Card] = append(a.CardStock[v.Card], price)
		}
		return nil
	})
}
func (s *Server) sell(w http.ResponseWriter, r *http.Request) {
	var v struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}
	if !decode(w, r, &v) {
		return
	}
	if v.All && len(v.IDs) > 0 || !v.All && len(v.IDs) == 0 {
		fail(w, 400, errors.New("choose item ids or all"))
		return
	}
	s.economyAction(w, r, func(a *account) error {
		wanted := map[string]bool{}
		for _, id := range v.IDs {
			if wanted[id] {
				return errors.New("duplicate item id")
			}
			wanted[id] = true
		}
		remaining := []storedItem{}
		var value int64
		for _, i := range a.Warehouse {
			if v.All || wanted[i.ID] {
				value += i.Item.Price
				delete(wanted, i.ID)
			} else {
				remaining = append(remaining, i)
			}
		}
		if len(wanted) > 0 {
			return errors.New("item not owned")
		}
		if value > math.MaxInt64-a.Assets {
			return errors.New("cash overflow")
		}
		a.Assets += value
		a.Warehouse = remaining
		return nil
	})
}
func (s *Server) relief(w http.ResponseWriter, r *http.Request) {
	s.economyAction(w, r, func(a *account) error {
		items, cards := accountWealth(a)
		today := time.Now().UTC().Add(8 * time.Hour).Format("2006-01-02")
		if a.Assets+items+cards >= 10000 {
			return errors.New("relief requires total wealth below 10000")
		}
		if a.ReliefAt == today {
			return errors.New("relief already claimed today")
		}
		a.Assets += 50000
		a.ReliefAt = today
		return nil
	})
}
func (s *Server) economyAction(w http.ResponseWriter, r *http.Request, apply func(*account) error) {
	a := s.requireActor(w, r)
	if a == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[token(r)] != a {
		fail(w, 401, errors.New("login session required"))
		return
	}
	if s.inAuction(a) {
		fail(w, 409, errors.New("cash and inventory are locked while bidding"))
		return
	}
	next := cloneAccount(a)
	if err := apply(&next); err != nil {
		fail(w, 409, err)
		return
	}
	if !s.saveIdentity(w, &next, s.credentialHashes[a.ID]) {
		return
	}
	*a = next
	s.inventoryReply(w, a)
}
func (s *Server) loadout(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Cards     []string `json:"cards"`
		Specialty string   `json:"specialty"`
		Category  string   `json:"category"`
	}
	if !decode(w, r, &v) {
		return
	}
	if s.playerAction(w, r, func(g *game.Game, t string) error {
		a := s.sessions[token(r)]
		if err := s.checkDeck(a, v.Cards); err != nil {
			return err
		}
		return g.Configure(t, v.Specialty, v.Category, v.Cards)
	}) {
		s.state(w, r)
	}
}
