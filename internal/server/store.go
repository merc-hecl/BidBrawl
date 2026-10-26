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
	CardStock      map[string][]int64 `json:"cards"`
	Warehouse      []storedItem       `json:"warehouse"`
	ReliefAt       string             `json:"relief_claimed_at,omitempty"`
	ID             string             `json:"account_id"`
	Name           string             `json:"name"`
	Assets         int64              `json:"assets"`
	CredentialHash string             `json:"credential_sha256"`
}

var identityBucket = []byte("accounts")

const identitySchemaVersion = "4"

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
			return nil, fmt.Errorf("identity database %q is locked by another process (timed out waiting for the file lock); only one host can use a data directory. Use the running host, or stop it before retrying. Changing the listen port does not release the database lock: %w", path, err)
		}
		return nil, fmt.Errorf("open identity database %q: %w", path, err)
	}
	s := NewWithHints(geometry, cfg, hintConfig)
	s.store = &persistentStore{db: db}
	err = s.loadDatabase(path)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load identity database %q: %w", path, err)
	}
	if err := s.resumeDeadlines(); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("resume room deadlines: %w", err)
	}
	return s, nil
}

func identityFor(a *account, hash [32]byte) identityRecord {
	return identityRecord{ID: a.ID, Name: a.Name, Assets: a.Assets, CardStock: a.CardStock, Warehouse: a.Warehouse, ReliefAt: a.ReliefAt, CredentialHash: hex.EncodeToString(hash[:])}
}

func (s *persistentStore) saveAccount(a *account, hash [32]byte) error {
	record, err := json.Marshal(identityFor(a, hash))
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
