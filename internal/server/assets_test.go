package server

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
)

// Deterministic HTTP protocol fixtures, not autonomous-player game acceptance.
func TestAccountAssetsAcrossRoomsDeletionAndRestart(t *testing.T) {
	for _, value := range []int64{1, 2000000} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			cfg := game.DefaultConfig()
			cfg.InitialAssets = 100000
			cfg.TicketFee = 0
			cfg.Box = game.Dimensions{Width: 1, Height: 1}
			cfg.Items = game.ItemLimits{MaxWidth: 1, MaxHeight: 1}
			for i := range cfg.Qualities {
				q := &cfg.Qualities[i]
				q.Weight = 0
				if q.ID == "common" {
					q.Min, q.Max, q.Weight = value, value, 100
				}
			}
			path := filepath.Join(t.TempDir(), "accounts.db")
			open := func() *Server {
				s, err := Open(cfg, cards.Default(), path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = s.Close() })
				return s
			}
			s := open()
			a, ca := accountSession(t, s, "Atlas")
			b, cb := accountSession(t, s, "Echo")
			newRoom := func() string {
				raw := checkedRequest(t, s, "POST", "/api/create", s.HostToken, `{"name":"Asset fixture","players":2}`, 201)
				var info struct {
					ID string `json:"room_id"`
				}
				_ = json.Unmarshal(raw, &info)
				return info.ID
			}
			first, second := newRoom(), newRoom()
			for _, token := range []string{a, b} {
				purchaseJoinCards(t, s, token, `{"cards":[]}`)
				checkedRequest(t, s, "POST", "/api/join?room="+first, token, `{"cards":[]}`, 201)
			}
			checkedRequest(t, s, "POST", "/api/join?room="+second, a, `{"cards":[]}`, 409)
			own := func(room, token string, expected int64) {
				var view game.View
				_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state?room="+room, token, "", 200), &view)
				if view.Self == nil || view.Self.Assets != expected || view.Self.MaxBid != expected {
					t.Fatalf("asset limit = %+v, want %d", view.Self, expected)
				}
				var profile accountProfile
				if err := json.Unmarshal(checkedRequest(t, s, "GET", "/api/me", token, "", 200), &profile); err != nil {
					t.Fatal(err)
				}
				var webProfile agentMetrics
				_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/agents/"+profile.ID, "", "", 200), &webProfile)
				if profile.Assets != expected || !reflect.DeepEqual(profile.Stats, webProfile) {
					t.Fatal("self profile differs from assets or WebUI metrics")
				}
			}
			own(first, a, 100000)
			checkedRequest(t, s, "POST", "/api/start?room="+first, s.HostToken, "", 200)
			checkedRequest(t, s, "POST", "/api/join?room="+second, a, `{"cards":[]}`, 409)
			checkedRequest(t, s, "POST", "/api/bid?room="+first, a, `{"round":1,"amount":100001,"reason":"protocol fixture"}`, 409)
			checkedRequest(t, s, "POST", "/api/bid?room="+first, a, `{"round":1,"amount":100000,"reason":"protocol fixture"}`, 200)
			// A pending bid has not paid anything yet.
			own(first, a, 100000)
			_ = s.Close()
			s = open()
			a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
			own(first, a, 100000)
			checkedRequest(t, s, "POST", "/api/join?room="+second, a, `{"cards":[]}`, 409)
			checkedRequest(t, s, "POST", "/api/withdraw?room="+first, b, `{"round":1,"reason":"protocol fixture"}`, 200)
			balance := int64(0) // Purchase leaves cash at zero; warehouse retains the item.
			if len(s.sessions[a].Warehouse) != 1 || s.sessions[a].Warehouse[0].Item.Price != value {
				t.Fatal("missing purchased inventory")
			}
			own(first, a, balance)
			own(first, b, 100000)
			var profile accountProfile
			_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/me", a, "", 200), &profile)
			profit := value - 100000
			if profile.Stats.Games != 1 || profile.Stats.Profit != profit || profile.Stats.Paid != 100000 || profile.Stats.Average != float64(profit) || profile.Stats.MaxProfit != max(int64(0), profit) || profile.Stats.MaxLoss != min(int64(0), profit) || profile.Stats.Rate == nil || *profile.Stats.Rate != 100*float64(profit)/100000 {
				t.Fatal("incorrect self economic metrics", profile.Stats)
			}
			for _, token := range []string{a, b} {
				purchaseJoinCards(t, s, token, `{"cards":[]}`)
				checkedRequest(t, s, "POST", "/api/join?room="+second, token, `{"cards":[]}`, 201)
			}
			own(second, a, balance)
			checkedRequest(t, s, "POST", "/api/rooms/manage", s.HostToken, fmt.Sprintf(`{"action":"archive","room_ids":[%q]}`, first), 200)
			own(first, a, balance)
			checkedRequest(t, s, "POST", "/api/rooms/manage", s.HostToken, fmt.Sprintf(`{"action":"delete","room_ids":[%q]}`, first), 200)
			own(second, a, balance)
			checkedRequest(t, s, "POST", "/api/start?room="+second, s.HostToken, "", 200)
			checkedRequest(t, s, "POST", "/api/bid?room="+second, a, fmt.Sprintf(`{"round":1,"amount":%d,"reason":"protocol fixture"}`, balance+1), 409)
			checkedRequest(t, s, "POST", "/api/bid?room="+second, a, fmt.Sprintf(`{"round":1,"amount":%d,"reason":"protocol fixture"}`, max(balance, 1)), map[bool]int{true: 200, false: 409}[balance > 0])
			_ = s.Close()
			cfg.InitialAssets = 999999 // New accounts only; existing grants remain fixed.
			s = open()
			a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
			own(second, a, balance)
			var me struct {
				Assets int64 `json:"assets"`
			}
			_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/me", a, "", 200), &me)
			if me.Assets != balance {
				t.Fatal("restart/config change changed account assets")
			}
			c, _ := accountSession(t, s, "Moss")
			_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/me", c, "", 200), &me)
			if me.Assets != cfg.InitialAssets {
				t.Fatal("new account ignored configured grant")
			}
			if balance == 0 {
				checkedRequest(t, s, "POST", "/api/withdraw?room="+second, a, `{"round":1,"reason":"cash exhausted"}`, 200)
			}
			checkedRequest(t, s, "POST", "/api/withdraw?room="+second, b, `{"round":1,"reason":"protocol fixture"}`, 200)
			own(second, a, 0)
			// Repeated queries/restarts must not apply a settlement twice.
			_ = s.Close()
			s = open()
			a = loginAgain(t, s, ca)
			own(second, a, 0)
		})
	}
}

