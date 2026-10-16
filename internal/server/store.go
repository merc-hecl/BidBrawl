package server

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	"bidbrawl/internal/hints"
	bolt "go.etcd.io/bbolt"
)

// Identities and complete private room snapshots share one transactional file.
// Login sessions and the host token are intentionally not restored.
type persistentStore struct {
	db *bolt.DB
}

type identityRecord struct {
	ID             string `json:"account_id"`
	Name           string `json:"name"`
	CredentialHash string `json:"credential_sha256"`
}

var identityBucket = []byte("accounts")

// Open restores identities and rooms from a local database, failing rather than silently
// starting with an empty account set if it is locked, damaged or unsupported.
func Open(geometry game.Config, cfg cards.Config, path string) (*Server, error) {
	return OpenWithHints(geometry, cfg, hints.Default(), path)
}

func OpenWithHints(geometry game.Config, cfg cards.Config, hintConfig hints.Config, path string) (*Server, error) {
	if err := geometry.Validate(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := hintConfig.Validate(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("identity database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create identity directory: %w", err)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		if errors.Is(err, bolt.ErrTimeout) {
			return nil, fmt.Errorf("身份数据库 %q 正被其他进程占用（等待文件锁超时）；同一数据目录只能运行一个宿主，请使用已运行的宿主，或先停止原宿主再重试。更换监听端口不会释放数据库锁: %w", path, err)
		}
		return nil, fmt.Errorf("open identity database %q: %w", path, err)
	}
	s := NewWithHints(geometry, cfg, hintConfig)
	s.store = &persistentStore{db: db}
	err = db.Update(func(tx *bolt.Tx) error {
		if meta := tx.Bucket([]byte("metadata")); meta != nil {
			version := string(meta.Get([]byte("schema_version")))
			if version != "2" {
				return errors.New("unsupported identity database schema")
			}
			if tx.Bucket(identityBucket) == nil {
				return errors.New("identity database is missing accounts")
			}
			if tx.Bucket(roomBucket) == nil {
				return errors.New("database is missing rooms")
			}
			return nil
		}
		if key, _ := tx.Cursor().First(); key != nil {
			return errors.New("unrecognized identity database")
		}
		meta, err := tx.CreateBucket([]byte("metadata"))
		if err != nil {
			return err
		}
		if err := meta.Put([]byte("schema_version"), []byte("2")); err != nil {
			return err
		}
		if _, err = tx.CreateBucket(identityBucket); err != nil {
			return err
		}
		_, err = tx.CreateBucket(roomBucket)
		return err
	})
	if err == nil {
		err = db.View(func(tx *bolt.Tx) error {
			if err := tx.Bucket(identityBucket).ForEach(func(key, value []byte) error {
				var rec identityRecord
				if json.Unmarshal(value, &rec) != nil {
					return errors.New("invalid identity record")
				}
				id, idErr := hex.DecodeString(rec.ID)
				digest, hashErr := hex.DecodeString(rec.CredentialHash)
				if string(key) != rec.ID || idErr != nil || len(id) != 16 || hashErr != nil || len(digest) != 32 || !validName(rec.Name) {
					return errors.New("invalid identity record")
				}
				hash := [32]byte(digest)
				if s.accounts[rec.Name] != nil || s.credentials[hash] != nil {
					return errors.New("duplicate identity record")
				}
				a := &account{ID: rec.ID, Name: rec.Name}
				s.accounts[a.Name] = a
				s.accountsByID[a.ID] = a
				s.credentials[hash] = a
				s.credentialHashes[a.ID] = hash
				return nil
			}); err != nil {
				return err
			}
			if err := s.loadRooms(tx); err != nil {
				return err
			}
			return s.loadDeletedResults(tx)
		})
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load identity database: %w", err)
	}
	return s, nil
}

func (s *persistentStore) saveAccount(a *account, hash [32]byte) error {
	record, err := json.Marshal(identityRecord{ID: a.ID, Name: a.Name, CredentialHash: hex.EncodeToString(hash[:])})
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(identityBucket).Put([]byte(a.ID), record)
	})
}

// Close is called after HTTP requests have drained, releasing the database lock.
func (s *Server) Close() error {
	s.CloseEvents()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return nil
	}
	return s.store.db.Close()
}
