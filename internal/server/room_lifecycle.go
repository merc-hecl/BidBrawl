package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Only the public economic summary survives deletion, never a game snapshot.
var deletedResultBucket = []byte("deleted_results")

func (s *Server) roomExists(w http.ResponseWriter, rm *room) bool {
	if s.rooms[rm.id] == rm {
		return true
	}
	fail(w, 404, errors.New("room not found"))
	return false
}

func (s *Server) manageRooms(w http.ResponseWriter, r *http.Request) {
	manager := s.isRoomManager(r)
	var actor *account
	if !manager {
		actor = s.requireActor(w, r)
		if actor == nil {
			return
		}
	}
	var input struct {
		Action  string   `json:"action"`
		RoomIDs []string `json:"room_ids"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Action != "archive" && input.Action != "restore" && input.Action != "delete" || len(input.RoomIDs) == 0 {
		fail(w, 400, errors.New("expected archive, restore or delete and unique room IDs"))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if actor != nil && s.sessions[token(r)] != actor {
		fail(w, 401, errors.New("login session required"))
		return
	}
	seen := make(map[string]bool, len(input.RoomIDs))
	next := make([]*room, 0, len(input.RoomIDs))
	retained := make(map[string]roomResult)
	now := time.Now().UTC()
	for _, id := range input.RoomIDs {
		if id == "" || seen[id] {
			fail(w, 400, errors.New("room IDs must be unique and nonempty"))
			return
		}
		seen[id] = true
		if s.unavailableRooms[id] != nil {
			fail(w, 503, errors.New("unrecovered rooms cannot be changed or deleted"))
			return
		}
		rm := s.rooms[id]
		if rm == nil {
			fail(w, 404, errors.New("room not found"))
			return
		}
		if !manager && rm.owner != actor.ID {
			fail(w, 403, errors.New("room owner required"))
			return
		}
		view, err := publicView(rm.game)
		if err != nil {
			roomFailure(w, err)
			return
		}
		if view.Phase == "bidding" {
			fail(w, 409, errors.New("ongoing rooms cannot be archived, restored or deleted"))
			return
		}
		copy := *rm
		switch input.Action {
		case "archive":
			if copy.archivedAt.IsZero() {
				copy.archivedAt = now
			}
		case "restore":
			copy.archivedAt = time.Time{}
		case "delete":
			result, err := resultForRoom(rm)
			if err != nil {
				roomFailure(w, err)
				return
			}
			if result != nil {
				retained[id] = *result
			}
		}
		next = append(next, &copy)
	}
	// Validate the entire selection, then commit it in one transaction.
	if s.store != nil {
		err := s.store.db.Update(func(tx *bolt.Tx) error {
			for _, rm := range next {
				if input.Action == "delete" {
					if result, ok := retained[rm.id]; ok {
						bucket, err := tx.CreateBucketIfNotExists(deletedResultBucket)
						if err != nil {
							return err
						}
						data, err := json.Marshal(result)
						if err != nil {
							return err
						}
						if err = bucket.Put([]byte(rm.id), data); err != nil {
							return err
						}
					}
					if err := tx.Bucket(roomBucket).Delete([]byte(rm.id)); err != nil {
						return err
					}
				} else {
					data, err := encodeRoom(rm)
					if err != nil {
						return err
					}
					if err = tx.Bucket(roomBucket).Put([]byte(rm.id), data); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			roomFailure(w, err)
			return
		}
	}
	for _, rm := range next {
		if input.Action == "delete" {
			delete(s.rooms, rm.id)
			if result, ok := retained[rm.id]; ok {
				s.deletedResults[rm.id] = result
			}
		} else {
			s.rooms[rm.id].archivedAt = rm.archivedAt
		}
		kind := map[string]string{"archive": "room_archived", "restore": "room_restored", "delete": "room_deleted"}[input.Action]
		event := roomEvent{Kind: kind, RoomID: rm.id, Rooms: true}
		s.publishEvent(event)
		publishTo(s.observerSubscribers, event)
	}
	reply(w, 200, map[string]any{"ok": true, "room_ids": input.RoomIDs})
}

func (s *Server) loadDeletedResults(tx *bolt.Tx) error {
	bucket := tx.Bucket(deletedResultBucket)
	if bucket == nil {
		return nil // No settled room has been deleted.
	}
	return bucket.ForEach(func(key, data []byte) error {
		var result roomResult
		if err := json.Unmarshal(data, &result); err != nil {
			return err
		}
		if result.RoomID == "" || string(key) != result.RoomID || s.rooms[result.RoomID] != nil || !validName(result.RoomName) || result.CreatedAt.IsZero() || result.SettledAt.Before(result.CreatedAt) || len(result.Players) < 2 || len(result.Players) > 7 || result.Settlement.Profit != result.Settlement.GrossProfit-totalCardCost(result.Settlement.CardCosts)-totalCardCost(result.Settlement.TicketCosts) {
			return errors.New("invalid retained room result")
		}
		settlement := result.Settlement
		if settlement.Paid < 0 || settlement.Total < 0 || settlement.Round < 1 || settlement.Round > 5 || settlement.Method != "instant" && settlement.Method != "final" && settlement.Method != "unsold" || len(settlement.Payoffs) != len(result.Players) {
			return errors.New("invalid retained settlement")
		}
		seats, agents := map[string]bool{}, map[string]bool{}
		for _, player := range result.Players {
			a := s.accountsByID[strings.TrimPrefix(player.AgentID, "account:")]
			if a == nil || player.AgentID != "account:"+a.ID || a.Name != player.Name || player.ID == "" || seats[player.ID] || agents[player.AgentID] {
				return errors.New("invalid retained result identity")
			}
			seats[player.ID], agents[player.AgentID] = true, true
			expected := -settlement.CardCosts[player.ID] - settlement.TicketCosts[player.ID]
			if player.ID == settlement.WinnerID {
				expected += settlement.GrossProfit
			}
			if payoff, ok := settlement.Payoffs[player.ID]; !ok || payoff != expected {
				return errors.New("inconsistent retained payoffs")
			}
		}
		if settlement.Method != "unsold" && !seats[settlement.WinnerID] {
			return errors.New("invalid retained winner")
		}
		s.deletedResults[result.RoomID] = result
		return nil
	})
}

func totalCardCost(costs map[string]int64) int64 {
	var total int64
	for _, n := range costs {
		total += n
	}
	return total
}
