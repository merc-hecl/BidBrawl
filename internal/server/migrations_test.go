package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

// Build persisted records matching v0.1.0: schema 3 DB, schema 10 snapshots,
// no deadlines or timeout decisions. Gameplay itself uses normal protocol tests.
func releasedDatabase(t *testing.T, phase string) (string, string, []string, map[string][]byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, ca := accountSession(t, s, "Upgrade A")
	b, cb := accountSession(t, s, "Upgrade B")
	purchaseJoinCards(t, s, a, `{"cards":["item_count"]}`)
	checkedRequest(t, s, "POST", "/api/join?room=table", a, `{"cards":["item_count"]}`, 201)
	checkedRequest(t, s, "POST", "/api/join?room=table", b, `{"cards":[]}`, 201)
	if phase != "lobby" {
		checkedRequest(t, s, "POST", "/api/start?room=table", s.HostToken, "", 200)
		checkedRequest(t, s, "POST", "/api/card?room=table", a, `{"round":1,"card":"item_count","reason":"count before estimate"}`, 200)
		checkedRequest(t, s, "POST", "/api/bid?room=table", a, `{"round":1,"amount":100,"reason":"first estimate"}`, 200)
	}
	if phase == "settled" {
		checkedRequest(t, s, "POST", "/api/bid?room=table", b, `{"round":1,"amount":100,"reason":"equal estimate"}`, 200)
		for round := 2; round <= game.Rounds; round++ {
			for _, token := range []string{a, b} {
				checkedRequest(t, s, "POST", "/api/bid?room=table", token, fmt.Sprintf(`{"round":%d,"amount":100,"reason":"equal estimate"}`, round), 200)
			}
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	accounts := map[string][]byte{}
	err = db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(identityBucket).ForEach(func(k, v []byte) error { accounts[string(k)] = bytes.Clone(v); return nil }); err != nil {
			return err
		}
		var rec map[string]json.RawMessage
		if err := json.Unmarshal(tx.Bucket(roomBucket).Get([]byte("table")), &rec); err != nil {
			return err
		}
		var snapshot map[string]json.RawMessage
		if err := json.Unmarshal(rec["game"], &snapshot); err != nil {
			return err
		}
		snapshot["schema_version"] = json.RawMessage(`10`)
		delete(snapshot, "round_deadline")
		rec["game"], err = json.Marshal(snapshot)
		if err != nil {
			return err
		}
		rec["retained_extension"] = json.RawMessage(`{"keep":true}`)
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if err := tx.Bucket(roomBucket).Put([]byte("table"), data); err != nil {
			return err
		}
		return tx.Bucket([]byte("metadata")).Put([]byte("schema_version"), []byte("3"))
	})
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	return path, "table", []string{ca, cb}, accounts
}

