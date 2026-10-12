package server

import (
	"bidbrawl/internal/game"
	"encoding/json"
	"errors"
	"net/http"
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
	if observer {
		full["can_manage"] = s.isRoomManager(r)
	}
	if !rm.settledAt.IsZero() {
		full["settled_at"] = rm.settledAt
	}
	for _, raw := range full["players"].([]any) {
		p := raw.(map[string]any)
		p["agent_id"] = "account:" + rm.seatAccounts[p["id"].(string)]
	}
	count := len(view.History)
	if view.Settlement != nil {
		count++
	}
	frames := make([]map[string]any, 0, count+1)
	for cursor := 0; cursor <= count; cursor++ {
		frame := make(map[string]any, len(full))
		for k, v := range full {
			frame[k] = v
		}
		n := min(cursor, len(view.History))
		frame["history"] = view.History[:n]
		hints := []game.Hint{}
		for _, h := range view.PublicHints {
			if h.Round <= min(n+1, view.Rules.Rounds) {
				hints = append(hints, h)
			}
		}
		frame["public_hints"] = hints
		if observer {
			decisions := []game.ObserverDecision{}
			for _, d := range view.Decisions {
				if d.Round <= n {
					decisions = append(decisions, d)
				}
			}
			frame["decisions"] = decisions
		}
		settled := view.Settlement != nil && n == len(view.History)
		if !settled {
			delete(frame, "settlement")
			delete(frame, "settled_at")
		}
		if view.Phase != "lobby" {
			if settled {
				frame["phase"] = "settled"
			} else {
				frame["phase"] = "bidding"
				frame["round"] = min(n+1, view.Rules.Rounds)
			}
		}
		players := []map[string]any{}
		for _, raw := range full["players"].([]any) {
			p := raw.(map[string]any)
			copy := map[string]any{}
			for k, v := range p {
				copy[k] = v
			}
			copy["submitted"] = view.Settlement == nil && n == len(view.History) && p["submitted"].(bool)
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
	full["frames"] = frames
	reply(w, 200, full)
}
