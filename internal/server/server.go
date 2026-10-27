package server

import (
	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	"bidbrawl/internal/hints"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

//go:embed web/*
var assets embed.FS

// AppIcon is the shared desktop and WebUI identity.
func AppIcon() []byte {
	data, _ := assets.ReadFile("web/brand-avatar.png")
	return data
}

type account struct {
	CardStock map[string][]int64 `json:"cards"`
	Warehouse []storedItem       `json:"warehouse"`
	ReliefAt  string             `json:"relief_claimed_at,omitempty"`
	ID        string             `json:"account_id"`
	Name      string             `json:"name"`
	Assets    int64              `json:"assets"`
}
type accountProfile struct {
	account
	Stats agentMetrics `json:"stats"`
}
type room struct {
	activities      []activity
	id, name, owner string
	game            *game.Game
	members         map[string]string
	seatAccounts    map[string]string
	createdAt       time.Time
	settledAt       time.Time
	archivedAt      time.Time
}
type Server struct {
	unavailableRooms    map[string]*unavailableRoom
	deadlineTimers      map[string]*time.Timer
	HostToken           string
	ObserverToken       string
	UIToken             string
	webUI               bool
	observerLaunchCode  string
	observerLaunchUntil time.Time
	mu                  sync.Mutex
	accounts            map[string]*account
	accountsByID        map[string]*account
	credentials         map[[32]byte]*account
	credentialHashes    map[string][32]byte
	store               *persistentStore
	sessions            map[string]*account
	rooms               map[string]*room
	deletedResults      map[string]roomResult
	cardConfig          cards.Config
	gameConfig          game.Config
	hintConfig          hints.Config
	handler             http.Handler
	subscribers         map[chan roomEvent]struct{}
	observerSubscribers map[chan roomEvent]struct{}
	eventsClosed        bool
}

func secret() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

// New creates an ephemeral server for isolated protocol tests. Hosts use Open.
func New(geometry game.Config, cfg cards.Config) *Server {
	return NewWithHints(geometry, cfg, hints.Default())
}

func NewWithHints(geometry game.Config, cfg cards.Config, hintConfig hints.Config) *Server {
	s := &Server{HostToken: secret(), UIToken: secret(), accounts: map[string]*account{}, accountsByID: map[string]*account{}, credentials: map[[32]byte]*account{}, credentialHashes: map[string][32]byte{}, sessions: map[string]*account{}, rooms: map[string]*room{}, cardConfig: cfg, gameConfig: geometry.Clone(), hintConfig: hintConfig.Clone()}
	s.subscribers = make(map[chan roomEvent]struct{})
	s.deadlineTimers = make(map[string]*time.Timer)
	s.unavailableRooms = make(map[string]*unavailableRoom)
	s.deletedResults = make(map[string]roomResult)
	s.ObserverToken = secret()
	s.observerSubscribers = make(map[chan roomEvent]struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("GET /api/accounts", s.listAccounts)
	mux.HandleFunc("POST /api/accounts/{id}/credential", s.rotateCredential)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/inventory", s.inventory)
	mux.HandleFunc("POST /api/buy", s.buy)
	mux.HandleFunc("POST /api/sell", s.sell)
	mux.HandleFunc("POST /api/relief", s.relief)
	mux.HandleFunc("POST /api/loadout", s.loadout)
	mux.HandleFunc("GET /api/rooms", s.listRooms)
	mux.HandleFunc("POST /api/rooms/manage", s.manageRooms)
	mux.HandleFunc("GET /api/results", s.listResults)
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /api/leaderboard", s.leaderboard)
	mux.HandleFunc("GET /api/agents/{id}", s.agentStats)
	mux.HandleFunc("GET /api/replay", s.replay)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/catalog", s.catalog)
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"game": s.gameConfig, "qualities": game.CatalogQualities(s.gameConfig), "cards": s.cardConfig, "public_hint_plugins": s.hintConfig.IDs(), "public_hint_definitions": s.hintConfig.Definitions(), "specialties": s.gameConfig.Specialties, "categories": game.Categories, "catalog": game.Catalog(s.gameConfig), "warehouses": game.Warehouses})
	})
	mux.HandleFunc("POST /api/invite", s.invite)
	mux.HandleFunc("POST /api/observer/launch", s.observerLaunch)
	mux.HandleFunc("POST /api/observer/login", s.observerLogin)
	mux.HandleFunc("GET /api/observer/replay", s.observerReplay)
	mux.HandleFunc("POST /api/observer/bootstrap", s.observerBootstrap)
	mux.HandleFunc("GET /api/observer/session", s.observerSession)
	mux.HandleFunc("GET /api/observer/state", s.observerState)
	mux.HandleFunc("GET /api/observer/events", s.observerEvents)
	mux.HandleFunc("POST /api/create", s.create)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("POST /api/join", s.join)
	mux.HandleFunc("POST /api/start", s.start)
	mux.HandleFunc("POST /api/card", s.card)
	mux.HandleFunc("POST /api/specialty", s.specialty)
	mux.HandleFunc("POST /api/bid", s.bid)
	mux.HandleFunc("POST /api/withdraw", s.withdraw)
	mux.HandleFunc("POST /api/activity", s.reportActivity)
	web, _ := fs.Sub(assets, "web")
	files := http.FileServer(http.FS(web))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if !s.webUI {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/invite" {
			http.ServeFileFS(w, r, web, "index.html")
			return
		}
		files.ServeHTTP(w, r)
	})
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			origin := r.Header.Get("Origin")
			if r.Method != "GET" && origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
				fail(w, 403, errors.New("cross-origin request denied"))
				return
			}
		}
		s.auditRequest(mux, w, r)
	})
	return s
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }
func (s *Server) EnableWebUI()                                     { s.webUI = true }
func token(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	reply(w, status, map[string]string{"error": e.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fail(w, 400, e)
		return false
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		fail(w, 400, errors.New("expected exactly one JSON object"))
		return false
	}
	return true
}
func validName(name string) bool {
	if strings.TrimSpace(name) != name || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 32 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *Server) actor(r *http.Request) *account {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[token(r)]
}
func (s *Server) requireActor(w http.ResponseWriter, r *http.Request) *account {
	a := s.actor(r)
	if a == nil {
		fail(w, 401, errors.New("login session required"))
	}
	return a
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := token(r)
	if s.sessions[t] == nil {
		fail(w, 401, errors.New("login session required"))
		return
	}
	delete(s.sessions, t)
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	a := s.requireActor(w, r)
	if a != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.sessions[token(r)] != a {
			fail(w, 401, errors.New("login session required"))
			return
		}
		stats, err := s.accountStatsLocked(a)
		if err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, accountProfile{account: *a, Stats: stats})
	}
}
func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) *room {
	id := r.URL.Query().Get("room")
	if id == "" {
		fail(w, 400, errors.New("room is required"))
		return nil
	}
	s.mu.Lock()
	rm := s.rooms[id]
	unavailable := s.unavailableRooms[id] != nil
	s.mu.Unlock()
	if unavailable {
		fail(w, 503, errors.New("room cannot be recovered; original data has been preserved"))
		return nil
	}
	if rm == nil {
		fail(w, 404, errors.New("room not found"))
	}
	return rm
}
func (s *Server) listRooms(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := []string{}
	for id := range s.rooms {
		ids = append(ids, id)
	}
	for id := range s.unavailableRooms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := []map[string]any{}
	for _, id := range ids {
		if rm := s.unavailableRooms[id]; rm != nil {
			list = append(list, map[string]any{"room_id": id, "name": rm.name, "phase": "unavailable", "players": 0, "player_count": 0, "created_at": rm.createdAt})
			continue
		}
		rm := s.rooms[id]
		b, _ := rm.game.Observe("")
		var v game.View
		_ = json.Unmarshal(b, &v)
		entry := map[string]any{"room_id": id, "name": rm.name, "phase": v.Phase, "players": len(v.Players), "player_count": v.Rules.PlayerCount, "created_at": rm.createdAt, "ticket_fee": v.Rules.TicketFee}
		if !rm.archivedAt.IsZero() {
			entry["archived_at"] = rm.archivedAt
		}
		list = append(list, entry)
	}
	reply(w, 200, map[string]any{"rooms": list, "warehouses": game.Warehouses})
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var a *account
	if !s.isRoomManager(r) {
		a = s.requireActor(w, r)
		if a == nil {
			return
		}
	}
	var v struct {
		Name    string `json:"name"`
		Players int    `json:"players"`
		Ticket  *int64 `json:"ticket_fee"`
	}
	if !decode(w, r, &v) {
		return
	}
	if !validName(v.Name) {
		fail(w, 400, errors.New("room name must contain 1 to 32 non-control characters"))
		return
	}
	geometry := s.gameConfig.Clone()
	if v.Ticket != nil {
		geometry.TicketFee = *v.Ticket
	}
	g, e := game.NewWithHints(v.Players, geometry, s.cardConfig, s.hintConfig)
	if e != nil {
		fail(w, 400, e)
		return
	}
	warehouse, err := rand.Int(rand.Reader, big.NewInt(int64(len(game.Warehouses))))
	if err != nil {
		roomFailure(w, err)
		return
	}
	if err := g.SetWarehouse(game.Warehouses[warehouse.Int64()].ID); err != nil {
		roomFailure(w, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if a != nil && s.sessions[token(r)] != a {
		fail(w, 401, errors.New("login session required"))
		return
	}
	id := secret()[:16]
	for s.rooms[id] != nil || s.unavailableRooms[id] != nil || s.deletedResults[id].RoomID != "" {
		id = secret()[:16]
	}
	owner := ""
	if a != nil {
		owner = a.ID
	}
	rm := &room{id: id, name: v.Name, owner: owner, game: g, members: map[string]string{}, seatAccounts: map[string]string{}, createdAt: time.Now().UTC()}
	public, err := publicView(g)
	if err != nil {
		roomFailure(w, err)
		return
	}
	if s.store != nil {
		if err := s.store.saveRoom(rm); err != nil {
			roomFailure(w, err)
			return
		}
	}
	s.rooms[id] = rm
	s.publishEvent(roomEvent{Kind: "room_created", RoomID: id, Version: public.Version, Rooms: true})
	publishTo(s.observerSubscribers, roomEvent{Kind: "room_created", RoomID: id, Version: public.Version, Rooms: true})
	reply(w, 201, map[string]any{"room_id": id, "name": v.Name, "players": v.Players, "owner_id": owner, "invite_path": "/invite?room=" + id})
}
func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	s.stateFor(w, r, false)
}
func (s *Server) stateFor(w http.ResponseWriter, r *http.Request, observer bool) {
	rm := s.getRoom(w, r)
	if rm == nil {
		return
	}
	t := ""
	raw := token(r)
	var a *account
	if !observer && raw != "" && subtle.ConstantTimeCompare([]byte(raw), []byte(s.HostToken)) != 1 {
		a = s.requireActor(w, r)
		if a == nil {
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.roomExists(w, rm) {
		return
	}
	if a != nil {
		if s.sessions[raw] != a {
			fail(w, 401, errors.New("login session required"))
			return
		}
		t = rm.members[a.ID]
	}
	b, e := rm.game.Observe(t)
	if observer {
		b, e = rm.game.ObserveObserver()
	}
	if e != nil {
		fail(w, 401, e)
		return
	}
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	if a != nil && t != "" {
		self := v["self"].(map[string]any)
		self["assets"], self["max_bid"] = a.Assets, a.Assets
		self["cash"] = a.Assets
	}
	v["room_id"] = rm.id
	v["room_name"] = rm.name
	v["owner_id"] = rm.owner
	if observer || a != nil && a.ID == rm.owner {
		v["can_manage"] = s.isRoomManager(r) || a != nil && a.ID == rm.owner
	}
	for _, rawPlayer := range v["players"].([]any) {
		player := rawPlayer.(map[string]any)
		player["agent_id"] = "account:" + rm.seatAccounts[player["id"].(string)]
	}
	v["created_at"] = rm.createdAt
	if !rm.archivedAt.IsZero() {
		v["archived_at"] = rm.archivedAt
		v["can_manage"] = false
	}
	if !rm.settledAt.IsZero() {
		v["settled_at"] = rm.settledAt
	}
	if observer {
		v["activities"] = activityView(rm, "", 6)
	} else if a != nil && t != "" {
		v["self"].(map[string]any)["activities"] = activityView(rm, seatForAccount(rm, a.ID), 6)
	}
	reply(w, 200, v)
}
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	a := s.requireActor(w, r)
	if a == nil {
		return
	}
	var v struct {
		Cards     []string `json:"cards"`
		Specialty string   `json:"specialty"`
		Category  string   `json:"category"`
	}
	if !decode(w, r, &v) {
		return
	}
	rm := s.getRoom(w, r)
	if rm == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[token(r)] != a {
		fail(w, 401, errors.New("login session required"))
		return
	}
	if rm.members[a.ID] != "" {
		fail(w, 409, errors.New("already joined this room; read state to resume"))
		return
	}
	if s.recoveryLocked(a.ID) {
		fail(w, 409, errors.New("account has an unrecovered room; assets remain locked"))
		return
	}
	for _, other := range s.rooms {
		if other.members[a.ID] == "" {
			continue
		}
		view, err := publicView(other.game)
		if err != nil {
			roomFailure(w, err)
			return
		}
		if view.Phase != "settled" {
			fail(w, 409, errors.New("already joined an unfinished room: "+other.id))
			return
		}
	}
	if err := s.checkDeck(a, v.Cards); err != nil {
		fail(w, 409, err)
		return
	}
	var id string
	if !s.mutateRoom(w, rm, func(next *room) error {
		var t string
		var err error
		id, t, err = next.game.Join(a.Name, v.Cards)
		if err == nil {
			next.members[a.ID] = t
			next.seatAccounts[id] = a.ID
			if v.Specialty != "" {
				err = next.game.Configure(t, v.Specialty, v.Category, v.Cards)
			}
		}
		return err
	}) {
		return
	}
	reply(w, 201, map[string]string{"room_id": rm.id, "player_id": id, "account_id": a.ID})
}
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	rm := s.getRoom(w, r)
	if rm == nil {
		return
	}
	var a *account
	if !s.isRoomManager(r) {
		a = s.requireActor(w, r)
		if a == nil {
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if a != nil {
		if s.sessions[token(r)] != a {
			fail(w, 401, errors.New("login session required"))
			return
		}
		if a.ID != rm.owner {
			fail(w, 403, errors.New("room owner required"))
			return
		}
	}
	prices := map[string]map[string]int64{}
	balances := make(map[string]int64, len(rm.seatAccounts))
	for seat, id := range rm.seatAccounts {
		actor := s.accountsByID[id]
		raw, _ := rm.game.Observe(rm.members[id])
		var own game.View
		_ = json.Unmarshal(raw, &own)
		if err := s.checkDeck(actor, own.Self.Deck); err != nil {
			fail(w, 409, err)
			return
		}
		prices[seat] = map[string]int64{}
		for _, card := range own.Self.Deck {
			prices[seat][card] = actor.CardStock[card][0]
		}
		balances[seat] = actor.Assets
	}
	if !s.mutateRoom(w, rm, func(next *room) error { next.game.SetCardPrices(prices); return next.game.StartWithAssets(balances) }) {
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) playerAction(w http.ResponseWriter, r *http.Request, apply func(*game.Game, string) error) bool {
	a := s.requireActor(w, r)
	if a == nil {
		return false
	}
	rm := s.getRoom(w, r)
	if rm == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[token(r)] != a {
		fail(w, 401, errors.New("login session required"))
		return false
	}
	t := rm.members[a.ID]
	if t == "" {
		fail(w, 403, errors.New("join the room before acting"))
		return false
	}
	return s.mutateRoom(w, rm, func(next *room) error { return apply(next.game, t) })
}
func (s *Server) specialty(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Round  int    `json:"round"`
		Action string `json:"action"`
		Topic  string `json:"topic"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &v) {
		return
	}
	if err := game.ValidateReason(v.Reason); err != nil {
		fail(w, 400, err)
		return
	}
	if s.playerAction(w, r, func(g *game.Game, t string) error { return g.SpecialtyAction(t, v.Round, v.Action, v.Topic, v.Reason) }) {
		s.state(w, r)
	}
}

func (s *Server) card(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Round  int    `json:"round"`
		Card   string `json:"card"`
		Target string `json:"target"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &v) {
		return
	}
	if err := game.ValidateReason(v.Reason); err != nil {
		fail(w, 400, err)
		return
	}
	if s.playerAction(w, r, func(g *game.Game, t string) error { return g.UseCard(t, v.Round, v.Card, v.Target, v.Reason) }) {
		s.state(w, r)
	}
}
func (s *Server) withdraw(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Round  int    `json:"round"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &v) {
		return
	}
	if err := game.ValidateReason(v.Reason); err != nil {
		fail(w, 400, err)
		return
	}
	if s.playerAction(w, r, func(g *game.Game, t string) error { return g.Withdraw(t, v.Round, v.Reason) }) {
		s.state(w, r)
	}
}
func (s *Server) bid(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Round  int    `json:"round"`
		Amount *int64 `json:"amount"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &v) {
		return
	}
	if v.Amount == nil {
		fail(w, 400, errors.New("amount is required; use withdraw to exit"))
		return
	}
	if err := game.ValidateReason(v.Reason); err != nil {
		fail(w, 400, err)
		return
	}
	if s.playerAction(w, r, func(g *game.Game, t string) error { return g.Submit(t, v.Round, *v.Amount, v.Reason) }) {
		s.state(w, r)
	}
}
