package server

import (
	"bidbrawl/internal/game"
	"encoding/json"
	"fmt"
	"testing"
)

func TestResultsOnlySettledAndStableIdentity(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	sessions := []string{}
	for _, name := range []string{"Atlas", "Echo"} {
		token, _ := accountSession(t, s, name)
		sessions = append(sessions, token)
	}
	finish := func(roomID string, reversed bool) {
		t.Helper()
		order := sessions
		if reversed {
			order = []string{sessions[1], sessions[0]}
		}
		for _, token := range order {
			if code, body := request(t, s, "POST", "/api/join?room="+roomID, token, `{"cards":[]}`); code != 201 {
				t.Fatalf("join: %d %s", code, body)
			}
		}
		if code, body := request(t, s, "POST", "/api/start?room="+roomID, s.HostToken, ""); code != 200 {
			t.Fatalf("start: %d %s", code, body)
		}
		for round := 1; round <= game.Rounds; round++ {
			for _, token := range sessions {
				if code, body := request(t, s, "POST", "/api/bid?room="+roomID, token, fmt.Sprintf(`{"reason":"protocol test","round":%d,"amount":100}`, round)); code != 200 {
					t.Fatalf("bid: %d %s", code, body)
				}
			}
			_, body := request(t, s, "GET", "/api/results", "", "")
			var results struct {
				Rooms []json.RawMessage `json:"rooms"`
			}
			if err := json.Unmarshal(body, &results); err != nil {
				t.Fatal(err)
			}
			if roomID == "table" && round < game.Rounds && len(results.Rooms) != 0 {
				t.Fatal("unsettled game leaked into results")
			}
		}
	}
	finish("table", false)
	_, body := request(t, s, "POST", "/api/create", sessions[0], `{"name":"Second","players":2}`)
	var created map[string]any
	_ = json.Unmarshal(body, &created)
	finish(created["room_id"].(string), true)
	code, body := request(t, s, "GET", "/api/results", "", "")
	if code != 200 {
		t.Fatal(code)
	}
	var results struct {
		Rooms []struct {
			Players []struct {
				ID, Name string
				AgentID  string `json:"agent_id"`
			} `json:"players"`
			Settlement map[string]json.RawMessage `json:"settlement"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		t.Fatal(err)
	}
	if len(results.Rooms) != 2 {
		t.Fatalf("got %d settled rooms", len(results.Rooms))
	}
	identities := map[string]string{}
	for _, room := range results.Rooms {
		if len(room.Settlement) != 7 || room.Settlement["items"] != nil || room.Settlement["decisions"] != nil {
			t.Fatal("summary must contain economics and settlement stage only")
		}
		for _, player := range room.Players {
			if prior := identities[player.Name]; prior != "" && prior != player.AgentID {
				t.Fatal("account identity changed with seat")
			}
			identities[player.Name] = player.AgentID
		}
	}
	if identities["Atlas"] == identities["Echo"] {
		t.Fatal("different accounts merged")
	}
}

func TestEmbeddedWebUIAssets(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	for _, path := range []string{"/", "/app.js", "/style.css", "/i18n.js", "/render.js", "/room-ui.js", "/avatars.js", "/appearance.js", "/theme.js", "/html.js", "/icons.svg", "/model.js", "/locales/zh-CN.js", "/locales/en.js", "/fonts/fonts.css", "/icons/lucide/boxes.svg", "/fonts/zcool-kuaile/LICENSE", "/fonts/space-grotesk/LICENSE"} {
		if code, body := request(t, s, "GET", path, "", ""); code != 200 || len(body) == 0 {
			t.Fatalf("embedded asset %s: %d", path, code)
		}
	}
}
