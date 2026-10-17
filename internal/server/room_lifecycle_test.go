package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

func lifecycleRequest(t *testing.T, s *Server, token, action string, ids []string, expected int) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"action": action, "room_ids": ids})
	if err != nil {
		t.Fatal(err)
	}
	checkedRequest(t, s, "POST", "/api/rooms/manage", token, string(body), expected)
}

func lifecycleRoom(t *testing.T, s *Server, token, name string) string {
	t.Helper()
	var room struct {
		ID string `json:"room_id"`
	}
	data := checkedRequest(t, s, "POST", "/api/create", token, fmt.Sprintf(`{"name":%q,"players":2}`, name), 201)
	if err := json.Unmarshal(data, &room); err != nil {
		t.Fatal(err)
	}
	return room.ID
}

func TestRoomManagementPermissionsAndAtomicSelection(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	a, _ := accountSession(t, s, "Owner")
	b, _ := accountSession(t, s, "Visitor")
	id := lifecycleRoom(t, s, a, "Occupied lobby")
	other := lifecycleRoom(t, s, b, "Other owner")
	suffix := "?room=" + id
	purchaseJoinCards(t, s, a, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join"+suffix, a, `{"cards":[]}`, 201)
	for _, token := range []string{"", s.ObserverToken} {
		lifecycleRequest(t, s, token, "delete", []string{id}, 401)
	}
	lifecycleRequest(t, s, b, "delete", []string{id}, 403)
	lifecycleRequest(t, s, a, "archive", []string{id, other}, 403)
	lifecycleRequest(t, s, s.HostToken, "archive", []string{id, "missing"}, 404)
	lifecycleRequest(t, s, a, "archive", []string{id, id}, 400)
	if !s.rooms[id].archivedAt.IsZero() {
		t.Fatal("invalid batch partly archived a room")
	}
	events := make(chan roomEvent, 10)
	s.subscribers[events] = struct{}{}
	lifecycleRequest(t, s, a, "archive", []string{id}, 200)
	if event := <-events; event.Kind != "room_archived" || !event.Rooms || event.Results {
		t.Fatalf("invalid event: %+v", event)
	}
	if !bytes.Contains(checkedRequest(t, s, "GET", "/api/rooms", "", "", 200), []byte("archived_at")) {
		t.Fatal("missing archive metadata")
	}
	checkedRequest(t, s, "POST", "/api/join"+suffix, b, `{"cards":[]}`, 409)
	checkedRequest(t, s, "POST", "/api/start"+suffix, a, "", 409)
	checkedRequest(t, s, "POST", "/api/invite"+suffix, a, "", 409)
	lifecycleRequest(t, s, a, "restore", []string{id}, 200)
	purchaseJoinCards(t, s, b, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join"+suffix, b, `{"cards":[]}`, 201)
	checkedRequest(t, s, "POST", "/api/start"+suffix, a, "", 200)
	for _, action := range []string{"archive", "restore", "delete"} {
		lifecycleRequest(t, s, s.HostToken, action, []string{other, id}, 409)
	}
	if !s.rooms[other].archivedAt.IsZero() {
		t.Fatal("ongoing room batch partly applied")
	}
	lifecycleRequest(t, s, s.HostToken, "delete", []string{other}, 200)
}

func TestDeletedSettlementKeepsOnlyEconomicSummaryAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, ca := accountSession(t, s, "Atlas")
	b, _ := accountSession(t, s, "Echo")
	id := lifecycleRoom(t, s, a, "Completed room")
	lobby := lifecycleRoom(t, s, a, "Joined waiting room")
	for _, token := range []string{a, b} {
		purchaseJoinCards(t, s, token, `{"cards":[]}`)
		checkedRequest(t, s, "POST", "/api/join?room="+id, token, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "POST", "/api/start?room="+id, a, "", 200)
	// Deterministic protocol fixture, not an Agent game acceptance run.
	for round := 1; round <= game.Rounds; round++ {
		for _, token := range []string{a, b} {
			checkedRequest(t, s, "POST", "/api/bid?room="+id, token, fmt.Sprintf(`{"round":%d,"amount":10,"reason":"private protocol fixture"}`, round), 200)
		}
	}
	purchaseJoinCards(t, s, a, `{"cards":[]}`)
	checkedRequest(t, s, "POST", "/api/join?room="+lobby, a, `{"cards":[]}`, 201)
	before := checkedRequest(t, s, "GET", "/api/stats", "", "", 200)
	results := checkedRequest(t, s, "GET", "/api/results", "", "", 200)
	lifecycleRequest(t, s, a, "archive", []string{id, lobby}, 200)
	archived := checkedRequest(t, s, "GET", "/api/rooms", "", "", 200)
	_ = s.Close()
	s = persistentServer(t, path)
	a = loginAgain(t, s, ca)
	if !bytes.Equal(archived, checkedRequest(t, s, "GET", "/api/rooms", "", "", 200)) {
		t.Fatal("archives not restored")
	}
	lifecycleRequest(t, s, a, "restore", []string{id, lobby}, 200)
	if !s.rooms[id].archivedAt.IsZero() || !s.rooms[lobby].archivedAt.IsZero() {
		t.Fatal("batch restore failed")
	}
	lifecycleRequest(t, s, a, "archive", []string{id, lobby}, 200)
	stale := s.rooms[lobby]
	lifecycleRequest(t, s, a, "delete", []string{id, lobby}, 200)
	for _, rid := range []string{id, lobby} {
		for _, route := range []string{"/api/state", "/api/replay", "/api/observer/replay"} {
			checkedRequest(t, s, "GET", route+"?room="+rid, s.ObserverToken, "", 404)
		}
	}
	recorder := httptest.NewRecorder()
	s.mu.Lock()
	applied := s.mutateRoom(recorder, stale, func(*room) error { t.Fatal("deleted room mutation ran"); return nil })
	s.mu.Unlock()
	if applied || recorder.Code != 404 {
		t.Fatal("stale request resurrected deleted room")
	}
	err := s.store.db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(roomBucket).Get([]byte(id)) != nil || tx.Bucket(roomBucket).Get([]byte(lobby)) != nil {
			t.Fatal("deleted snapshot persisted")
		}
		data := tx.Bucket(deletedResultBucket).Get([]byte(id))
		if len(data) == 0 || tx.Bucket(deletedResultBucket).Get([]byte(lobby)) != nil {
			t.Fatal("invalid retained results")
		}
		for _, forbidden := range []string{"private protocol fixture", `"game"`, `"items"`, `"decisions"`, `"members"`, `"credential"`} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Fatalf("retained private data: %s", forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, checkedRequest(t, s, "GET", "/api/stats", "", "", 200)) {
		t.Fatal("deletion changed statistics")
	}
	_ = s.Close()
	s = persistentServer(t, path)
	if !bytes.Equal(before, checkedRequest(t, s, "GET", "/api/stats", "", "", 200)) || !bytes.Equal(results, checkedRequest(t, s, "GET", "/api/results", "", "", 200)) {
		t.Fatal("retained statistics changed after restart")
	}
	if s.rooms[id] != nil || s.rooms[lobby] != nil {
		t.Fatal("deleted rooms restored")
	}
}

func TestFailedManagementWritesLeaveRoomsAndEventsIntact(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	id := lifecycleRoom(t, s, s.HostToken, "Room")
	events := make(chan roomEvent, 10)
	// Close only persistence so failed mutations can still be checked for events.
	_ = s.store.db.Close()
	s.subscribers[events] = struct{}{}
	for _, action := range []string{"archive", "restore", "delete"} {
		lifecycleRequest(t, s, s.HostToken, action, []string{id}, 500)
		if s.rooms[id] == nil || !s.rooms[id].archivedAt.IsZero() || len(events) != 0 || len(s.deletedResults) != 0 {
			t.Fatal("failed write changed live state")
		}
	}
}
