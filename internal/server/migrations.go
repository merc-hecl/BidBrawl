package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

func (s *Server) loadDatabase(path string) error {
	version := ""
	err := s.store.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("metadata"))
		if meta == nil {
			if key, _ := tx.Cursor().First(); key != nil {
				return errors.New("unrecognized identity database")
			}
			return nil
		}
		version = string(meta.Get([]byte("schema_version")))
		if version != "3" && version != "4" && version != identitySchemaVersion {
			return fmt.Errorf("unsupported identity database schema %q (expected %q or released schema 3/4); check the data directory specified by -data-dir or BIDBRAWL_DATA_DIR; newer databases require a newer application", version, identitySchemaVersion)
		}
		if tx.Bucket(identityBucket) == nil || tx.Bucket(roomBucket) == nil {
			return errors.New("identity database is missing accounts or rooms")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if version == "3" || version == "4" {
		if err := s.backupDatabase(path); err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
	}
	return s.store.db.Update(func(tx *bolt.Tx) error {
		if version == "" {
			for _, name := range [][]byte{[]byte("metadata"), identityBucket, roomBucket} {
				if _, err := tx.CreateBucket(name); err != nil {
					return err
				}
			}
		}
		if err := s.loadAccounts(tx); err != nil {
			return err
		}
		if version == "3" || version == "4" {
			if err := s.upgradeRooms(tx, time.Now().UTC()); err != nil {
				return err
			}
		}
		if err := s.loadRooms(tx); err != nil {
			return err
		}
		if err := s.loadDeletedResults(tx); err != nil {
			return err
		}
		return tx.Bucket([]byte("metadata")).Put([]byte("schema_version"), []byte(identitySchemaVersion))
	})
}

func (s *Server) backupDatabase(path string) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".pre-v5-*.bak")
	if err != nil {
		return err
	}
	err = s.store.db.View(func(tx *bolt.Tx) error { _, err := tx.WriteTo(f); return err })
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	log.Printf("database migration backup: %s", f.Name())
	return nil
}

func (s *Server) upgradeRooms(tx *bolt.Tx, now time.Time) error {
	updates := map[string][]byte{}
	err := tx.Bucket(roomBucket).ForEach(func(key, value []byte) error {
		var rec map[string]json.RawMessage
		if json.Unmarshal(value, &rec) != nil {
			return nil
		} // Preserved for isolation by loadRooms.
		upgraded, err := game.UpgradeSnapshot(rec["game"], now)
		if errors.Is(err, game.ErrFutureSnapshot) {
			return fmt.Errorf("room %s: %w", key, err)
		}
		if err != nil || bytes.Equal(upgraded, rec["game"]) {
			return nil
		}
		rec["game"] = upgraded
		candidate, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := s.restoreRoom(key, candidate); err != nil {
			return nil
		}
		updates[string(key)] = candidate
		return nil
	})
	if err != nil {
		return err
	}
	for key, value := range updates {
		if err := tx.Bucket(roomBucket).Put([]byte(key), value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) loadAccounts(tx *bolt.Tx) error {
	return tx.Bucket(identityBucket).ForEach(func(key, value []byte) error {
		var rec identityRecord
		if json.Unmarshal(value, &rec) != nil {
			return errors.New("invalid identity record")
		}
		id, idErr := hex.DecodeString(rec.ID)
		digest, hashErr := hex.DecodeString(rec.CredentialHash)
		if string(key) != rec.ID || idErr != nil || len(id) != 16 || hashErr != nil || len(digest) != 32 || !validName(rec.Name) || rec.Assets < 0 {
			return errors.New("invalid identity record")
		}
		hash := [32]byte(digest)
		if s.accounts[rec.Name] != nil || s.credentials[hash] != nil {
			return errors.New("duplicate identity record")
		}
		a := &account{ID: rec.ID, Name: rec.Name, Assets: rec.Assets, CardStock: rec.CardStock, Warehouse: rec.Warehouse, ReliefAt: rec.ReliefAt}
		s.accounts[a.Name], s.accountsByID[a.ID], s.credentials[hash], s.credentialHashes[a.ID] = a, a, a, hash
		return nil
	})
}
