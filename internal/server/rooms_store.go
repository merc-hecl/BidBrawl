package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"time"

	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

var roomBucket = []byte("rooms")

type roomRecord struct {
	Activities   []activity        `json:"activities"`
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
	return json.Marshal(roomRecord{Activities: rm.activities, ID: rm.id, Name: rm.name, Owner: rm.owner, Members: rm.members, SeatAccounts: rm.seatAccounts, CreatedAt: rm.createdAt, SettledAt: rm.settledAt, ArchivedAt: rm.archivedAt, Game: snapshot})
}

func (s *Server) loadRooms(tx *bolt.Tx) error {
	s.rooms = map[string]*room{}
	s.unavailableRooms = map[string]*unavailableRoom{}
	return tx.Bucket(roomBucket).ForEach(func(key, value []byte) error {
		var header struct {
			Game struct {
				SchemaVersion int `json:"schema_version"`
			} `json:"game"`
		}
		if json.Unmarshal(value, &header) == nil && header.Game.SchemaVersion > game.SnapshotSchemaVersion {
			return fmt.Errorf("room %s has snapshot schema %d, supported schema %d: %w", key, header.Game.SchemaVersion, game.SnapshotSchemaVersion, game.ErrFutureSnapshot)
		}
		rm, err := s.restoreRoom(key, value)
		if errors.Is(err, game.ErrFutureSnapshot) {
			return err
		}
		if err != nil {
			return s.isolateRoom(tx, key, value, err)
		}
		s.rooms[rm.id] = rm
		return nil
	})
}

func (s *Server) restoreRoom(key, value []byte) (*room, error) {
	var rec roomRecord
	if err := json.Unmarshal(value, &rec); err != nil {
		return nil, err
	}
	if string(key) != rec.ID || rec.ID == "" || !validName(rec.Name) || rec.CreatedAt.IsZero() || rec.Members == nil || rec.SeatAccounts == nil || rec.Owner != "" && s.accountsByID[rec.Owner] == nil {
		return nil, errors.New("invalid room record")
	}
	g, err := game.Restore(rec.Game)
	if err != nil {
		return nil, err
	}
	public, err := g.Observe("")
	if err != nil {
		return nil, err
	}
	var view game.View
	if err = json.Unmarshal(public, &view); err != nil {
		return nil, err
	}
	if len(rec.Members) != len(view.Players) || len(rec.SeatAccounts) != len(view.Players) || (view.Phase == "settled") != !rec.SettledAt.IsZero() || !rec.SettledAt.IsZero() && rec.SettledAt.Before(rec.CreatedAt) {
		return nil, errors.New("inconsistent room membership or settlement date")
	}
	for _, p := range view.Players {
		id := rec.SeatAccounts[p.ID]
		a := s.accountsByID[id]
		if a == nil || a.Name != p.Name || rec.Members[id] == "" {
			return nil, errors.New("invalid seat account")
		}
		private, err := g.Observe(rec.Members[id])
		if err != nil {
			return nil, errors.New("invalid stored seat token")
		}
		var own game.View
		if json.Unmarshal(private, &own) != nil || own.Self == nil || own.Self.ID != p.ID {
			return nil, errors.New("seat token does not match its account")
		}
	}
	if !rec.ArchivedAt.IsZero() && (view.Phase == "bidding" || rec.ArchivedAt.Before(rec.CreatedAt)) {
		return nil, errors.New("invalid room archive")
	}
	return &room{activities: rec.Activities, id: rec.ID, name: rec.Name, owner: rec.Owner, game: g, members: rec.Members, seatAccounts: rec.SeatAccounts, createdAt: rec.CreatedAt, settledAt: rec.SettledAt, archivedAt: rec.ArchivedAt}, nil
}
func roomFailure(w http.ResponseWriter, err error) {
	log.Printf("persist room: %v", err)
	fail(w, 500, errors.New("could not persist room state"))
}

