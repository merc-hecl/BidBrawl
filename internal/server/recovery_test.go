package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

func TestUnavailableRoomPreservesRawDataAndOnlyLocksParticipants(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		t.Run(fmt.Sprint(unreadable), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "accounts.db")
			s := persistentServer(t, path)
			a, ca := accountSession(t, s, "Recovery A")
			b, _ := accountSession(t, s, "Recovery B")
			c, cc := accountSession(t, s, "Healthy C")
			d, cd := accountSession(t, s, "Healthy D")
			for _, token := range []string{a, b} {
				checkedRequest(t, s, "POST", "/api/join?room=table", token, `{"cards":[]}`, 201)
			}
			checkedRequest(t, s, "POST", "/api/start?room=table", s.HostToken, "", 200)
			created := checkedRequest(t, s, "POST", "/api/create", c, `{"name":"Healthy","players":2}`, 201)
			var createdRoom struct {
				ID string `json:"room_id"`
			}
			_ = json.Unmarshal(created, &createdRoom)
			good := "?room=" + createdRoom.ID
			for _, token := range []string{c, d} {
				checkedRequest(t, s, "POST", "/api/join"+good, token, `{"cards":[]}`, 201)
			}
			checkedRequest(t, s, "POST", "/api/start"+good, c, "", 200)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := bolt.Open(path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			var original []byte
			err = db.Update(func(tx *bolt.Tx) error {
				original = []byte(`{`)
				if !unreadable {
					var rec map[string]json.RawMessage
					_ = json.Unmarshal(tx.Bucket(roomBucket).Get([]byte("table")), &rec)
					var snapshot map[string]json.RawMessage
					_ = json.Unmarshal(rec["game"], &snapshot)
					snapshot["private_extra"] = json.RawMessage(`"secret-recovery-note"`)
					rec["game"], err = json.Marshal(snapshot)
					if err != nil {
						return err
					}
					original, err = json.Marshal(rec)
					if err != nil {
						return err
					}
				}
				return tx.Bucket(roomBucket).Put([]byte("table"), original)
			})
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			s, err = Open(game.DefaultConfig(), cards.Default(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if len(s.unavailableRooms) != 1 || len(s.rooms) != 1 {
				t.Fatal("one room failure affected healthy loading")
			}
			a = loginAgain(t, s, ca)
			c = loginAgain(t, s, cc)
			d = loginAgain(t, s, cd)
			list := checkedRequest(t, s, "GET", "/api/rooms", "", "", 200)
			if !bytes.Contains(list, []byte(`"phase":"unavailable"`)) || bytes.Contains(list, []byte("secret-recovery-note")) {
				t.Fatal("room status missing or private data exposed")
			}
			for _, route := range []string{"/api/state?room=table", "/api/replay?room=table", "/api/observer/state?room=table", "/api/observer/replay?room=table", "/api/catalog?room=table"} {
				token := ""
				if bytes.Contains([]byte(route), []byte("/observer/")) {
					token = s.ObserverToken
				}
				checkedRequest(t, s, "GET", route, token, "", 503)
			}
			checkedRequest(t, s, "POST", "/api/rooms/manage", s.HostToken, `{"action":"delete","room_ids":["table"]}`, 503)
			inventory := checkedRequest(t, s, "GET", "/api/inventory", a, "", 200)
			if !bytes.Contains(inventory, []byte(`"recovery_blocked":true`)) {
				t.Fatal("affected account was unlocked")
			}
			checkedRequest(t, s, "POST", "/api/buy", a, `{"card":"item_count","quantity":1}`, 409)
			checkedRequest(t, s, "POST", "/api/join"+good, a, `{"cards":[]}`, 409)
			code := 200
			if unreadable {
				code = 409
			}
			for _, token := range []string{c, d} {
				checkedRequest(t, s, "POST", "/api/bid"+good, token, `{"round":1,"amount":100,"reason":"healthy room estimate"}`, code)
			}
			view := readGameView(t, checkedRequest(t, s, "GET", "/api/state"+good, "", "", 200))
			if !unreadable && view.Round != 2 {
				t.Fatal("healthy room could not progress")
			}
			for _, route := range []string{"/api/stats", "/api/leaderboard", "/api/results"} {
				result := checkedRequest(t, s, "GET", route, "", "", 200)
				if !bytes.Contains(result, []byte(`"unavailable_rooms":1`)) {
					t.Fatal("incomplete statistics not marked")
				}
			}
			err = s.store.db.View(func(tx *bolt.Tx) error {
				if !bytes.Equal(original, tx.Bucket(roomBucket).Get([]byte("table"))) {
					return fmt.Errorf("unavailable record was changed")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			fresh, freshCredential := accountSession(t, s, "New identity")
			if s.recoveryLocked(s.sessions[fresh].ID) {
				t.Fatal("new identity was mistaken for old room member")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(game.DefaultConfig(), cards.Default(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			fresh = loginAgain(t, s, freshCredential)
			if s.recoveryLocked(s.sessions[fresh].ID) || !s.recoveryLocked(s.accounts["Recovery A"].ID) {
				t.Fatal("restart changed recovery constraints")
			}
		})
	}
}

func TestMigrationFailureAfterRoomConversionRollsBackAllWrites(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(fmt.Sprint(future), func(t *testing.T) {
			path, _, _, _ := releasedDatabase(t, "bidding")
			db, err := bolt.Open(path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			var original []byte
			err = db.Update(func(tx *bolt.Tx) error {
				if future {
					var rec roomRecord
					_ = json.Unmarshal(tx.Bucket(roomBucket).Get([]byte("table")), &rec)
					rec.Game = json.RawMessage(`{"schema_version":12,"unknown_future_field":true}`)
					raw, err := json.Marshal(rec)
					if err != nil {
						return err
					}
					if err := tx.Bucket(roomBucket).Put([]byte("table"), raw); err != nil {
						return err
					}
				} else {
					bucket, err := tx.CreateBucketIfNotExists(deletedResultBucket)
					if err != nil {
						return err
					}
					if err := bucket.Put([]byte("bad"), []byte(`{}`)); err != nil {
						return err
					}
				}
				original = bytes.Clone(tx.Bucket(roomBucket).Get([]byte("table")))
				return nil
			})
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			if s, err := Open(game.DefaultConfig(), cards.Default(), path); err == nil {
				_ = s.Close()
				t.Fatal("accepted future or invalid global data")
			}
			db, err = bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			err = db.View(func(tx *bolt.Tx) error {
				if string(tx.Bucket([]byte("metadata")).Get([]byte("schema_version"))) != "3" || !bytes.Equal(original, tx.Bucket(roomBucket).Get([]byte("table"))) {
					return fmt.Errorf("failed migration partially committed")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
