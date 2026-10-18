package server

import (
	"bidbrawl/internal/game"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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
func TestHeadlessObserverStatisticsAndReplay(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	s.webUI = false
	a, _ := accountSession(t, s, "A")
	accountID := s.sessions[a].ID
	b, _ := accountSession(t, s, "B")
	checkedRequest(t, s, "POST", "/api/observer/login", a, "", 403)
	var credential map[string]string
	json.Unmarshal(checkedRequest(t, s, "POST", "/api/observer/login", s.HostToken, "", 200), &credential)
	observer := credential["observer_token"]
	if observer == "" || observer == s.HostToken {
		t.Fatal("invalid observer credential")
	}
	checkedRequest(t, s, "POST", "/api/create", observer, `{"name":"No","players":2}`, 401)
	checkedRequest(t, s, "POST", "/api/observer/login", observer, "", 403)
	checkedRequest(t, s, "GET", "/api/observer/replay?room=table", a, "", 401)
	checkedRequest(t, s, "GET", "/api/agents/"+accountID, "", "", 200)
	checkedRequest(t, s, "GET", "/api/agents/missing", "", "", 404)
	checkedRequest(t, s, "GET", "/api/stats?sort=wins", "", "", 400)
	purchaseJoinCards(t, s, a, `{"cards":["item_count"]}`)
	checkedRequest(t, s, "POST", "/api/join?room=table", a, `{"cards":["item_count"]}`, 201)
	purchaseJoinCards(t, s, b, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join?room=table", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start?room=table", s.HostToken, "", 200)
	var contents []game.Item
	for _, route := range []string{"/api/observer/state?room=table", "/api/observer/replay?room=table&frame=0"} {
		var opening game.View
		json.Unmarshal(checkedRequest(t, s, "GET", route, observer, "", 200), &opening)
		if len(opening.Items) == 0 || opening.Settlement != nil || opening.Self != nil {
			t.Fatal("observer cannot inspect the complete box before settlement")
		}
		if contents != nil && !reflect.DeepEqual(contents, opening.Items) {
			t.Fatal("observer state and opening replay have different contents")
		}
		contents = opening.Items
	}
	checkedRequest(t, s, "POST", "/api/card?room=table", a, `{"round":1,"card":"item_count","reason":"secret first decision"}`, 200)
	for round := 1; round <= 5; round++ {
		for _, route := range []string{"/api/state?room=table", "/api/replay?room=table"} {
			for _, token := range []string{"", a, b, s.HostToken} {
				var public struct {
					game.View
					Frames []game.View `json:"frames"`
				}
				json.Unmarshal(checkedRequest(t, s, "GET", route, token, "", 200), &public)
				for _, frame := range append(public.Frames, public.View) {
					if len(frame.Items) != 0 || frame.Settlement != nil {
						t.Fatal("complete box leaked before settlement through public/player route")
					}
				}
			}
		}
		for _, token := range []string{a, b} {
			checkedRequest(t, s, "POST", "/api/bid?room=table", token, fmt.Sprintf(`{"round":%d,"amount":100,"reason":"secret round %d"}`, round, round), 200)
		}
	}
	public := checkedRequest(t, s, "GET", "/api/replay?room=table", a, "", 200)
	if strings.Contains(string(public), "secret") || strings.Contains(string(public), `"self"`) {
		t.Fatal("private replay leaked")
	}
	var full map[string]any
	json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/replay?room=table", observer, "", 200), &full)
	if _, exists := full["self"]; exists || len(full["decisions"].([]any)) != 11 || len(full["frames"].([]any)) != 6 {
		t.Fatal("wrong observer projection")
	}
	for cursor := 0; cursor <= 5; cursor++ {
		var frame game.View
		json.Unmarshal(checkedRequest(t, s, "GET", fmt.Sprintf("/api/observer/replay?room=table&frame=%d", cursor), observer, "", 200), &frame)
		if !reflect.DeepEqual(contents, frame.Items) || cursor == 5 && !reflect.DeepEqual(contents, frame.Settlement.Items) {
			t.Fatal("observer replay lost complete contents or diverged from settlement")
		}
	}
	var fourth game.View
	json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/replay?room=table&frame=4", observer, "", 200), &fourth)
	if fourth.Settlement != nil || len(fourth.History) != 4 || len(fourth.Decisions) != 9 || fourth.Self != nil {
		t.Fatal("future replay leaked")
	}
	var opening game.View
	json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/replay?room=table&frame=0", observer, "", 200), &opening)
	if opening.Settlement != nil || len(opening.Decisions) != 0 || len(opening.History) != 0 {
		t.Fatal("opening leaked")
	}
	var final game.View
	json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/replay?room=table&frame=5", observer, "", 200), &final)
	if final.Settlement == nil || len(final.History) != 5 {
		t.Fatal("final reveal incomplete")
	}
	for _, frame := range []string{"-1", "6", "x", "1&frame=2"} {
		checkedRequest(t, s, "GET", "/api/replay?room=table&frame="+frame, "", "", 400)
	}
	var summary statistics
	json.Unmarshal(checkedRequest(t, s, "GET", "/api/stats?sort=rate", b, "", 200), &summary)
	var profile agentMetrics
	json.Unmarshal(checkedRequest(t, s, "GET", "/api/agents/"+accountID, b, "", 200), &profile)
	if summary.Games != 1 || len(summary.Agents) != 2 || profile.Games != 1 {
		t.Fatal("statistics missing game")
	}
	for _, key := range []string{"profit", "rate", "average", "max_profit", "max_loss"} {
		var expected statistics
		_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/stats?sort="+key, "", "", 200), &expected)
		var board leaderboard
		raw := checkedRequest(t, s, "GET", "/api/leaderboard?sort="+key, "", "", 200)
		_ = json.Unmarshal(raw, &board)
		if board.Sort != key || board.Games != expected.Games || len(board.Rankings) != len(expected.Agents) {
			t.Fatal("leaderboard missing settled players")
		}
		for i, row := range board.Rankings {
			if row.Rank != i+1 || !reflect.DeepEqual(row.agentMetrics, expected.Agents[i]) {
				t.Fatal("leaderboard diverged from WebUI statistics")
			}
		}
		for _, forbidden := range []string{`"assets"`, `"credential"`, `"token"`, `"decisions"`, `"hints"`, "secret"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal("leaderboard leaked private information")
			}
		}
	}
	checkedRequest(t, s, "GET", "/api/leaderboard?sort=wins", "", "", 400)
	for _, record := range summary.Agents {
		if record.ID == profile.ID && (record.Profit != profile.Profit || record.Average != profile.Average) {
			t.Fatal("agent metrics diverged")
		}
	}
}
func TestInstantReplayFinalAtomic(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	a, _ := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	for _, token := range []string{a, b} {
		purchaseJoinCards(t, s, token, `{"cards":[]}`)
		checkedRequest(t, s, "POST", "/api/join?room=table", token, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "POST", "/api/start?room=table", s.HostToken, "", 200)
	checkedRequest(t, s, "POST", "/api/bid?room=table", a, `{"round":1,"amount":1000,"reason":"early"}`, 200)
	checkedRequest(t, s, "POST", "/api/bid?room=table", b, `{"round":1,"amount":1,"reason":"low"}`, 200)
	for _, route := range []struct{ path, token string }{
		{"/api/replay?room=table", ""},
		{"/api/observer/replay?room=table", s.ObserverToken},
	} {
		var replay struct {
			Frames []game.View `json:"frames"`
		}
		json.Unmarshal(checkedRequest(t, s, "GET", route.path, route.token, "", 200), &replay)
		if len(replay.Frames) != 2 {
			t.Fatalf("expected opening and one atomic settlement frame, got %d", len(replay.Frames))
		}
		for frame := range replay.Frames {
			var view game.View
			json.Unmarshal(checkedRequest(t, s, "GET", fmt.Sprintf("%s&frame=%d", route.path, frame), route.token, "", 200), &view)
			if (view.Settlement != nil) != (frame == 1) || len(view.History) != frame || !reflect.DeepEqual(view, replay.Frames[frame]) {
				t.Fatal("instant reveal not atomic or frame endpoint diverged")
			}
		}
		checkedRequest(t, s, "GET", route.path+"&frame=2", route.token, "", 400)
	}
}
