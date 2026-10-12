package server

import (
	"bidbrawl/internal/game"
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

type resultPlayer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
}
type resultSettlement struct {
	WinnerID string           `json:"winner_id"`
	Paid     int64            `json:"paid"`
	Total    int64            `json:"total_value"`
	Profit   int64            `json:"profit"`
	Payoffs  map[string]int64 `json:"payoffs"`
	Round    int              `json:"round"`
	Method   string           `json:"method"`
}
type roomResult struct {
	RoomID     string           `json:"room_id"`
	RoomName   string           `json:"room_name"`
	Players    []resultPlayer   `json:"players"`
	CreatedAt  time.Time        `json:"created_at"`
	SettledAt  time.Time        `json:"settled_at"`
	Settlement resultSettlement `json:"settlement"`
}

// Caller holds s.mu. This public projection never contains reasons or skill results.
func (s *Server) resultsLocked() ([]roomResult, error) {
	ids := make([]string, 0, len(s.rooms))
	for id := range s.rooms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	results := []roomResult{}
	for _, id := range ids {
		rm := s.rooms[id]
		public, err := rm.game.Observe("")
		if err != nil {
			return nil, err
		}
		var view game.View
		if err = json.Unmarshal(public, &view); err != nil {
			return nil, err
		}
		if view.Settlement == nil {
			continue
		}
		players := []resultPlayer{}
		for _, p := range view.Players {
			players = append(players, resultPlayer{p.ID, p.Name, "account:" + rm.seatAccounts[p.ID]})
		}
		r := view.Settlement
		results = append(results, roomResult{id, rm.name, players, rm.createdAt, rm.settledAt, resultSettlement{r.WinnerID, r.Paid, r.Total, r.Profit, r.Payoffs, r.Round, r.Method}})
	}
	return results, nil
}
func (s *Server) listResults(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	results, err := s.resultsLocked()
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, map[string]any{"rooms": results})
}
