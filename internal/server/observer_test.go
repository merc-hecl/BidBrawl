package server

import (
	"bidbrawl/internal/game"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecisionReasonJSONSizeBoundary(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	a, _ := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	purchaseJoinCards(t, s, a, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":[]}`, 201)
	purchaseJoinCards(t, s, b, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	// Go's JSON encoder expands HTML characters to six-byte escapes.
	body, err := json.Marshal(map[string]any{"round": 1, "amount": 100, "reason": strings.Repeat("<", 2000)})
	if err != nil {
		t.Fatal(err)
	}
	checkedRequest(t, s, "POST", "/api/bid", a, string(body), 200)
}

func TestObserverVisionUpdatesImmediatelyAndReplayExcludesFutureSkills(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	a, _ := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	purchaseJoinCards(t, s, a, `{"cards":["random_shape","largest_price","item_count"]}`)
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["random_shape","largest_price","item_count"]}`, 201)
	purchaseJoinCards(t, s, b, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	var own game.View
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state", a, "", 200), &own)
	id := own.Self.ID
	for round, card := range []string{"random_shape", "largest_price", "item_count"} {
		round++
		body, _ := json.Marshal(map[string]any{"round": round, "card": card, "reason": "inspect current knowledge"})
		checkedRequest(t, s, "POST", "/api/card", a, string(body), 200)
		var latest struct {
			game.View
			Frames []game.View `json:"frames"`
		}
		_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/replay?room=table", s.ObserverToken, "", 200), &latest)
		if len(latest.ItemReveals[id]) != min(round, 2) {
			t.Fatal("live vision did not follow skill use")
		}
		if len(latest.Frames[0].ItemReveals[id]) != 0 {
			t.Fatal("opening includes future skill")
		}
		if round > 1 && len(latest.Frames[1].ItemReveals[id]) != 1 {
			t.Fatal("first round includes later skill")
		}
		first := latest.ItemReveals[id][0]
		if first.TopLeft == nil || first.BottomRight == nil || first.Quality != "" || first.Price != nil {
			t.Fatal("shape scan exposed extra attributes")
		}
		if round > 1 {
			price := latest.ItemReveals[id][1]
			if price.Price == nil || price.TopLeft == nil || price.BottomRight == nil || price.Quality != "" {
				t.Fatal("price skill failed to locate target or disclosed quality")
			}
		}
		for _, path := range []string{"/api/state", "/api/replay?room=table"} {
			for _, token := range []string{"", a, b} {
				var public game.View
				_ = json.Unmarshal(checkedRequest(t, s, "GET", path, token, "", 200), &public)
				if len(public.ItemReveals) != 0 || public.Self != nil && token == b && len(public.Self.ItemReveals) != 0 {
					t.Fatal("observer vision leaked through public/player route")
				}
			}
		}
		bid, _ := json.Marshal(map[string]any{"round": round, "amount": 100, "reason": "protocol sample"})
		checkedRequest(t, s, "POST", "/api/bid", a, string(bid), 200)
		checkedRequest(t, s, "POST", "/api/bid", b, string(bid), 200)
	}
	var frame game.View
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/replay?room=table&frame=2", s.ObserverToken, "", 200), &frame)
	if len(frame.ItemReveals[id]) != 2 {
		t.Fatal("recorded vision is incomplete")
	}
}

func TestObserverReasonsAreAuthenticatedReadOnlyAndDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, credential := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	purchaseJoinCards(t, s, a, `{"cards":["item_count"]}`)
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["item_count"]}`, 201)
	purchaseJoinCards(t, s, b, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	pubCh, obsCh := make(chan roomEvent, 4), make(chan roomEvent, 4)
	s.subscribers[pubCh] = struct{}{}
	s.observerSubscribers[obsCh] = struct{}{}
	before := checkedRequest(t, s, "GET", "/api/state", "", "", 200)
	checkedRequest(t, s, "POST", "/api/card", a, `{"round":1,"card":"item_count","reason":"private-count-purpose"}`, 200)
	if len(pubCh) != 0 || len(obsCh) < 1 || (<-obsCh).Kind != "observer_changed" {
		t.Fatal("private card generated public event or no observer event")
	}
	if !bytes.Equal(before, checkedRequest(t, s, "GET", "/api/state", "", "", 200)) {
		t.Fatal("public private-action disclosure")
	}
	checkedRequest(t, s, "POST", "/api/bid", a, `{"round":1,"amount":100,"reason":"private-bid-rationale"}`, 200)
	for _, token := range []string{"", a, b, s.HostToken} {
		checkedRequest(t, s, "GET", "/api/observer/state", token, "", 401)
		checkedRequest(t, s, "GET", "/api/observer/events", token, "", 401)
	}
	for _, token := range []string{"", b, s.HostToken} {
		data := checkedRequest(t, s, "GET", "/api/state", token, "", 200)
		if bytes.Contains(data, []byte("private-count-purpose")) || bytes.Contains(data, []byte("private-bid-rationale")) || bytes.Contains(data, []byte(`"card_id"`)) {
			t.Fatal("other player/public sees reasons")
		}
	}
	observer := checkedRequest(t, s, "GET", "/api/observer/state", s.ObserverToken, "", 200)
	var v struct {
		Decisions []any `json:"decisions"`
	}
	_ = json.Unmarshal(observer, &v)
	if len(v.Decisions) != 2 {
		t.Fatal("missing observer decisions")
	}
	var observed game.View
	if err := json.Unmarshal(observer, &observed); err != nil || observed.Decisions[0].CardID != "item_count" || observed.Decisions[1].CardID != "" || observed.Self != nil {
		t.Fatal("observer card identity missing or private skill results exposed")
	}
	if len(observed.InitialInspections) != 2 {
		t.Fatal("human observer is missing initial inspections")
	}
	for _, path := range []string{"/api/state?room=table", "/api/replay?room=table"} {
		public := checkedRequest(t, s, "GET", path, a, "", 200)
		if bytes.Contains(public, []byte(`"initial_inspections"`)) {
			t.Fatal("public route exposes other players' inspections")
		}
	}
	checkedRequest(t, s, "POST", "/api/bid", s.ObserverToken, `{"round":1,"amount":100,"reason":"unauthorized"}`, 401)
	for _, body := range []string{`{"round":1,"amount":100}`, `{"round":1,"amount":100,"reason":" "}`, `{"round":1,"card":"item_count"}`} {
		endpoint := "/api/bid"
		if bytes.Contains([]byte(body), []byte("card")) {
			endpoint = "/api/card"
		}
		checkedRequest(t, s, "POST", endpoint, b, body, 400)
	}
	checkedRequest(t, s, "POST", "/api/bid", b, `{"round":1,"amount":200,"reason":"private-b-bid"}`, 200)
	observer = checkedRequest(t, s, "GET", "/api/observer/state", s.ObserverToken, "", 200)
	var settled game.View
	if err := json.Unmarshal(observer, &settled); err != nil || settled.Phase != "settled" || len(settled.Decisions) != 3 {
		t.Fatal("settlement lost decision reasons")
	}
	_ = s.Close()
	s = persistentServer(t, path)
	if restored := checkedRequest(t, s, "GET", "/api/observer/state", s.ObserverToken, "", 200); !bytes.Equal(observer, restored) {
		t.Fatal("observer records changed across restart")
	}
	a = loginAgain(t, s, credential)
	self := checkedRequest(t, s, "GET", "/api/state", a, "", 200)
	if !bytes.Contains(self, []byte("private-bid-rationale")) {
		t.Fatal("self decisions did not recover")
	}
}

func TestObserverCookieSession(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	w := httptest.NewRecorder()
	a, _ := accountSession(t, s, "A")
	for _, credential := range []string{"", a, s.ObserverToken} {
		checkedRequest(t, s, "POST", "/api/observer/launch", credential, "", 403)
	}
	var launch struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(checkedRequest(t, s, "POST", "/api/observer/launch", s.HostToken, "", 200), &launch); err != nil || launch.Code == "" || launch.Code == s.ObserverToken {
		t.Fatal("missing independent launch code")
	}
	checkedRequest(t, s, "POST", "/api/observer/bootstrap", "", `{"code":"invalid"}`, 401)
	r := httptest.NewRequest("POST", "/api/observer/bootstrap", bytes.NewBufferString(`{"code":"`+launch.Code+`"}`))
	s.ServeHTTP(w, r)
	if w.Code != 200 || len(w.Result().Cookies()) != 2 {
		t.Fatal("automatic session bootstrap failed")
	}
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || c.Path != "/api/observer" || c.SameSite != http.SameSiteStrictMode {
		t.Fatal("observer cookie attributes")
	}
	r = httptest.NewRequest("GET", "/api/observer/state?room=table", nil)
	r.AddCookie(c)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("cookie authentication failed")
	}
	r = httptest.NewRequest("POST", "/api/start?room=table", nil)
	r.AddCookie(c)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("read-only cookie obtained host permissions")
	}
	checkedRequest(t, s, "POST", "/api/observer/bootstrap", "", `{"code":"`+launch.Code+`"}`, 401)
	expired := s.NewObserverLaunchCode()
	s.observerLaunchUntil = time.Now().Add(-time.Second)
	checkedRequest(t, s, "POST", "/api/observer/bootstrap", "", `{"code":"`+expired+`"}`, 401)
	rotated := s.NewObserverLaunchCode()
	s.NewObserverLaunchCode()
	checkedRequest(t, s, "POST", "/api/observer/bootstrap", "", `{"code":"`+rotated+`"}`, 401)
}