func TestReleasedMigrationPreservesAccountsRoomsAndBackup(t *testing.T) {
	for _, phase := range []string{"lobby", "bidding", "settled"} {
		t.Run(phase, func(t *testing.T) {
			path, id, credentials, accounts := releasedDatabase(t, phase)
			start := time.Now().UTC()
			s, err := Open(game.DefaultConfig(), cards.Default(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			v, err := publicView(s.rooms[id].game)
			if err != nil {
				t.Fatal(err)
			}
			if v.Phase != phase || len(s.unavailableRooms) != 0 {
				t.Fatal("released room failed to migrate")
			}
			if phase == "bidding" {
				if v.RoundDeadline.Before(start.Add(game.RoundTimeout)) || v.RoundDeadline.After(time.Now().Add(game.RoundTimeout)) {
					t.Fatal("active round did not receive five minutes")
				}
				ownData, err := s.rooms[id].game.Observe(s.rooms[id].members[s.accounts["Upgrade A"].ID])
				if err != nil {
					t.Fatal(err)
				}
				own := readGameView(t, ownData)
				if own.Self.Bid == nil || *own.Self.Bid != 100 || len(own.Self.Used) != 1 || len(own.Self.Hints) != 1 {
					t.Fatal("migration lost sealed bid or private card history")
				}
			} else if !v.RoundDeadline.IsZero() {
				t.Fatal("migration invented inactive deadline")
			}
			if phase == "settled" && (v.Settlement == nil || v.Settlement.Paid != 100 || len(v.History) != 5) {
				t.Fatal("settlement changed")
			}
			err = s.store.db.View(func(tx *bolt.Tx) error {
				if string(tx.Bucket([]byte("metadata")).Get([]byte("schema_version"))) != identitySchemaVersion {
					return fmt.Errorf("database version not committed")
				}
				for aid, before := range accounts {
					if !bytes.Equal(before, tx.Bucket(identityBucket).Get([]byte(aid))) {
						return fmt.Errorf("account changed during migration")
					}
				}
				var rec map[string]json.RawMessage
				_ = json.Unmarshal(tx.Bucket(roomBucket).Get([]byte(id)), &rec)
				if !bytes.Equal(rec["retained_extension"], []byte(`{"keep":true}`)) {
					return fmt.Errorf("outer record field lost")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			_ = loginAgain(t, s, credentials[0])
			backups, err := filepath.Glob(path + ".pre-v4-*.bak")
			if err != nil || len(backups) != 1 {
				t.Fatal("missing unique backup")
			}
			backup, err := bolt.Open(backups[0], 0600, &bolt.Options{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			err = backup.View(func(tx *bolt.Tx) error {
				if string(tx.Bucket([]byte("metadata")).Get([]byte("schema_version"))) != "3" {
					return fmt.Errorf("backup was made after upgrade")
				}
				var rec roomRecord
				_ = json.Unmarshal(tx.Bucket(roomBucket).Get([]byte(id)), &rec)
				var header struct {
					Version int `json:"schema_version"`
				}
				_ = json.Unmarshal(rec.Game, &header)
				if header.Version != 10 {
					return fmt.Errorf("backup snapshot was changed")
				}
				for aid, before := range accounts {
					if !bytes.Equal(before, tx.Bucket(identityBucket).Get([]byte(aid))) {
						return fmt.Errorf("backup lost account")
					}
				}
				return nil
			})
			_ = backup.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(game.DefaultConfig(), cards.Default(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			again, _ := publicView(s.rooms[id].game)
			if again.Version != v.Version || !again.RoundDeadline.Equal(v.RoundDeadline) {
				t.Fatal("restart repeated migration")
			}
			backups, _ = filepath.Glob(path + ".pre-v4-*.bak")
			if len(backups) != 1 {
				t.Fatal("restart repeated backup")
			}
		})
	}
}

func TestMigrationRollsBackWhenAccountDataIsInvalid(t *testing.T) {
	path, _, _, _ := releasedDatabase(t, "bidding")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	var original []byte
	err = db.Update(func(tx *bolt.Tx) error {
		original = bytes.Clone(tx.Bucket(roomBucket).Get([]byte("table")))
		key, _ := tx.Bucket(identityBucket).Cursor().First()
		return tx.Bucket(identityBucket).Put(key, []byte(`{"assets":-1}`))
	})
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if s, err := Open(game.DefaultConfig(), cards.Default(), path); err == nil {
		_ = s.Close()
		t.Fatal("invalid accounts accepted")
	}
	db, err = bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		if string(tx.Bucket([]byte("metadata")).Get([]byte("schema_version"))) != "3" || !bytes.Equal(original, tx.Bucket(roomBucket).Get([]byte("table"))) {
			return fmt.Errorf("failed migration changed original")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMigrationBackupFailureLeavesDatabaseUnchanged(t *testing.T) {
	path, _, _, _ := releasedDatabase(t, "lobby")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(game.DefaultConfig(), cards.Default())
	defer s.CloseEvents()
	s.store = &persistentStore{db: db}
	if err := s.loadDatabase(filepath.Join(path, "missing", "accounts.db")); err == nil {
		t.Fatal("upgrade continued without backup")
	}
	err = db.View(func(tx *bolt.Tx) error {
		if string(tx.Bucket([]byte("metadata")).Get([]byte("schema_version"))) != "3" {
			return fmt.Errorf("backup failure changed original")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
