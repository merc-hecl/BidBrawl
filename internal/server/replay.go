package server

import (
	"bidbrawl/internal/game"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
)

func (s *Server) replay(w http.ResponseWriter, r *http.Request) { s.replayFor(w, r, false) }
func (s *Server) observerReplay(w http.ResponseWriter, r *http.Request) {
	if s.requireObserver(w, r) {
		s.replayFor(w, r, true)
	}
}

// Public replay deliberately ignores player login: it never contains self or skill results.
func (s *Server) replayFor(w http.ResponseWriter, r *http.Request, observer bool) {
	rm := s.getRoom(w, r)
	if rm == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.roomExists(w, rm) {
		return
	}
	b, err := rm.game.Observe("")
	if observer {
		b, err = rm.game.ObserveObserver()
	}
	if err != nil {
		fail(w, 500, err)
		return
	}
	var full map[string]any
	var view game.View
	if err = json.Unmarshal(b, &full); err != nil {
		fail(w, 500, err)
		return
	}
	if err = json.Unmarshal(b, &view); err != nil {
		fail(w, 500, err)
		return
	}
	full["room_id"], full["room_name"], full["owner_id"] = rm.id, rm.name, rm.owner
	full["created_at"] = rm.createdAt
	if !rm.archivedAt.IsZero() {
		full["archived_at"] = rm.archivedAt
	}
	if observer {
		full["can_manage"] = s.isRoomManager(r) && rm.archivedAt.IsZero()
	}
	if !rm.settledAt.IsZero() {
		full["settled_at"] = rm.settledAt
	}
	for _, raw := range full["players"].([]any) {
		p := raw.(map[string]any)
		p["agent_id"] = "account:" + rm.seatAccounts[p["id"].(string)]
	}
	count := len(view.History)
	// Frame zero is the opening inspection. Frame N includes only rounds 1..N;
	// the live projection separately includes an ongoing round.
	// The settling round reveals its bids and settlement atomically in one frame.
	frames := make([]map[string]any, 0, count+1)
	for cursor := 0; cursor <= count; cursor++ {
		frame := make(map[string]any, len(full))
		for k, v := range full {
			frame[k] = v
		}
		n := cursor
		frame["history"] = view.History[:n]
		hints := []game.Hint{}
		for _, h := range view.PublicHints {
			if h.Round <= n {
				hints = append(hints, h)
			}
		}
		frame["public_hints"] = hints
		multiplier := float64(0)
		if n > 0 {
			multiplier = view.Rules.EarlyMultipliers[n-1]
		}
		frame["round"] = n
		frame["context"] = map[string]any{"warehouse": view.Rules.Warehouse, "round": n, "remaining_rounds": view.Rules.Rounds - n, "ticket": view.Rules.TicketFee, "early_multiplier": multiplier}
		if observer {
			frame["activities"] = activityView(rm, "", n)
			intelligence := map[string][]game.Hint{}
			for p, list := range view.Intelligence {
				intelligence[p] = []game.Hint{}
				for _, h := range list {
					if h.Round <= n {
						intelligence[p] = append(intelligence[p], h)
					}
				}
			}
			frame["intelligence"] = intelligence
			costs := map[string]game.PlayerCosts{}
			for seat, list := range intelligence {
				cost := game.PlayerCosts{}
				if view.Phase != "lobby" {
					cost.TicketCost = view.Rules.TicketFee
				}
				for _, h := range list {
					if h.Source != "specialty" {
						cost.CardCost += rm.game.UsedCardPrice(seat, h.Source)
					}
				}
				costs[seat] = cost
			}
			frame["player_costs"] = costs
			decisions := []game.ObserverDecision{}
			for _, d := range view.Decisions {
				if d.Round <= n {
					decisions = append(decisions, d)
				}
			}
			frame["decisions"] = decisions
			reveals := make(map[string][]game.ItemReveal, len(view.ItemReveals))
			for player, items := range view.ItemReveals {
				reveals[player] = []game.ItemReveal{}
				for _, item := range items {
					if item.Round <= n {
						reveals[player] = append(reveals[player], item)
					}
				}
			}
			frame["item_reveals"] = reveals
		}
		settled := view.Settlement != nil && n == len(view.History)
		if !settled {
			delete(frame, "settlement")
			delete(frame, "settled_at")
		}
		if view.Phase != "lobby" {
			if settled {
				frame["phase"] = "settled"
			} else if n == 0 {
				frame["phase"] = "opening"
			} else {
				frame["phase"] = "revealed"
			}
		}
		players := []map[string]any{}
		for _, raw := range full["players"].([]any) {
			p := raw.(map[string]any)
			copy := map[string]any{}
			for k, v := range p {
				copy[k] = v
			}
			copy["submitted"] = n > 0
			copy["withdrawn"] = slices.ContainsFunc(view.History[:n], func(r game.RoundResult) bool {
				return slices.ContainsFunc(r.Bids, func(b game.Bid) bool { return b.PlayerID == p["id"] && b.Amount == 0 })
			})
			players = append(players, copy)
		}
		frame["players"] = players
		frames = append(frames, frame)
	}
	if values, ok := r.URL.Query()["frame"]; ok {
		if len(values) != 1 {
			fail(w, 400, errors.New("one replay frame required"))
			return
		}
		cursor, err := strconv.Atoi(values[0])
		if err != nil || cursor < 0 || cursor > count {
			fail(w, 400, errors.New("replay frame outside recorded history"))
			return
		}
		reply(w, 200, frames[cursor])
		return
	}
	if observer {
		full["activities"] = activityView(rm, "", 6)
	}
	full["frames"] = frames
	reply(w, 200, full)
}
