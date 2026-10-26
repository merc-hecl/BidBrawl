package server

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	bolt "go.etcd.io/bbolt"
)

var roomRecoveryBucket = []byte("room_recovery")

type recoveryRecord struct {
	Checksum string          `json:"checksum"`
	Accounts map[string]bool `json:"accounts"`
}

// The original room record stays untouched in the rooms bucket. Unavailable
// rooms never enter the game engine, request audit, deadline or settlement paths.
type unavailableRoom struct {
	name      string
	createdAt time.Time
	accounts  map[string]bool
}

func (s *Server) isolateRoom(tx *bolt.Tx, key, value []byte, cause error) error {
	var rec roomRecord
	parsed := json.Unmarshal(value, &rec) == nil
	name := rec.Name
	if !validName(name) {
		name = string(key)
	}
	locked := map[string]bool{}
	identified := parsed && rec.ID == string(key) && rec.Members != nil && rec.SeatAccounts != nil && len(rec.Members) > 0 && len(rec.Members) == len(rec.SeatAccounts)
	for _, aid := range rec.SeatAccounts {
		if s.accountsByID[aid] == nil || rec.Members[aid] == "" || locked[aid] {
			identified = false
		}
		locked[aid] = true
	}
	if !identified {
		// Membership cannot be trusted: preserve constraints for all existing
		// identities. Accounts registered after startup cannot belong to this room.
		for aid := range s.accountsByID {
			locked[aid] = true
		}
	}
	bucket, err := tx.CreateBucketIfNotExists(roomRecoveryBucket)
	if err != nil {
		return err
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(value))
	if data := bucket.Get(key); data != nil {
		var prior recoveryRecord
		if json.Unmarshal(data, &prior) != nil || prior.Accounts == nil || prior.Checksum == "" {
			return errors.New("invalid room recovery constraints")
		}
		for aid, held := range prior.Accounts {
			if !held || s.accountsByID[aid] == nil {
				return errors.New("invalid room recovery account")
			}
		}
		if prior.Checksum == checksum {
			locked = prior.Accounts
		} else {
			for aid := range prior.Accounts {
				locked[aid] = true
			}
		}
	}
	data, err := json.Marshal(recoveryRecord{Checksum: checksum, Accounts: locked})
	if err != nil {
		return err
	}
	if err := bucket.Put(key, data); err != nil {
		return err
	}
	s.unavailableRooms[string(key)] = &unavailableRoom{name: name, createdAt: rec.CreatedAt, accounts: locked}
	log.Printf("room %s is unavailable; original record preserved: %v", key, cause)
	return nil
}

func (s *Server) recoveryLocked(accountID string) bool {
	for _, rm := range s.unavailableRooms {
		if rm.accounts[accountID] {
			return true
		}
	}
	return false
}

func (s *Server) roomRecoveryLocked(rm *room) bool {
	for aid := range rm.members {
		if s.recoveryLocked(aid) {
			return true
		}
	}
	return false
}
