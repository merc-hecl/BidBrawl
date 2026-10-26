package server

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

func deadlineTable(t *testing.T, s *Server) ([]string, string) {
	t.Helper()
	tokens := []string{}
	for _, name := range []string{"Deadline A", "Deadline B", "Deadline C"} {
		token, _ := accountSession(t, s, name)
		tokens = append(tokens, token)
	}
	data := checkedRequest(t, s, "POST", "/api/create", tokens[0], `{"name":"Timed auction","players":3}`, 201)
	var roomInfo struct {
		ID string `json:"room_id"`
	}
	if err := json.Unmarshal(data, &roomInfo); err != nil {
		t.Fatal(err)
	}
	suffix := "?room=" + roomInfo.ID
	for _, token := range tokens {
		checkedRequest(t, s, "POST", "/api/join"+suffix, token, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "POST", "/api/start"+suffix, tokens[0], "", 200)
	for _, token := range tokens[:2] {
		checkedRequest(t, s, "POST", "/api/bid"+suffix, token, `{"round":1,"amount":100,"reason":"equal estimate"}`, 200)
	}
	return tokens, suffix
}

func TestDeadlineAdvancesWithoutRequestsAndPublishesPublicInvalidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New(game.DefaultConfig(), cards.Default())
		defer s.Close()
		tokens, suffix := deadlineTable(t, s)
		events := make(chan roomEvent, 8)
		s.mu.Lock()
		s.subscribers[events] = struct{}{}
		s.mu.Unlock()
		time.Sleep(game.RoundTimeout - time.Nanosecond)
		v := readGameView(t, checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200))
		if v.Round != 1 || len(v.History) != 0 {
			t.Fatal("round expired early")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		select {
		case e := <-events:
			if e.Kind != "round_revealed" {
				t.Fatalf("wrong expiry event: %+v", e)
			}
		default:
			t.Fatal("no round event without an HTTP request")
		}
		public := checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200)
		v = readGameView(t, public)
		if v.Round != 2 || v.History[0].Bids[2].Amount != 0 || v.Players[2].Withdrawn {
			t.Fatal("timeout became a withdrawal")
		}
		if bytes.Contains(public, []byte(`"reason"`)) || bytes.Contains(public, []byte(`"action":"timeout"`)) {
			t.Fatal("public state leaked timeout reasons")
		}
		own := checkedRequest(t, s, "GET", "/api/state"+suffix, tokens[2], "", 200)
		if !bytes.Contains(own, []byte(`"action":"timeout"`)) {
			t.Fatal("missing private timeout activity")
		}
		checkedRequest(t, s, "POST", "/api/bid"+suffix, tokens[2], `{"round":1,"amount":90,"reason":"late old round"}`, 409)
		checkedRequest(t, s, "POST", "/api/withdraw"+suffix, tokens[2], `{"round":2,"reason":"I choose to exit"}`, 200)
		for _, token := range tokens[:2] {
			checkedRequest(t, s, "POST", "/api/bid"+suffix, token, `{"round":2,"amount":100,"reason":"same estimate"}`, 200)
		}
		for frame, want := range []bool{false, false, true} {
			data := checkedRequest(t, s, "GET", "/api/replay"+suffix+"&frame="+strconv.Itoa(frame), "", "", 200)
			v := readGameView(t, data)
			if v.Players[2].Withdrawn != want || !v.RoundDeadline.IsZero() {
				t.Fatal("replay inferred withdrawal from zero or leaked live deadline")
			}
			if !want && v.Players[2].WithdrawRound != 0 {
				t.Fatal("replay leaked future withdrawal")
			}
		}
	})
}

func TestDeadlineSurvivesRestartAndOverdueRoundIsProcessedOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	synctest.Test(t, func(t *testing.T) {
		s := persistentServer(t, path)
		_, suffix := deadlineTable(t, s)
		before := readGameView(t, checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200))
		time.Sleep(2 * time.Minute)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s = persistentServer(t, path)
		v := readGameView(t, checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200))
		if !v.RoundDeadline.Equal(before.RoundDeadline) {
			t.Fatal("restart reset the deadline")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(4 * time.Minute)
		s = persistentServer(t, path)
		v = readGameView(t, checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200))
		if v.Round != 2 || len(v.History) != 1 || v.RoundDeadline.Sub(time.Now()) != game.RoundTimeout {
			t.Fatal("overdue restart did not process exactly one round with full new deadline")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s = persistentServer(t, path)
		again := readGameView(t, checkedRequest(t, s, "GET", "/api/state"+suffix, "", "", 200))
		if again.Version != v.Version || !again.RoundDeadline.Equal(v.RoundDeadline) {
			t.Fatal("restart repeated timeout")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(game.RoundTimeout)
		v2, err := publicView(s.rooms[strings.TrimPrefix(suffix, "?room=")].game)
		if err != nil || v2.Version != v.Version {
			t.Fatal("shutdown left an active deadline timer")
		}
	})
}

func readGameView(t *testing.T, data []byte) game.View {
	t.Helper()
	var v game.View
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDeadlineRetriesPersistenceWithoutPublishingUncommittedZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	synctest.Test(t, func(t *testing.T) {
		s := persistentServer(t, path)
		_, suffix := deadlineTable(t, s)
		id := strings.TrimPrefix(suffix, "?room=")
		s.mu.Lock()
		before, err := publicView(s.rooms[id].game)
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		err = s.store.db.Close()
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(game.RoundTimeout)
		synctest.Wait()
		s.mu.Lock()
		v, err := publicView(s.rooms[id].game)
		s.mu.Unlock()
		if err != nil || v.Version != before.Version || len(v.History) != 0 {
			t.Fatal("failed persistence published timeout")
		}
		db, err := bolt.Open(path, 0600, nil)
		if err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		s.store.db = db
		s.mu.Unlock()
		time.Sleep(time.Second)
		synctest.Wait()
		s.mu.Lock()
		v, err = publicView(s.rooms[id].game)
		s.mu.Unlock()
		if err != nil || v.Round != 2 || len(v.History) != 1 || v.History[0].Bids[2].Amount != 0 {
			t.Fatal("failed deadline was not retried")
		}
	})
}
