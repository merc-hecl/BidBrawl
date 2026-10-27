package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"bidbrawl/internal/game"
)

// Activity is a private, durable account/room projection. No activity payload,
// sequence, count, or revision is sent on the public stream.
type activity struct {
	ID         int            `json:"id"`
	AccountID  string         `json:"account_id,omitempty"`
	PlayerID   string         `json:"player_id,omitempty"`
	Round      int            `json:"round"`
	At         time.Time      `json:"at"`
	Action     string         `json:"action"`
	Tool       string         `json:"tool"`
	Source     string         `json:"source"`
	Status     string         `json:"status"`
	Code       int            `json:"code,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Cost       int64          `json:"cost,omitempty"`
}

type activityResponse struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (w *activityResponse) Header() http.Header { return w.header }
func (w *activityResponse) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}
func (w *activityResponse) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return w.body.Write(b)
}

func (s *Server) activityRoom(r *http.Request, a *account) *room {
	if id := r.URL.Query().Get("room"); id != "" {
		return s.rooms[id]
	}
	if a == nil {
		return nil
	}
	var latest *room
	for _, rm := range s.rooms {
		if rm.members[a.ID] == "" {
			continue
		}
		v, _ := publicView(rm.game)
		if v.Phase != "settled" {
			return rm
		}
		if latest == nil || rm.createdAt.After(latest.createdAt) {
			latest = rm
		}
	}
	return latest
}
func observerView(g *game.Game) game.View {
	var v game.View
	b, _ := g.ObserveObserver()
	_ = json.Unmarshal(b, &v)
	return v
}
func (s *Server) auditRequest(next http.Handler, w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/api/observer/") || r.URL.Path == "/api/activity" {
		next.ServeHTTP(w, r)
		return
	}
	s.mu.Lock()
	a := s.sessions[token(r)]
	rm := s.activityRoom(r, a)
	var before game.View
	if rm != nil {
		before = observerView(rm.game)
	}
	if r.URL.Path == "/api/events" {
		if a != nil && rm != nil && !s.eventsClosed {
			s.appendActivities(rm, []activity{{AccountID: a.ID, Round: before.Round, Action: "watch", Tool: "GET /api/events", Source: "server", Status: "connected", At: time.Now().UTC()}})
		}
		s.mu.Unlock()
		next.ServeHTTP(w, r)
		return
	}
	s.mu.Unlock()
	// Credential-bearing bodies may be decoded by the handler, but only the
	// explicit argument whitelist below can enter durable activity storage.
	params := map[string]any{}
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 65537))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var input map[string]any
		_ = json.Unmarshal(raw, &input)
		for _, key := range []string{"round", "card", "target", "quantity", "amount", "specialty", "category", "cards", "ids", "all", "players", "ticket_fee", "topic", "lang", "action"} {
			if value, ok := input[key]; ok {
				switch v := value.(type) {
				case string:
					if len(v) <= 256 {
						params[key] = v
					}
				case float64, bool:
					params[key] = v
				case []any:
					if key == "cards" || key == "ids" {
						values := []string{}
						for _, item := range v {
							if text, ok := item.(string); ok && len(text) <= 128 && len(values) < 100 {
								values = append(values, text)
							}
						}
						params[key] = values
					}
				}
			}
		}
		if value, ok := input["reason"].(string); ok && len(value) <= 8000 {
			params["reason"] = value
		}
	}
	if v := r.URL.Query().Get("frame"); v != "" {
		params["frame"] = v
	}
	if r.URL.Path == "/api/catalog" {
		for _, key := range []string{"size", "quality", "name", "category"} {
			if v := r.URL.Query().Get(key); v != "" && len(v) <= 256 {
				params[key] = v
			}
		}
	}
	start := time.Now()
	capture := &activityResponse{header: make(http.Header)}
	next.ServeHTTP(capture, r)
	if capture.code == 0 {
		capture.code = http.StatusOK
	}
	var replyBody map[string]any
	_ = json.Unmarshal(capture.body.Bytes(), &replyBody)
	s.mu.Lock()
	if a != nil && r.URL.Path == "/api/create" && capture.code < 300 {
		if id, ok := replyBody["room_id"].(string); ok {
			rm = s.rooms[id]
			before = observerView(rm.game)
		}
	}
	if a == nil && r.URL.Path == "/api/login" && capture.code < 300 {
		if tok, ok := replyBody["token"].(string); ok {
			a = s.sessions[tok]
			rm = s.activityRoom(r, a)
			if rm != nil {
				before = observerView(rm.game)
			}
		}
	}
	if rm != nil && s.rooms[rm.id] == rm && (a != nil || r.URL.Path == "/api/start") {
		after := observerView(rm.game)
		events := []activity{}
		if a != nil {
			action := strings.TrimPrefix(r.URL.Path, "/api/")
			if action == "specialty" {
				action = "specialty_action"
			}
			command := r.Header.Get("X-BidBrawl-Command")
			if r.URL.Path == "/api/config" {
				action = "catalog"
				if command == "cards" || command == "hints" || command == "config" || command == "handbook" {
					action = command
				}
			}
			if r.URL.Path == "/api/state" && (command == "cards" || command == "hints" || command == "config" || command == "handbook" || command == "catalog") {
				action = command
			}
			result := map[string]any{"accepted": capture.code < 300}
			status := "success"
			if capture.code >= 300 {
				status = "rejected"
				if msg, ok := replyBody["error"].(string); ok {
					result["message"] = msg
				}
			}
			if action == "state" {
				result["version"] = replyBody["version"]
			}
			if action == "catalog" {
				if r.URL.Path == "/api/catalog" {
					if capture.code < 300 {
						result["templates"] = replyBody["count"]
					}
				} else {
					result["templates"] = len(after.Rules.Catalog)
				}
			}
			if action == "card" && capture.code < 300 {
				for _, h := range after.Intelligence[seatForAccount(rm, a.ID)] {
					if h.Source == params["card"] {
						result["data"] = h.Data
					}
				}
			}
			if action == "specialty_action" && capture.code < 300 {
				result["specialty_state"] = after.SpecialtyStates[seatForAccount(rm, a.ID)]
			}
			reason, _ := params["reason"].(string)
			delete(params, "reason")
			cost := int64(0)
			if action == "card" && capture.code < 300 {
				seat := seatForAccount(rm, a.ID)
				card, _ := params["card"].(string)
				cost = rm.game.UsedCardPrice(seat, card)
			}
			round := before.Round
			if n, ok := replyBody["round"].(float64); ok {
				round = int(n)
			}
			if n, ok := params["round"].(float64); ok && capture.code < 300 && n >= 1 && n <= 5 {
				round = int(n)
			}
			events = append(events, activity{AccountID: a.ID, Round: round, At: start.UTC(), Action: action, Tool: r.Method + " " + r.URL.Path, Source: "server", Status: status, Code: capture.code, DurationMS: time.Since(start).Milliseconds(), Params: params, Result: result, Reason: reason, Cost: cost})
		}
		for seat, hints := range after.Intelligence {
			for _, h := range hints {
				if h.Source == "specialty" {
					events = append(events, activity{PlayerID: seat, Round: h.Round, At: time.Now().UTC(), Action: "specialty", Tool: "specialty:" + h.Kind, Source: "system", Status: "success", Result: h.Data})
				}
			}
		}
		for _, h := range after.PublicHints {
			events = append(events, activity{Round: h.Round, Action: "public_hint", Tool: h.Kind, Source: "system", Status: "success", Result: h.Data})
		}
		for _, round := range after.History {
			events = append(events, activity{Round: round.Round, At: time.Now().UTC(), Action: "round_revealed", Tool: "auction", Source: "system", Status: "success"})
		}
		if before.Phase == "lobby" && after.Phase == "bidding" {
			for seat := range rm.seatAccounts {
				events = append(events, activity{PlayerID: seat, Round: 0, At: time.Now().UTC(), Action: "ticket", Tool: "auction", Source: "system", Status: "success", Cost: after.Rules.TicketFee})
			}
		}
		if before.Phase != "settled" && after.Phase == "settled" {
			events = append(events, activity{Round: after.Round, At: time.Now().UTC(), Action: "settled", Tool: "auction", Source: "system", Status: "success"})
		}
		if !s.appendActivities(rm, events) {
			capture.header.Set("X-BidBrawl-Activity", "unavailable")
		}
	}
	s.mu.Unlock()
	for key, values := range capture.header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	if capture.code == 0 {
		capture.code = 200
	}
	w.WriteHeader(capture.code)
	_, _ = w.Write(capture.body.Bytes())
}
func seatForAccount(rm *room, id string) string {
	for seat, a := range rm.seatAccounts {
		if a == id {
			return seat
		}
	}
	return ""
}
func (s *Server) appendActivities(rm *room, events []activity) bool {
	if len(events) == 0 {
		return true
	}
	copy := *rm
	copy.activities = append([]activity{}, rm.activities...)
	for _, e := range events {
		if e.Source == "system" {
			duplicate := false
			for _, prior := range copy.activities {
				if prior.Source == "system" && prior.Action == e.Action && prior.Tool == e.Tool && prior.Round == e.Round && prior.PlayerID == e.PlayerID {
					if e.Action == "specialty" {
						a, _ := json.Marshal(prior.Result)
						b, _ := json.Marshal(e.Result)
						if !bytes.Equal(a, b) {
							continue
						}
					}
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
		}
		e.ID = len(copy.activities) + 1
		if e.At.IsZero() {
			e.At = time.Now().UTC()
		}
		copy.activities = append(copy.activities, e)
	}
	if s.store != nil {
		if err := s.store.saveRoom(&copy); err != nil {
			log.Printf("persist private activities: %v", err)
			return false
		}
	}
	rm.activities = copy.activities
	publishTo(s.observerSubscribers, roomEvent{Kind: "observer_changed", RoomID: rm.id, Version: len(rm.activities)})
	return true
}
func activityView(rm *room, seat string, maxRound int) []activity {
	out := []activity{}
	for _, e := range rm.activities {
		if e.AccountID != "" {
			e.PlayerID = seatForAccount(rm, e.AccountID)
			if e.PlayerID == "" {
				continue
			}
		}
		if seat != "" && e.PlayerID != "" && e.PlayerID != seat {
			continue
		}
		if e.Round > maxRound {
			continue
		}
		e.AccountID = ""
		if seat != "" {
			// A global sequence would reveal how many private calls other players
			// made between this player's own calls.
			e.ID = len(out) + 1
		}
		out = append(out, e)
	}
	return out
}
func (s *Server) reportActivity(w http.ResponseWriter, r *http.Request) {
	a := s.requireActor(w, r)
	if a == nil {
		return
	}
	var input struct {
		Command string `json:"command"`
		Topic   string `json:"topic"`
		Lang    string `json:"lang"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Command != "watch" && (input.Command != "handbook" || len(input.Topic) > 32 || input.Lang != "en" && input.Lang != "zh-CN") {
		fail(w, 400, io.ErrUnexpectedEOF)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[token(r)] != a {
		fail(w, 401, game.ErrUnauthorized)
		return
	}
	rm := s.activityRoom(r, a)
	if rm == nil {
		reply(w, 200, map[string]bool{"recorded": false})
		return
	}
	v, _ := publicView(rm.game)
	params := map[string]any{}
	if input.Command == "handbook" {
		params["topic"], params["lang"] = input.Topic, input.Lang
	}
	if !s.appendActivities(rm, []activity{{AccountID: a.ID, Round: v.Round, Action: input.Command, Tool: input.Command, Source: "cli", Status: "reported", Params: params}}) {
		fail(w, 500, io.ErrClosedPipe)
		return
	}
	reply(w, 200, map[string]bool{"recorded": true})
}