func TestFailedSettlementDoesNotChangeAssets(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	a, _ := accountSession(t, s, "Atlas")
	b, _ := accountSession(t, s, "Echo")
	for _, token := range []string{a, b} {
		purchaseJoinCards(t, s, token, `{"cards":[]}`)
		checkedRequest(t, s, "POST", "/api/join", token, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	checkedRequest(t, s, "POST", "/api/bid", a, `{"round":1,"amount":100000,"reason":"protocol fixture"}`, 200)
	_ = s.Close()
	checkedRequest(t, s, "POST", "/api/withdraw", b, `{"round":1,"reason":"protocol fixture"}`, 500)
	var own game.View
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state", a, "", 200), &own)
	if own.Settlement != nil || own.Self.Assets != game.DefaultConfig().InitialAssets-game.DefaultConfig().TicketFee {
		t.Fatal("failed transaction changed assets")
	}
}

func TestOnlyOneUnfinishedRoomPerAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, credential := accountSession(t, s, "Atlas")
	first := lifecycleRoom(t, s, s.HostToken, "First lobby")
	second := lifecycleRoom(t, s, s.HostToken, "Second lobby")
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, id := range []string{first, second} {
		purchaseJoinCards(t, s, a, `{"cards":[]}`)
		wg.Go(func() { code, _ := request(t, s, "POST", "/api/join?room="+id, a, `{"cards":[]}`); codes <- code })
	}
	wg.Wait()
	close(codes)
	joined := 0
	for code := range codes {
		if code == 201 {
			joined++
		} else if code != 409 {
			t.Fatalf("join: HTTP %d", code)
		}
	}
	if joined != 1 {
		t.Fatal("concurrent requests joined multiple rooms")
	}
	actor := s.sessions[a]
	if s.rooms[first].members[actor.ID] == "" {
		first, second = second, first
	}
	lifecycleRequest(t, s, s.HostToken, "archive", []string{first}, 200)
	_ = s.Close()
	s = persistentServer(t, path)
	a = loginAgain(t, s, credential)
	checkedRequest(t, s, "POST", "/api/join?room="+second, a, `{"cards":[]}`, 409)
	lifecycleRequest(t, s, s.HostToken, "delete", []string{first}, 200)
	purchaseJoinCards(t, s, a, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join?room="+second, a, `{"cards":[]}`, 201)
}
