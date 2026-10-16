package client

import (
	"bidbrawl/internal/game"
	"bidbrawl/internal/server"
	"context"
	"net/http/httptest"
	"testing"
)

func TestExternalClientProtocolFlow(t *testing.T) {
	g, _ := game.New(2)
	s := server.New(g.GameConfig(), g.CardConfig())
	host := httptest.NewServer(s)
	defer host.Close()
	ctx := context.Background()
	clients := []*Client{New(host.URL, ""), New(host.URL, "")}
	admin := New(host.URL, s.HostToken)
	var created struct {
		ID string `json:"room_id"`
	}
	if err := admin.Request(ctx, "POST", "/api/create", map[string]any{"name": "Client table", "players": 2}, &created); err != nil {
		t.Fatal(err)
	}
	roomID := created.ID
	for i, c := range clients {
		var registered struct {
			Credential string `json:"credential"`
		}
		if e := c.Request(ctx, "POST", "/api/register", map[string]string{"name": string(rune('A' + i))}, &registered); e != nil {
			t.Fatal(e)
		}
		var logged struct {
			Token string `json:"token"`
		}
		if e := c.Request(ctx, "POST", "/api/login", map[string]string{"credential": registered.Credential}, &logged); e != nil {
			t.Fatal(e)
		}
		c.Token = logged.Token
		if e := c.Request(ctx, "POST", RoomPath("/api/join", roomID), map[string]any{"cards": []string{"item_count"}}, nil); e != nil {
			t.Fatal(e)
		}
	}
	if e := admin.Request(ctx, "POST", RoomPath("/api/start", roomID), nil, nil); e != nil {
		t.Fatal(e)
	}
	if e := clients[0].Request(ctx, "POST", RoomPath("/api/card", roomID), map[string]any{"reason": "protocol test", "round": 1, "card": "item_count"}, nil); e != nil {
		t.Fatal(e)
	}
	a, _ := clients[0].Observe(ctx, roomID)
	b, _ := clients[1].Observe(ctx, roomID)
	if len(a.Self.Hints) != 1 || len(b.Self.Hints) != 0 {
		t.Fatal("private observation leak")
	}
	for r := 1; r <= 5; r++ {
		for i, c := range clients {
			amount := 1000 - i
			if r == 5 {
				amount = 100 + i
			}
			if e := c.Request(ctx, "POST", RoomPath("/api/bid", roomID), map[string]any{"reason": "protocol test", "round": r, "amount": amount}, nil); e != nil {
				t.Fatal(e)
			}
		}
	}
	v, e := clients[0].Observe(ctx, roomID)
	if e != nil {
		t.Fatal(e)
	}
	if v.Phase != "settled" || len(v.History) != 5 || v.Settlement.Paid != 101 || v.Settlement.WinnerID != "p2" {
		t.Fatal("wrong settlement")
	}
	if RoomPath("/api/state", "a&b") != "/api/state?room=a%26b" {
		t.Fatal("room query not escaped")
	}
}
