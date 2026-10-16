package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

func loginAgain(t *testing.T, s *Server, credential string) string {
	t.Helper()
	code, body := request(t, s, "POST", "/api/login", "", `{"credential":"`+credential+`"}`)
	return responseStrings(t, code, body, 200)["token"]
}

func checkedRequest(t *testing.T, s *Server, method, path, token, body string, expected int) []byte {
	t.Helper()
	code, response := request(t, s, method, path, token, body)
	if code != expected {
		t.Fatalf("%s %s: HTTP %d, expected %d", method, path, code, expected)
	}
	return response
}

func TestRoomsRestorePrivateBidsReplayAndCrossRestartResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, ca := accountSession(t, s, "Atlas")
	b, cb := accountSession(t, s, "Echo")
	_, outsiderCredential := accountSession(t, s, "Observer")
	created := checkedRequest(t, s, "POST", "/api/create", a, `{"name":"Persistent table","players":2}`, 201)
	var roomInfo map[string]any
	_ = json.Unmarshal(created, &roomInfo)
	id := roomInfo["room_id"].(string)
	suffix := "?room=" + id
	checkedRequest(t, s, "POST", "/api/join"+suffix, a, `{"cards":["largest_price","item_count"]}`, 201)
	initial := checkedRequest(t, s, "GET", "/api/state"+suffix, a, "", 200)
	var lobbyView game.View
	if err := json.Unmarshal(initial, &lobbyView); err != nil {
		t.Fatal(err)
	}
	if len(lobbyView.Players) != 1 || lobbyView.Players[0].Avatar == nil {
		t.Fatal("first player did not receive an avatar before the room filled")
	}
	_ = s.Close()
	s = persistentServer(t, path)
	a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
	if restored := checkedRequest(t, s, "GET", "/api/state"+suffix, a, "", 200); !bytes.Equal(initial, restored) {
		t.Fatal("lobby membership or room metadata changed on restart")
	}
	checkedRequest(t, s, "POST", "/api/join"+suffix, a, `{"cards":[]}`, 409)
	checkedRequest(t, s, "POST", "/api/join"+suffix, b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start"+suffix, b, "", 403)
	checkedRequest(t, s, "POST", "/api/start"+suffix, a, "", 200)
	checkedRequest(t, s, "POST", "/api/card"+suffix, a, `{"reason":"protocol test","round":1,"card":"largest_price"}`, 200)
	private := checkedRequest(t, s, "POST", "/api/bid"+suffix, a, `{"reason":"protocol test","round":1,"amount":77777}`, 200)
	public := checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200)
	var avatarView game.View
	if err := json.Unmarshal(public, &avatarView); err != nil {
		t.Fatal(err)
	}
	if avatarView.Players[0].Avatar == nil || avatarView.Players[1].Avatar == nil || *avatarView.Players[0].Avatar == *avatarView.Players[1].Avatar {
		t.Fatal("public snapshot is missing distinct avatars")
	}
	if *avatarView.Players[0].Avatar != *lobbyView.Players[0].Avatar {
		t.Fatal("later join or start changed the first player's avatar")
	}
	for _, forbidden := range []string{`"self":`, `"amount":77777`, `"decisions":`, `"reason":"protocol test"`, `"settlement":`} {
		if bytes.Contains(public, []byte(forbidden)) {
			t.Fatalf("public state leaked sealed or private data: %s", forbidden)
		}
	}
	_ = s.Close()
	s = persistentServer(t, path)
	a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
	for token, expected := range map[string][]byte{a: private, "": public, s.HostToken: public, loginAgain(t, s, outsiderCredential): public} {
		if restored := checkedRequest(t, s, "GET", "/api/state"+suffix, token, "", 200); !bytes.Equal(expected, restored) {
			t.Fatal("restored private/public observation changed")
		}
	}
	checkedRequest(t, s, "POST", "/api/bid"+suffix, a, `{"reason":"protocol test","round":1,"amount":1}`, 409)
	checkedRequest(t, s, "POST", "/api/card"+suffix, a, `{"reason":"protocol test","round":1,"card":"largest_price"}`, 409)
	checkedRequest(t, s, "POST", "/api/bid"+suffix, b, `{"reason":"protocol test","round":1,"amount":60000}`, 200)
	for round := 2; round <= game.Rounds; round++ {
		for _, token := range []string{a, b} {
			checkedRequest(t, s, "POST", "/api/bid"+suffix, token, fmt.Sprintf(`{"reason":"protocol test","round":%d,"amount":100}`, round), 200)
		}
	}
	settled := checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200)
	replay := checkedRequest(t, s, "GET", "/api/replay"+suffix, "", "", 200)
	var replayView struct {
		Frames []game.View `json:"frames"`
	}
	if err := json.Unmarshal(replay, &replayView); err != nil {
		t.Fatal(err)
	}
	for _, frame := range replayView.Frames {
		for i, p := range frame.Players {
			if p.Avatar == nil || *p.Avatar != *avatarView.Players[i].Avatar {
				t.Fatal("replay changed player avatar")
			}
		}
	}
	results := checkedRequest(t, s, "GET", "/api/results", "", "", 200)
	for range 2 {
		_ = s.Close()
		s = persistentServer(t, path)
		if after := checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200); !bytes.Equal(settled, after) {
			t.Fatal("settlement, tie winner, box or replay changed after restart")
		}
		if after := checkedRequest(t, s, "GET", "/api/results", "", "", 200); !bytes.Equal(results, after) {
			t.Fatal("restart changed historical results")
		}
	}
	a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
	checkedRequest(t, s, "POST", "/api/bid"+suffix, a, `{"reason":"protocol test","round":5,"amount":100}`, 409)
	// A second game after restart reverses seats; both histories must associate
	// the same agents with their permanent account IDs rather than p1/p2.
	for _, token := range []string{b, a} {
		checkedRequest(t, s, "POST", "/api/join", token, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	for round := 1; round <= game.Rounds; round++ {
		for _, token := range []string{a, b} {
			checkedRequest(t, s, "POST", "/api/bid", token, fmt.Sprintf(`{"reason":"protocol test","round":%d,"amount":101}`, round), 200)
		}
	}
	_ = s.Close()
	s = persistentServer(t, path)
	results = checkedRequest(t, s, "GET", "/api/results", "", "", 200)
	var summaries struct {
		Rooms []struct {
			Players []struct {
				Name    string
				AgentID string `json:"agent_id"`
			} `json:"players"`
		} `json:"rooms"`
	}
	if json.Unmarshal(results, &summaries) != nil || len(summaries.Rooms) != 2 {
		t.Fatal("cross-restart result history lost or duplicated")
	}
	identities := map[string]string{}
	for _, rm := range summaries.Rooms {
		for _, p := range rm.Players {
			if prior := identities[p.Name]; prior != "" && prior != p.AgentID {
				t.Fatal("cross-restart statistics split one agent into multiple identities")
			}
			identities[p.Name] = p.AgentID
		}
	}
	if identities["Atlas"] == identities["Echo"] {
		t.Fatal("statistics merged separate agents")
	}
}

func TestFailedRoomWritesAreNotAcknowledgedOrPublished(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, ca := accountSession(t, s, "A")
	b, cb := accountSession(t, s, "B")
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["item_count"]}`, 201)
	_ = s.Close()
	lobby := checkedRequest(t, s, "GET", "/api/state", "", "", 200)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 500)
	checkedRequest(t, s, "POST", "/api/create", a, `{"name":"Lost","players":2}`, 500)
	if len(s.rooms) != 1 || !bytes.Equal(lobby, checkedRequest(t, s, "GET", "/api/state", "", "", 200)) {
		t.Fatal("failed room write changed in-memory lobby")
	}
	s = persistentServer(t, path)
	a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
	checkedRequest(t, s, "POST", "/api/join", b, `{"cards":[]}`, 201)
	_ = s.Close()
	lobby = checkedRequest(t, s, "GET", "/api/state", "", "", 200)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 500)
	if !bytes.Equal(lobby, checkedRequest(t, s, "GET", "/api/state", "", "", 200)) {
		t.Fatal("failed start published a generated box")
	}
	s = persistentServer(t, path)
	a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	_ = s.Close()
	private := checkedRequest(t, s, "GET", "/api/state", a, "", 200)
	checkedRequest(t, s, "POST", "/api/card", a, `{"reason":"protocol test","round":1,"card":"item_count"}`, 500)
	checkedRequest(t, s, "POST", "/api/bid", a, `{"reason":"protocol test","round":1,"amount":1}`, 500)
	if !bytes.Equal(private, checkedRequest(t, s, "GET", "/api/state", a, "", 200)) {
		t.Fatal("failed write consumed a card or sealed a bid")
	}
	s = persistentServer(t, path)
	a, b = loginAgain(t, s, ca), loginAgain(t, s, cb)
	for round := 1; round <= game.Rounds; round++ {
		checkedRequest(t, s, "POST", "/api/bid", a, fmt.Sprintf(`{"reason":"protocol test","round":%d,"amount":1}`, round), 200)
		if round < game.Rounds {
			checkedRequest(t, s, "POST", "/api/bid", b, fmt.Sprintf(`{"reason":"protocol test","round":%d,"amount":1}`, round), 200)
		}
	}
	_ = s.Close()
	before := checkedRequest(t, s, "GET", "/api/state", "", "", 200)
	checkedRequest(t, s, "POST", "/api/bid", b, `{"reason":"protocol test","round":5,"amount":2}`, 500)
	if !bytes.Equal(before, checkedRequest(t, s, "GET", "/api/state", "", "", 200)) || !bytes.Equal([]byte("{\"rooms\":[]}\n"), checkedRequest(t, s, "GET", "/api/results", "", "", 200)) {
		t.Fatal("failed settlement was published or counted")
	}
	s = persistentServer(t, path)
	b = loginAgain(t, s, cb)
	checkedRequest(t, s, "POST", "/api/bid", b, `{"reason":"protocol test","round":5,"amount":2}`, 200)
}

func TestConcurrentBidsPersistTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	tokens := []string{}
	for _, name := range []string{"A", "B"} {
		token, _ := accountSession(t, s, name)
		tokens = append(tokens, token)
		checkedRequest(t, s, "POST", "/api/join", token, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	var wg sync.WaitGroup
	for _, token := range tokens {
		wg.Go(func() {
			checkedRequest(t, s, "POST", "/api/bid", token, `{"reason":"protocol test","round":1,"amount":123}`, 200)
		})
	}
	wg.Wait()
	_ = s.Close()
	s = persistentServer(t, path)
	var view game.View
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state", "", "", 200), &view)
	if view.Round != 2 || len(view.History) != 1 || len(view.History[0].Bids) != 2 {
		t.Fatal("concurrent writes lost a bid or round transition")
	}
}

func TestRestoredRoomsKeepRulesWhenHostDefaultsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	geometry := game.DefaultConfig()
	geometry.Box, geometry.Items = game.Dimensions{Width: 7, Height: 5}, game.ItemLimits{MaxWidth: 2, MaxHeight: 3}
	geometry.Qualities[0].Min, geometry.Qualities[0].Max = 11, 19
	geometry.InitialAssets = 7654321
	cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{"audit"}, Cards: []cards.Definition{{ID: "audit", Name: "Audit", Description: "Custom count", Effect: cards.Effect{Type: "count"}}}}
	g, err := game.NewWithConfig(2, geometry, cfg)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(g.GameConfig(), g.CardConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	rm := testTable(g).rooms["table"]
	if err := s.store.saveRoom(rm); err != nil {
		t.Fatal(err)
	}
	s.rooms["table"] = rm
	a, credential := accountSession(t, s, "A")
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":["audit"]}`, 201)
	before := checkedRequest(t, s, "GET", "/api/state", a, "", 200)
	_ = s.Close()
	g, _ = game.New(7)
	s, err = Open(g.GameConfig(), g.CardConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	a = loginAgain(t, s, credential)
	if !bytes.Equal(before, checkedRequest(t, s, "GET", "/api/state", a, "", 200)) {
		t.Fatal("changed host defaults overwrote an existing room")
	}
	created := checkedRequest(t, s, "POST", "/api/create", a, `{"name":"New defaults","players":3}`, 201)
	var roomInfo map[string]any
	_ = json.Unmarshal(created, &roomInfo)
	var view game.View
	_ = json.Unmarshal(checkedRequest(t, s, "GET", "/api/state?room="+roomInfo["room_id"].(string), "", "", 200), &view)
	if view.Rules.PlayerCount != 3 || !reflect.DeepEqual(view.Rules.GameConfig, game.DefaultConfig()) || view.Rules.Cards[0].ID == "audit" {
		t.Fatal("new rooms did not inherit new host defaults")
	}
}

func TestInvalidRoomRecordDoesNotResetHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	_ = s.Close()
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		var rec roomRecord
		if err := json.Unmarshal(tx.Bucket(roomBucket).Get([]byte("table")), &rec); err != nil {
			return err
		}
		rec.Game = json.RawMessage(`{"schema_version":99}`)
		bad, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket(roomBucket).Put([]byte("table"), bad)
	})
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	g, _ := game.New(2)
	if restored, err := Open(g.GameConfig(), g.CardConfig(), path); err == nil {
		_ = restored.Close()
		t.Fatal("invalid room record silently reset the database")
	}
}
