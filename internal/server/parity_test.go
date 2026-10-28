package server

import (
	"bidbrawl/internal/game"
	"encoding/json"
	"testing"
)

func TestSelfProfileAuthenticationAndEmptyLeaderboard(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	a, _ := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	for _, token := range []string{"", "invalid", s.HostToken, s.ObserverToken} {
		checkedRequest(t, s, "GET", "/api/me", token, "", 401)
	}
	var own accountProfile
	raw := checkedRequest(t, s, "GET", "/api/me?account_id="+s.sessions[b].ID, a, "", 200)
	if err := json.Unmarshal(raw, &own); err != nil {
		t.Fatal(err)
	}
	if own.ID != s.sessions[a].ID || own.Name != "A" || own.Stats.ID != "account:"+own.ID || own.Stats.Games != 0 || own.Stats.Rate != nil {
		t.Fatal("me queried another identity or invented history")
	}
	var board leaderboard
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/leaderboard", "", "", 200), &board)
	if board.Sort != "profit" || board.Games != 0 || board.Rankings == nil || len(board.Rankings) != 0 {
		t.Fatal("incorrect empty leaderboard")
	}
	checkedRequest(t, s, "POST", "/api/logout", a, "", 200)
	checkedRequest(t, s, "GET", "/api/me", a, "", 401)
}

func TestStatisticsEconomics(t *testing.T) {
	results := []roomResult{
		{Players: []resultPlayer{{"p1", "A", "account:a"}, {"p2", "B", "account:b"}}, Settlement: resultSettlement{WinnerID: "p1", Paid: 100, Profit: 20, Payoffs: map[string]int64{"p1": 20, "p2": 0}}},
		{Players: []resultPlayer{{"p2", "A", "account:a"}, {"p1", "C", "account:c"}}, Settlement: resultSettlement{WinnerID: "p2", Paid: 300, Profit: -10, Payoffs: map[string]int64{"p2": -10, "p1": 0}}},
		{Players: []resultPlayer{{"p1", "A", "account:a"}, {"p2", "B", "account:b"}}, Settlement: resultSettlement{WinnerID: "p2", Paid: 0, Profit: 50, Payoffs: map[string]int64{"p1": 0, "p2": 50}}},
	}
	stats := calculateStatistics(results)
	if stats.Games != 3 || stats.Profit != 60 || stats.Paid != 400 || *stats.Rate != 15 {
		t.Fatal(stats)
	}
	records := map[string]agentMetrics{}
	for _, a := range stats.Agents {
		records[a.ID] = a
	}
	a := records["account:a"]
	if a.Games != 3 || a.Average != float64(10)/3 || a.Profit != 10 || *a.Rate != 2.5 || a.MaxProfit != 20 || a.MaxLoss != -10 {
		t.Fatal(a)
	}
	b := records["account:b"]
	if b.Games != 2 || b.Rate != nil || b.MaxProfit != 50 || b.Average != 25 {
		t.Fatal(b)
	}
	if sortAgents(stats.Agents, "max_loss") != nil || stats.Agents[0].ID != "account:a" {
		t.Fatal(stats.Agents)
	}
	if sortAgents(stats.Agents, "rate") != nil || stats.Agents[0].ID != "account:a" {
		t.Fatal(stats.Agents)
	}
	if sortAgents(stats.Agents, "wins") == nil {
		t.Fatal("invalid sort accepted")
	}
	empty := calculateStatistics(nil)
	if empty.Rate != nil || len(empty.Agents) != 0 {
		t.Fatal(empty)
	}
}
