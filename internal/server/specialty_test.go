package server

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSpecialtyActionIsPrivateDurableAndTransactional(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, credential := accountSession(t, s, "Quick")
	b, _ := accountSession(t, s, "Survey")
	checkedRequest(t, s, "POST", "/api/join", a, `{"specialty":"quick","cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/join", b, `{"specialty":"survey","cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	public := checkedRequest(t, s, "GET", "/api/state", "", "", 200)
	other := checkedRequest(t, s, "GET", "/api/state", b, "", 200)
	pubCh, obsCh := make(chan roomEvent, 8), make(chan roomEvent, 32)
	s.subscribers[pubCh] = struct{}{}
	s.observerSubscribers[obsCh] = struct{}{}
	checkedRequest(t, s, "POST", "/api/specialty?room=table", s.ObserverToken, `{"round":1,"action":"take","reason":"observer cannot play"}`, 401)
	checkedRequest(t, s, "POST", "/api/specialty?room=table", a, `{"round":1,"action":"take","reason":"use early information before choosing an offer"}`, 200)
	if len(pubCh) != 0 || len(obsCh) == 0 {
		t.Fatal("specialty action published publicly or missed observer event")
	}
	if !bytes.Equal(public, checkedRequest(t, s, "GET", "/api/state", "", "", 200)) || !reflect.DeepEqual(readGameView(t, other), readGameView(t, checkedRequest(t, s, "GET", "/api/state", b, "", 200))) {
		t.Fatal("private choice changed another observation")
	}
	raw, _ := s.rooms["table"].game.Observe(s.rooms["table"].members[s.sessions[a].ID])
	own := readGameView(t, raw)
	if len(own.Self.Intelligence) != 2 || len(own.Self.SpecialtyState.Choices) != 1 {
		t.Fatal("quick action did not persist both reports")
	}
	count := 0
	for _, e := range s.rooms["table"].activities {
		if e.Action == "specialty" && e.PlayerID == "p1" {
			count++
		}
	}
	if count != 2 {
		t.Fatal("second report disappeared from the private activity record", count)
	}
	before, _ := s.rooms["table"].game.Snapshot()
	checkedRequest(t, s, "POST", "/api/specialty?room=table", a, `{"round":1,"action":"take","reason":"retry must not grant new samples"}`, 409)
	after, _ := s.rooms["table"].game.Snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("rejected repeat changed private progress")
	}
	s.Close()
	s = persistentServer(t, path)
	var login map[string]any
	data := checkedRequest(t, s, "POST", "/api/login", "", `{"credential":"`+credential+`"}`, 200)
	json.Unmarshal(data, &login)
	token := login["token"].(string)
	restored := readGameView(t, checkedRequest(t, s, "GET", "/api/state", token, "", 200))
	if len(restored.Self.Intelligence) != 2 || len(restored.Self.SpecialtyState.Actions) != 0 {
		t.Fatal("restart repeated specialty allowance")
	}
	for _, field := range []string{`"specialty_states"`, `"specialty_state"`, `"samples"`, `"choices"`, "use early information"} {
		if bytes.Contains(checkedRequest(t, s, "GET", "/api/replay?room=table", "", "", 200), []byte(field)) {
			t.Fatal("public replay leaked private progress", field)
		}
	}
}
