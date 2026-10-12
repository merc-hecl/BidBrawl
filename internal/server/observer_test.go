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
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	// Go's JSON encoder expands HTML characters to six-byte escapes.
	body, err := json.Marshal(map[string]any{"round": 1, "amount": 100, "reason": strings.Repeat("<", 2000)})
	if err != nil {
		t.Fatal(err)
	}
	checkedRequest(t, s, "POST", "/api/bid", a, string(body), 200)
}

func TestObserverReasonsAreAuthenticatedReadOnlyAndDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, credential := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["item_count"]}`, 201)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	pubCh, obsCh := make(chan roomEvent, 4), make(chan roomEvent, 4)
	s.subscribers[pubCh] = struct{}{}
	s.observerSubscribers[obsCh] = struct{}{}
	before := checkedRequest(t, s, "GET", "/api/state", "", "", 200)
	checkedRequest(t, s, "POST", "/api/card", a, `{"round":1,"card":"item_count","reason":"private-count-purpose"}`, 200)
	if len(pubCh) != 0 || len(obsCh) != 1 || (<-obsCh).Kind != "observer_changed" {
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
