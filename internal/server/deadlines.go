package server

import (
	"log"
	"time"

	"bidbrawl/internal/game"
)

// All timer changes and callbacks hold s.mu, just like player actions.
func (s *Server) scheduleDeadline(rm *room, view game.View) {
	if timer := s.deadlineTimers[rm.id]; timer != nil {
		timer.Stop()
		delete(s.deadlineTimers, rm.id)
	}
	if s.eventsClosed || view.Phase != "bidding" || s.roomRecoveryLocked(rm) {
		return
	}
	id := rm.id
	s.deadlineTimers[id] = time.AfterFunc(time.Until(view.RoundDeadline), func() { s.expireRoom(id) })
}

func (s *Server) expireRoom(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eventsClosed {
		return
	}
	rm := s.rooms[id]
	if rm == nil {
		return
	}
	view, err := publicView(rm.game)
	if err != nil {
		log.Printf("read room deadline: %v", err)
		return
	}
	now := time.Now().UTC()
	if view.Phase != "bidding" || now.Before(view.RoundDeadline) {
		return // An old callback may already have been queued before a round advanced.
	}
	if _, err := s.commitRoom(rm, func(next *room) error { return expireRound(next, now) }); err != nil {
		log.Printf("expire room %s: %v", id, err)
		// Failed persistence must neither publish the candidate nor lose the deadline.
		s.deadlineTimers[id] = time.AfterFunc(time.Second, func() { s.expireRoom(id) })
	}
}

func (s *Server) resumeDeadlines() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rm := range s.rooms {
		if s.roomRecoveryLocked(rm) {
			continue
		}
		view, err := publicView(rm.game)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if view.Phase == "bidding" && !now.Before(view.RoundDeadline) {
			if _, err := s.commitRoom(rm, func(next *room) error { return expireRound(next, now) }); err != nil {
				return err
			}
		} else {
			s.scheduleDeadline(rm, view)
		}
	}
	return nil
}

// Timeout activity commits with the round, including when no HTTP request runs.
func expireRound(rm *room, now time.Time) error {
	before := observerView(rm.game)
	if err := rm.game.ExpireRound(now); err != nil {
		return err
	}
	after := observerView(rm.game)
	events := []activity{}
	for _, d := range after.Decisions[len(before.Decisions):] {
		if d.Action == "timeout" {
			events = append(events, activity{PlayerID: d.PlayerID, Round: d.Round, Action: "timeout", Tool: "auction", Reason: d.Reason, Params: map[string]any{"amount": 0}})
		}
	}
	for seat, hints := range after.Intelligence {
		for _, h := range hints {
			if h.Round > before.Round {
				events = append(events, activity{PlayerID: seat, Round: h.Round, Action: "specialty", Tool: "specialty:" + h.Kind, Result: h.Data})
			}
		}
	}
	for _, h := range after.PublicHints[len(before.PublicHints):] {
		events = append(events, activity{Round: h.Round, Action: "public_hint", Tool: h.Kind, Result: h.Data})
	}
	for _, r := range after.History[len(before.History):] {
		events = append(events, activity{Round: r.Round, Action: "round_revealed", Tool: "auction"})
	}
	if after.Phase == "settled" {
		events = append(events, activity{Round: after.Round, Action: "settled", Tool: "auction"})
	}
	rm.activities = append([]activity{}, rm.activities...)
	for _, e := range events {
		e.ID, e.At, e.Source, e.Status = len(rm.activities)+1, now, "system", "success"
		rm.activities = append(rm.activities, e)
	}
	return nil
}