// Caller holds s.mu. Room mutations use a private candidate: readers cannot
// see uncommitted bids/settlements. Account assets and settlement commit together;
// failed writes leave both the live room and account unchanged.
func (s *Server) mutateRoom(w http.ResponseWriter, rm *room, change func(*room) error) bool {
	code, err := s.commitRoom(rm, change)
	if err != nil {
		if code == 500 {
			roomFailure(w, err)
		} else {
			fail(w, code, err)
		}
		return false
	}
	return true
}

// Caller holds s.mu. HTTP actions and deadline expiry share the same transaction.
func (s *Server) commitRoom(rm *room, change func(*room) error) (int, error) {
	if s.rooms[rm.id] != rm {
		return 404, errors.New("room not found")
	}
	if s.roomRecoveryLocked(rm) {
		return 409, errors.New("participant has an unrecovered room; assets remain locked")
	}
	if !rm.archivedAt.IsZero() {
		return 409, errors.New("restore the archived room before acting")
	}
	before, err := publicView(rm.game)
	if err != nil {
		return 500, err
	}
	beforeObserver, err := rm.game.ObserveObserver()
	if err != nil {
		return 500, err
	}
	snapshot, err := rm.game.Snapshot()
	if err != nil {
		return 500, err
	}
	g, err := game.Restore(snapshot)
	if err != nil {
		return 500, err
	}
	copy := *rm
	copy.game, copy.members, copy.seatAccounts = g, maps.Clone(rm.members), maps.Clone(rm.seatAccounts)
	next := &copy
	if err := change(next); err != nil {
		return 409, err
	}
	public, err := next.game.Observe("")
	if err != nil {
		return 500, err
	}
	var view game.View
	if err = json.Unmarshal(public, &view); err != nil {
		return 500, err
	}
	if view.Phase == "settled" && next.settledAt.IsZero() {
		next.settledAt = time.Now().UTC()
	}
	updated := map[string]account{}
	if before.Phase == "lobby" && view.Phase == "bidding" {
		for _, aid := range next.seatAccounts {
			a := cloneAccount(s.accountsByID[aid])
			if a.Assets < view.Rules.TicketFee {
				return 409, errors.New("insufficient cash for ticket")
			}
			a.Assets -= view.Rules.TicketFee
			updated[aid] = a
		}
	}
	beforeEconomy, afterEconomy := rm.game.Economy(), next.game.Economy()
	for seat, now := range afterEconomy {
		prior := beforeEconomy[seat]
		if len(now.Used) < len(prior.Used) {
			return 500, errors.New("card history shrank")
		}
		for _, id := range now.Used[len(prior.Used):] {
			aid := next.seatAccounts[seat]
			a, ok := updated[aid]
			if !ok {
				a = cloneAccount(s.accountsByID[aid])
			}
			if len(a.CardStock[id]) == 0 {
				return 409, errors.New("card stock exhausted")
			}
			a.CardStock[id] = a.CardStock[id][1:]
			updated[aid] = a
		}
	}
	if before.Phase != "settled" && view.Phase == "settled" && view.Settlement.WinnerID != "" {
		aid := next.seatAccounts[view.Settlement.WinnerID]
		a, ok := updated[aid]
		if !ok {
			a = cloneAccount(s.accountsByID[aid])
		}
		if a.Assets < view.Settlement.Paid {
			return 409, errors.New("insufficient settlement cash")
		}
		a.Assets -= view.Settlement.Paid
		for _, item := range view.Settlement.Items {
			a.Warehouse = append(a.Warehouse, storedItem{ID: next.id + ":" + item.ID, RoomID: next.id, Item: item})
		}
		updated[aid] = a
	}
	accounts := []identityRecord{}
	for id, a := range updated {
		accounts = append(accounts, identityFor(&a, s.credentialHashes[id]))
	}
	if s.store != nil {
		if err := s.store.saveRoom(next, accounts...); err != nil {
			return 500, err
		}
	}
	*rm = *next
	for id, a := range updated {
		*s.accountsByID[id] = a
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
	s.scheduleDeadline(rm, view)
	return 200, nil
}
