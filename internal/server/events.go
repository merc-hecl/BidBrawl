package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"bidbrawl/internal/game"
)

// Events are invalidations, never private observations or a durable event log.
// All publication and subscription changes hold s.mu. A reconnect always syncs.
type roomEvent struct {
	Kind    string `json:"-"`
	RoomID  string `json:"room_id"`
	Version int    `json:"version"`
	Rooms   bool   `json:"rooms"`
	Results bool   `json:"results"`
}

func publicView(g *game.Game) (game.View, error) {
	data, err := g.Observe("")
	var view game.View
	if err == nil {
		err = json.Unmarshal(data, &view)
	}
	return view, err
}

func (s *Server) publishRoomChange(id string, before, after game.View) {
	if before.Version == after.Version {
		return // Using a private card must not disclose that action.
	}
	event := roomEvent{Kind: "state_changed", RoomID: id, Version: after.Version,
		Rooms: len(before.Players) != len(after.Players) || before.Phase != after.Phase}
	switch {
	case before.Phase != "settled" && after.Phase == "settled":
		event.Kind, event.Results = "settled", true // Includes the final reveal atomically.
	case before.Phase == "lobby" && after.Phase == "bidding":
		event.Kind = "started"
	case len(after.History) > len(before.History):
		event.Kind = "round_revealed"
	}
	s.publishEvent(event)
}

func (s *Server) publishEvent(event roomEvent) {
	publishTo(s.subscribers, event)
}
func publishTo(subscribers map[chan roomEvent]struct{}, event roomEvent) {
	for ch := range subscribers {
		select {
		case ch <- event:
		default:
			// A slow observer cannot block game commits. Closing forces a full sync.
			delete(subscribers, ch)
			close(ch)
		}
	}
}

// CloseEvents ends long-lived handlers before HTTP Shutdown drains requests.
func (s *Server) CloseEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventsClosed = true
	for id, timer := range s.deadlineTimers {
		timer.Stop()
		delete(s.deadlineTimers, id)
	}
	for ch := range s.subscribers {
		delete(s.subscribers, ch)
		close(ch)
	}
	for ch := range s.observerSubscribers {
		delete(s.observerSubscribers, ch)
		close(ch)
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	s.serveEvents(w, r, false)
}
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request, observer bool) {
	rc := http.NewResponseController(w)
	// Override the host's ordinary request WriteTimeout for this response only.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return
	}
	ch := make(chan roomEvent, 32)
	s.mu.Lock()
	if s.eventsClosed {
		s.mu.Unlock()
		fail(w, http.StatusServiceUnavailable, errors.New("server is shutting down"))
		return
	}
	subscribers := s.subscribers
	if observer {
		subscribers = s.observerSubscribers
	}
	subscribers[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(subscribers, ch)
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(message string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := fmt.Fprint(w, message)
		if err == nil {
			err = rc.Flush()
		}
		_ = rc.SetWriteDeadline(time.Time{})
		return err == nil
	}
	// Subscribe before the ready message so changes during HTTP sync are queued.
	if !write("retry: 2000\nevent: ready\ndata: {}\n\n") {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			if !write(fmt.Sprintf("event: %s\ndata: %s\n\n", event.Kind, data)) {
				return
			}
		case <-heartbeat.C:
			if !write(": heartbeat\n\n") {
				return
			}
		}
	}
}
