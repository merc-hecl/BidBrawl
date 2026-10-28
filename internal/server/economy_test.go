package server

import (
	"bidbrawl/internal/game"
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestInventoryPurchaseConsumptionSettlementAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, ca := accountSession(t, s, "Buyer")
	b, cb := accountSession(t, s, "Other")
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["item_count"]}`, 409)
	checkedRequest(t, s, "POST", "/api/buy", a, `{"card":"item_count","quantity":2}`, 200)
	checkedRequest(t, s, "POST", "/api/buy", b, `{"card":"random_quality","quantity":1}`, 200)
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["item_count"],"specialty":"quick"}`, 201)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":["random_quality"],"specialty":"research"}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	checkedRequest(t, s, "POST", "/api/buy", a, `{"card":"quality_counts","quantity":1}`, 409)
	checkedRequest(t, s, "POST", "/api/loadout?room=table", a, `{"cards":[],"specialty":"research"}`, 409)
	checkedRequest(t, s, "POST", "/api/card", a, `{"card":"item_count","round":1,"reason":"count before estimating"}`, 200)
	checkedRequest(t, s, "POST", "/api/card", b, `{"card":"random_quality","round":1,"reason":"inspect before bidding"}`, 200)
	checkedRequest(t, s, "POST", "/api/card", a, `{"card":"item_count","round":1,"reason":"duplicate"}`, 409)
	if len(s.sessions[a].CardStock["item_count"]) != 1 {
		t.Fatal("card not consumed exactly once")
	}
	checkedRequest(t, s, "POST", "/api/bid", a, `{"amount":10000,"round":1,"reason":"bid from observation"}`, 200)
	checkedRequest(t, s, "POST", "/api/withdraw", b, `{"round":1,"reason":"not worth pursuing"}`, 200)
	var v game.View
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state", a, "", 200), &v)
	if v.Settlement == nil || v.Settlement.WinnerID != "p1" || v.Settlement.Payoffs["p2"] != -5500 || v.Settlement.TotalCardCost != 1700 {
		t.Fatal("costs missing from payoffs", v.Settlement)
	}
	winner := s.sessions[a]
	if winner.Assets != game.DefaultConfig().InitialAssets-2400-10000-5000 || len(winner.Warehouse) != len(v.Settlement.Items) {
		t.Fatal("cash or acquired inventory incorrect")
	}
	if s.sessions[b].Assets != game.DefaultConfig().InitialAssets-500-5000 {
		t.Fatal("loser charged twice")
	}
	public := checkedRequest(t, s, "GET", "/api/replay?room=table", "", "", 200)
	for _, field := range []string{`"intelligence"`, `"card_stock"`, `"initial_inspections"`, `"decisions"`, `"self"`} {
		if bytes.Contains(public, []byte(field)) {
			t.Fatal("public replay contains private data", field)
		}
	}
	_ = s.Close()
	s = persistentServer(t, path)
	a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
	if len(s.sessions[a].Warehouse) != len(v.Settlement.Items) || len(s.sessions[a].CardStock["item_count"]) != 1 {
		t.Fatal("restart changed inventory")
	}
	before := s.sessions[a].Assets
	checkedRequest(t, s, "POST", "/api/sell", a, `{"all":true}`, 200)
	if s.sessions[a].Assets != before+v.Settlement.Total || len(s.sessions[a].Warehouse) != 0 {
		t.Fatal("buyback incorrect")
	}
	checkedRequest(t, s, "POST", "/api/sell", a, `{"all":true}`, 200)
	if s.sessions[a].Assets != before+v.Settlement.Total {
		t.Fatal("repeat sale minted cash")
	}
	checkedRequest(t, s, "POST", "/api/sell", b, `{"ids":["not-owned"]}`, 409)
	stats, _ := s.accountStatsLocked(s.sessions[b])
	if stats.Profit != -5500 || stats.CardCost != 500 || stats.TicketCost != 5000 || stats.CashChange != 0 {
		t.Fatal("loser economic metrics", stats)
	}
}

func TestFailedCardPersistenceKeepsInventoryAndRoom(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	a, _ := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	checkedRequest(t, s, "POST", "/api/buy", a, `{"card":"item_count","quantity":1}`, 200)
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["item_count"]}`, 201)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	before := checkedRequest(t, s, "GET", "/api/state", a, "", 200)
	_ = s.Close()
	checkedRequest(t, s, "POST", "/api/card", a, `{"card":"item_count","round":1,"reason":"transaction failure fixture"}`, 500)
	after := checkedRequest(t, s, "GET", "/api/state", a, "", 200)
	if !sameGameProjection(before, after) || len(s.sessions[a].CardStock["item_count"]) != 1 {
		t.Fatal("failed transaction consumed card or published result")
	}
}

func TestUnsoldAwardsNothingAndReliefChecksAllWealth(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	a, _ := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	checkedRequest(t, s, "POST", "/api/withdraw", a, `{"round":1,"reason":"abstain"}`, 200)
	var public game.View
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state", "", "", 200), &public)
	if public.Players[0].Withdrawn {
		t.Fatal("pending zero bid leaked")
	}
	checkedRequest(t, s, "POST", "/api/withdraw", b, `{"round":1,"reason":"abstain"}`, 200)
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state", "", "", 200), &public)
	if public.Settlement.Method != "unsold" || public.Settlement.WinnerID != "" || len(s.sessions[a].Warehouse) != 0 {
		t.Fatal("unsold gave inventory")
	}
	s.sessions[a].Assets = 0
	s.sessions[a].CardStock["quality_counts"] = []int64{12000}
	checkedRequest(t, s, "POST", "/api/relief", a, "", 409)
	s.sessions[a].CardStock = map[string][]int64{}
	checkedRequest(t, s, "POST", "/api/relief", a, "", 200)
	s.sessions[a].Assets = 0
	checkedRequest(t, s, "POST", "/api/relief", a, "", 409)
}
