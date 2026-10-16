package server

import (
	"errors"
	"net/http"
	"sort"
)

type agentMetrics struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Games     int      `json:"games"`
	Profit    int64    `json:"profit"`
	Paid      int64    `json:"paid"`
	MaxProfit int64    `json:"max_profit"`
	MaxLoss   int64    `json:"max_loss"`
	Rate      *float64 `json:"rate"`
	Average   float64  `json:"average"`
}
type statistics struct {
	Games  int            `json:"games"`
	Profit int64          `json:"profit"`
	Paid   int64          `json:"paid"`
	Rate   *float64       `json:"rate"`
	Agents []agentMetrics `json:"agents"`
}
type rankedAgent struct {
	Rank int `json:"rank"`
	agentMetrics
}
type leaderboard struct {
	Sort     string        `json:"sort"`
	Games    int           `json:"games"`
	Rankings []rankedAgent `json:"rankings"`
}

func profitRate(profit, paid int64) *float64 {
	if paid == 0 {
		return nil
	}
	rate := 100 * float64(profit) / float64(paid)
	return &rate
}
func calculateStatistics(results []roomResult) statistics {
	out := statistics{Games: len(results), Agents: []agentMetrics{}}
	agents := map[string]*agentMetrics{}
	for _, r := range results {
		out.Profit += r.Settlement.Profit
		out.Paid += r.Settlement.Paid
		for _, p := range r.Players {
			a := agents[p.AgentID]
			if a == nil {
				a = &agentMetrics{ID: p.AgentID, Name: p.Name}
				agents[p.AgentID] = a
			}
			a.Games++
			payoff := r.Settlement.Payoffs[p.ID]
			a.Profit += payoff
			a.MaxProfit = max(a.MaxProfit, payoff)
			a.MaxLoss = min(a.MaxLoss, payoff)
			if r.Settlement.WinnerID == p.ID {
				a.Paid += r.Settlement.Paid
			}
		}
	}
	out.Rate = profitRate(out.Profit, out.Paid)
	for _, a := range agents {
		a.Rate = profitRate(a.Profit, a.Paid)
		a.Average = float64(a.Profit) / float64(a.Games)
		out.Agents = append(out.Agents, *a)
	}
	return out
}
func sortAgents(agents []agentMetrics, key string) error {
	value := func(a agentMetrics) float64 {
		switch key {
		case "profit":
			return float64(a.Profit)
		case "rate":
			if a.Rate != nil {
				return *a.Rate
			}
		case "average":
			return a.Average
		case "max_profit":
			return float64(a.MaxProfit)
		case "max_loss":
			return float64(a.MaxLoss)
		}
		return 0
	}
	switch key {
	case "profit", "rate", "average", "max_profit", "max_loss":
	default:
		return errors.New("invalid statistics sort")
	}
	sort.Slice(agents, func(i, j int) bool {
		a, b := agents[i], agents[j]
		if key == "rate" && (a.Rate == nil) != (b.Rate == nil) {
			return a.Rate != nil
		}
		x, y := value(a), value(b)
		if x == y {
			return a.ID < b.ID
		}
		if key == "max_loss" {
			return x < y
		}
		return x > y
	})
	return nil
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	s.serveStatistics(w, r, false)
}
func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request) {
	s.serveStatistics(w, r, true)
}
func (s *Server) serveStatistics(w http.ResponseWriter, r *http.Request, ranked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	results, err := s.resultsLocked()
	if err != nil {
		fail(w, 500, err)
		return
	}
	out := calculateStatistics(results)
	key := r.URL.Query().Get("sort")
	if key == "" {
		key = "profit"
	}
	if err = sortAgents(out.Agents, key); err != nil {
		fail(w, 400, err)
		return
	}
	if ranked {
		rows := make([]rankedAgent, 0, len(out.Agents))
		for i, a := range out.Agents {
			rows = append(rows, rankedAgent{Rank: i + 1, agentMetrics: a})
		}
		reply(w, 200, leaderboard{Sort: key, Games: out.Games, Rankings: rows})
	} else {
		reply(w, 200, out)
	}
}
func (s *Server) agentStats(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.PathValue("id")
	a := s.accountsByID[id]
	if a == nil {
		fail(w, 404, errors.New("agent not found"))
		return
	}
	record, err := s.accountStatsLocked(a)
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, record)
}

// Caller holds s.mu. Self queries and WebUI profiles share the same metrics.
func (s *Server) accountStatsLocked(a *account) (agentMetrics, error) {
	results, err := s.resultsLocked()
	if err != nil {
		return agentMetrics{}, err
	}
	for _, record := range calculateStatistics(results).Agents {
		if record.ID == "account:"+a.ID {
			return record, nil
		}
	}
	return agentMetrics{ID: "account:" + a.ID, Name: a.Name}, nil
}
