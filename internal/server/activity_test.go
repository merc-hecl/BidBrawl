package server

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

// Gameplay comparisons exclude the read's own audit entry; audit durability and
// isolation are checked separately below.
func sameGameProjection(a, b []byte) bool {
	clean := func(raw []byte) any {
		var v map[string]any
		if json.Unmarshal(raw, &v) != nil {
			return nil
		}
		delete(v, "activities")
		if self, ok := v["self"].(map[string]any); ok {
			delete(self, "activities")
		}
		return v
	}
	return reflect.DeepEqual(clean(a), clean(b))
}

func TestActivitiesAreDurablePrivateAndReplayBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, credential := accountSession(t, s, "Reader")
	b, _ := accountSession(t, s, "Other")
	for _, tok := range []string{a, b} {
		checkedRequest(t, s, "POST", "/api/join", tok, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "POST", "/api/activity", a, `{"command":"handbook","topic":"rules","lang":"zh-CN"}`, 200)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	pubCh := make(chan roomEvent, 20)
	s.subscribers[pubCh] = struct{}{}
	checkedRequest(t, s, "GET", "/api/state", a, "", 200)
	checkedRequest(t, s, "POST", "/api/bid", a, `{"round":1,"amount":0,"reason":"rejected zero"}`, 409)
	if len(pubCh) != 0 {
		t.Fatal("read or rejected request changed public SSE")
	}
	checkedRequest(t, s, "POST", "/api/bid", a, `{"round":1,"amount":100,"reason":"private independent estimate"}`, 200)
	for _, tok := range []string{"", b, s.HostToken} {
		raw := checkedRequest(t, s, "GET", "/api/state", tok, "", 200)
		if bytes.Contains(raw, []byte("private independent estimate")) || bytes.Contains(raw, []byte("rejected zero")) || tok == "" && bytes.Contains(raw, []byte(`"activities"`)) {
			t.Fatal("private activity leaked")
		}
	}
	var v struct {
		Activities []activity `json:"activities"`
	}
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/state", s.ObserverToken, "", 200), &v)
	seen := map[string]bool{}
	for _, e := range v.Activities {
		seen[e.Action+":"+e.Status+":"+e.Source] = true
		if e.AccountID != "" {
			t.Fatal("account storage identifier escaped activity projection")
		}
	}
	for _, key := range []string{"handbook:reported:cli", "state:success:server", "bid:rejected:server", "bid:success:server", "specialty:success:system", "ticket:success:system", "public_hint:success:system"} {
		if !seen[key] {
			t.Fatal("missing activity", key)
		}
	}
	var opening struct {
		Activities []activity `json:"activities"`
	}
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/replay?room=table&frame=0", s.ObserverToken, "", 200), &opening)
	for _, e := range opening.Activities {
		if e.Round > 0 {
			t.Fatal("future call leaked into opening")
		}
	}
	prior := append([]activity{}, v.Activities...)
	_ = s.Close()
	s = persistentServer(t, path)
	a = loginAgain(t, s, credential)
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/observer/state", s.ObserverToken, "", 200), &v)
	if len(v.Activities) < len(prior) || !reflect.DeepEqual(prior, v.Activities[:len(prior)]) {
		t.Fatal("restart lost or rewrote private activity")
	}
	checkedRequest(t, s, "GET", "/api/state", a, "", 200)
}

func TestPlayerActivityIDsDoNotRevealOtherPlayersCallCounts(t *testing.T) {
	rm := &room{seatAccounts: map[string]string{"p1": "a", "p2": "b"}, activities: []activity{
		{ID: 1, AccountID: "a", Round: 1, Action: "state"},
		{ID: 2, AccountID: "b", Round: 1, Action: "card"},
		{ID: 3, AccountID: "b", Round: 1, Action: "state"},
		{ID: 4, AccountID: "a", Round: 1, Action: "bid"},
	}}
	own := activityView(rm, "p1", 1)
	if len(own) != 2 || own[0].ID != 1 || own[1].ID != 2 {
		t.Fatal("private call count leaked through sequence gaps")
	}
	observer := activityView(rm, "", 1)
	if len(observer) != 4 || observer[3].ID != 4 {
		t.Fatal("observer lost canonical action identity")
	}
}
