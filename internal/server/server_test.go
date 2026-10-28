package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bidbrawl/internal/game"
)

// Protocol fixtures explicitly supply a table; production constructors stay empty.
func testTable(g *game.Game) *Server {
	s := New(g.GameConfig(), g.CardConfig())
	s.EnableWebUI()
	s.rooms["table"] = &room{id: "table", name: "Test table", game: g, members: map[string]string{}, seatAccounts: map[string]string{}, createdAt: time.Now().UTC()}
	return s
}

func request(t *testing.T, s http.Handler, method, path, token, body string) (int, []byte) {
	t.Helper()
	if !strings.Contains(path, "?") {
		switch path {
		case "/api/state", "/api/observer/state", "/api/start", "/api/join", "/api/bid", "/api/card", "/api/withdraw":
			path += "?room=table"
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w.Code, w.Body.Bytes()
}
func accountSession(t *testing.T, s *Server, name string) (string, string) {
	t.Helper()
	code, b := request(t, s, "POST", "/api/register", "", `{"name":"`+name+`"}`)
	if code != 201 {
		t.Fatal(code, string(b))
	}
	var reg map[string]string
	_ = json.Unmarshal(b, &reg)
	code, b = request(t, s, "POST", "/api/login", "", `{"credential":"`+reg["credential"]+`"}`)
	if code != 200 {
		t.Fatal(code, string(b))
	}
	var login map[string]string
	_ = json.Unmarshal(b, &login)
	return login["token"], reg["credential"]
}
func TestAuthenticationAndPrivacy(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	checkedRequest(t, s, "POST", "/api/register", "", `{"name":`, 400)
	for _, name := range []string{"", "   ", "A\nB", strings.Repeat("A", 33)} {
		body, _ := json.Marshal(map[string]string{"name": name})
		checkedRequest(t, s, "POST", "/api/register", "", string(body), 400)
	}
	tokens := []string{}
	for _, name := range []string{"A", "B"} {
		token, _ := accountSession(t, s, name)
		purchaseJoinCards(t, s, token, `{"cards":["largest_quality"]}`)
		code, b := request(t, s, "POST", "/api/join", token, `{"cards":["largest_quality"]}`)
		if code != 201 {
			t.Fatal(code, string(b))
		}
		tokens = append(tokens, token)
	}
	if code, _ := request(t, s, "POST", "/api/start", tokens[0], ""); code != 403 {
		t.Fatal("player started game")
	}
	if code, _ := request(t, s, "POST", "/api/start", s.HostToken, ""); code != 200 {
		t.Fatal(code)
	}
	if code, _ := request(t, s, "GET", "/api/state", "unknown", ""); code != 401 {
		t.Fatal(code)
	}
	_, before := request(t, s, "GET", "/api/state", "", "")
	code, b := request(t, s, "POST", "/api/card", tokens[0], `{"reason":"protocol test","round":1,"card":"largest_quality"}`)
	if code != 200 {
		t.Fatal(code, string(b))
	}
	var priv game.View
	_ = json.Unmarshal(b, &priv)
	if priv.Self == nil || len(priv.Self.Hints) != 1 {
		t.Fatal("missing private card result")
	}
	_, after := request(t, s, "GET", "/api/state", s.HostToken, "")
	if !bytes.Equal(before, after) {
		t.Fatal("host can see private change")
	}
	if code, _ := request(t, s, "POST", "/api/bid", tokens[0], `{"reason":"protocol test","round":1,"amount":42,"extra":1}`); code != 400 {
		t.Fatal("unknown field accepted")
	}
	if code, _ := request(t, s, "POST", "/api/bid", tokens[0], `{"reason":"protocol test","round":1,"amount":1.5}`); code != 400 {
		t.Fatal("fractional bid accepted")
	}
	for _, body := range []string{`{"reason":"protocol test","round":1}`, `{"reason":"protocol test","round":1,"amount":null}`} {
		if code, _ := request(t, s, "POST", "/api/bid", tokens[0], body); code != 400 {
			t.Fatal("missing amount accepted")
		}
	}
	if code, _ := request(t, s, "POST", "/api/bid", tokens[0], `{"reason":"protocol test","round":1,"amount":1} {}`); code != 400 {
		t.Fatal("trailing JSON accepted")
	}
	if code, _ := request(t, s, "POST", "/api/bid", tokens[0], `{"reason":"protocol test","round":1,"amount":999}`); code != 200 {
		t.Fatal(code)
	}
	_, b = request(t, s, "GET", "/api/state", "", "")
	var pub game.View
	_ = json.Unmarshal(b, &pub)
	if len(pub.History) != 0 || pub.Self != nil || pub.Settlement != nil {
		t.Fatal("public bid leak")
	}
}
func TestCrossOriginMutationAndCacheProtection(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	r := httptest.NewRequest("POST", "http://localhost/api/register", bytes.NewBufferString(`{"name":"X"}`))
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing private cache protection")
	}
}

func purchaseJoinCards(t *testing.T, s *Server, token, body string) {
	t.Helper()
	var v struct {
		Cards []string `json:"cards"`
	}
	_ = json.Unmarshal([]byte(body), &v)
	for _, id := range v.Cards {
		input, _ := json.Marshal(map[string]any{"card": id, "quantity": 1})
		status, data := request(t, s, "POST", "/api/buy", token, string(input))
		if status != 200 {
			t.Fatalf("purchase before join: %d %s", status, data)
		}
	}
}
