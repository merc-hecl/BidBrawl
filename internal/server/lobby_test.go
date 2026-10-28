package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
)

func TestHumanAndAgentRoomInvitationPermissions(t *testing.T) {
	s := New(game.DefaultConfig(), cards.Default())
	s.EnableWebUI()
	code := s.NewObserverLaunchCode()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/observer/bootstrap", bytes.NewBufferString(`{"code":"`+code+`"}`)))
	if w.Code != 200 {
		t.Fatal("bootstrap failed")
	}
	cookies := w.Result().Cookies()
	human := func(method, path, body string, expected int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != expected {
			t.Fatalf("human %s: %d", path, w.Code)
		}
		return w.Body.Bytes()
	}
	checkedRequest(t, s, "POST", "/api/create", "", `{"name":"Denied","players":2}`, 401)
	checkedRequest(t, s, "POST", "/api/create", s.ObserverToken, `{"name":"Read only","players":2}`, 401)
	var created struct {
		ID     string `json:"room_id"`
		Invite string `json:"invite_path"`
	}
	if err := json.Unmarshal(human("POST", "/api/create", `{"name":"Human table","players":2}`, 201), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Invite != "/invite?room="+created.ID || s.rooms[created.ID].owner != "" {
		t.Fatal("invalid human room")
	}
	suffix := "?room=" + created.ID
	human("POST", "/api/invite"+suffix, "", 200)
	human("POST", "/api/start"+suffix, "", 409)
	a, _ := accountSession(t, s, "A")
	b, _ := accountSession(t, s, "B")
	purchaseJoinCards(t, s, a, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join"+suffix, a, `{"cards":[]}`, 201)
	purchaseJoinCards(t, s, b, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join"+suffix, b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start"+suffix, a, "", 403)
	human("POST", "/api/start"+suffix, "", 200)
	human("POST", "/api/invite"+suffix, "", 409)
	checkedRequest(t, s, "POST", "/api/bid"+suffix, s.UIToken, `{"round":1,"amount":1,"reason":"cannot act as player"}`, 401)
	human("GET", "/api/accounts", "", 403)
	var agentRoom struct {
		ID string `json:"room_id"`
	}
	_ = json.Unmarshal(checkedRequest(t, s, "POST", "/api/create", a, `{"name":"Agent table","players":2}`, 201), &agentRoom)
	checkedRequest(t, s, "POST", "/api/invite?room="+agentRoom.ID, b, "", 403)
	checkedRequest(t, s, "POST", "/api/invite?room="+agentRoom.ID, a, "", 200)
	checkedRequest(t, s, "GET", "/api/observer/state?room="+agentRoom.ID, a, "", 401)
}
