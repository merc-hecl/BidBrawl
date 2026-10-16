package server

import (
	"encoding/json"
	"errors"
	"log"
	"maps"
	"math"
	"net/http"
	"time"

	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

var roomBucket = []byte("rooms")

type roomRecord struct {
	ID           string            `json:"room_id"`
	Name         string            `json:"name"`
	Owner        string            `json:"owner_id"`
	Members      map[string]string `json:"members"`
	SeatAccounts map[string]string `json:"seat_accounts"`
	CreatedAt    time.Time         `json:"created_at"`
	SettledAt    time.Time         `json:"settled_at"`
	ArchivedAt   time.Time         `json:"archived_at,omitempty"`
	Game         json.RawMessage   `json:"game"`
}

func (s *persistentStore) saveRoom(rm *room, accounts ...identityRecord) error {
	record, err := encodeRoom(rm)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, a := range accounts {
			data, err := json.Marshal(a)
			if err != nil {
				return err
			}
			if err := tx.Bucket(identityBucket).Put([]byte(a.ID), data); err != nil {
				return err
			}
		}
		return tx.Bucket(roomBucket).Put([]byte(rm.id), record)
	})
}

func encodeRoom(rm *room) ([]byte, error) {
	snapshot, err := rm.game.Snapshot()
	if err != nil {
		return nil, err
	}
	return json.Marshal(roomRecord{ID: rm.id, Name: rm.name, Owner: rm.owner, Members: rm.members, SeatAccounts: rm.seatAccounts, CreatedAt: rm.createdAt, SettledAt: rm.settledAt, ArchivedAt: rm.archivedAt, Game: snapshot})
}

func (s *Server) loadRooms(tx *bolt.Tx) error {
	s.rooms = map[string]*room{}
	return tx.Bucket(roomBucket).ForEach(func(key, value []byte) error {
		var rec roomRecord
		if err := json.Unmarshal(value, &rec); err != nil {
			return err
		}
		if string(key) != rec.ID || rec.ID == "" || !validName(rec.Name) || rec.CreatedAt.IsZero() || rec.Members == nil || rec.SeatAccounts == nil || rec.Owner != "" && s.accountsByID[rec.Owner] == nil {
			return errors.New("invalid room record")
		}
		g, err := game.Restore(rec.Game)
		if err != nil {
			return err
		}
		public, err := g.Observe("")
		if err != nil {
			return err
		}
		var view game.View
		if err = json.Unmarshal(public, &view); err != nil {
			return err
		}
		if len(rec.Members) != len(view.Players) || len(rec.SeatAccounts) != len(view.Players) || (view.Phase == "settled") != !rec.SettledAt.IsZero() || !rec.SettledAt.IsZero() && rec.SettledAt.Before(rec.CreatedAt) {
			return errors.New("inconsistent room membership or settlement date")
		}
		for _, p := range view.Players {
			id := rec.SeatAccounts[p.ID]
			a := s.accountsByID[id]
			if a == nil || a.Name != p.Name || rec.Members[id] == "" {
				return errors.New("invalid seat account")
			}
			private, err := g.Observe(rec.Members[id])
			if err != nil {
				return errors.New("invalid stored seat token")
			}
			var own game.View
			if json.Unmarshal(private, &own) != nil || own.Self == nil || own.Self.ID != p.ID {
				return errors.New("seat token does not match its account")
			}
		}
		if !rec.ArchivedAt.IsZero() && (view.Phase == "bidding" || rec.ArchivedAt.Before(rec.CreatedAt)) {
			return errors.New("invalid room archive")
		}
		s.rooms[rec.ID] = &room{id: rec.ID, name: rec.Name, owner: rec.Owner, game: g, members: rec.Members, seatAccounts: rec.SeatAccounts, createdAt: rec.CreatedAt, settledAt: rec.SettledAt, archivedAt: rec.ArchivedAt}
		return nil
	})
}

func roomFailure(w http.ResponseWriter, err error) {
	log.Printf("persist room: %v", err)
	fail(w, 500, errors.New("could not persist room state"))
}

// Caller holds s.mu. Room mutations use a private candidate: readers cannot
// see uncommitted bids/settlements. Account assets and settlement commit together;
// failed writes leave both the live room and account unchanged.
func (s *Server) mutateRoom(w http.ResponseWriter, rm *room, change func(*room) error) bool {
	if !s.roomExists(w, rm) {
		return false
	}
	if !rm.archivedAt.IsZero() {
		fail(w, 409, errors.New("restore the archived room before acting"))
		return false
	}
	before, err := publicView(rm.game)
	if err != nil {
		roomFailure(w, err)
		return false
	}
	beforeObserver, err := rm.game.ObserveObserver()
	if err != nil {
		roomFailure(w, err)
		return false
	}
	snapshot, err := rm.game.Snapshot()
	if err != nil {
		roomFailure(w, err)
		return false
	}
	g, err := game.Restore(snapshot)
	if err != nil {
		roomFailure(w, err)
		return false
	}
	copy := *rm
	copy.game, copy.members, copy.seatAccounts = g, maps.Clone(rm.members), maps.Clone(rm.seatAccounts)
	next := &copy
	if err := change(next); err != nil {
		fail(w, 409, err)
		return false
	}
	public, err := next.game.Observe("")
	if err != nil {
		roomFailure(w, err)
		return false
	}
	var view game.View
	if err = json.Unmarshal(public, &view); err != nil {
		roomFailure(w, err)
		return false
	}
	if view.Phase == "settled" && next.settledAt.IsZero() {
		next.settledAt = time.Now().UTC()
	}
	var winner *account
	var updated account
	var accounts []identityRecord
	if before.Phase != "settled" && view.Phase == "settled" {
		winner = s.accountsByID[next.seatAccounts[view.Settlement.WinnerID]]
		if winner == nil {
			roomFailure(w, errors.New("missing settlement account"))
			return false
		}
		updated = *winner
		profit := view.Settlement.Profit
		if profit > 0 && updated.Assets > math.MaxInt64-profit || profit < 0 && updated.Assets < -profit {
			fail(w, 409, errors.New("invalid resulting account assets"))
			return false
		}
		updated.Assets += profit
		accounts = append(accounts, identityFor(&updated, s.credentialHashes[updated.ID]))
	}
	if s.store != nil {
		if err := s.store.saveRoom(next, accounts...); err != nil {
			roomFailure(w, err)
			return false
		}
	}
	*rm = *next
	if winner != nil {
		winner.Assets = updated.Assets
	}
	s.publishRoomChange(rm.id, before, view)
	afterObserver, _ := rm.game.ObserveObserver()
	var prior, current game.View
	_ = json.Unmarshal(beforeObserver, &prior)
	_ = json.Unmarshal(afterObserver, &current)
	if current.Version != prior.Version {
		kind := "observer_changed"
		if before.Phase != "settled" && view.Phase == "settled" {
			kind = "settled"
		} else if before.Phase == "lobby" && view.Phase == "bidding" {
			kind = "started"
		} else if len(view.History) > len(before.History) {
			kind = "round_revealed"
		}
		publishTo(s.observerSubscribers, roomEvent{Kind: kind, RoomID: rm.id, Version: current.Version, Rooms: len(view.Players) != len(before.Players) || view.Phase != before.Phase, Results: view.Phase == "settled" && before.Phase != "settled"})
	}
	return true
}
