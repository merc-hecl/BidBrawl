package game

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"unicode/utf8"

	"bidbrawl/internal/cards"
)

// Snapshots are private storage records, never public observations. They retain
// randomness, pending bids, private hints and seat tokens so a restart cannot
// reroll a box, repeat a skill or change an already settled tie.
type gameSnapshot struct {
	SchemaVersion int              `json:"schema_version"`
	PlayerCount   int              `json:"player_count"`
	Config        Config           `json:"game_config"`
	Cards         cards.Config     `json:"card_config"`
	Phase         string           `json:"phase"`
	Round         int              `json:"round"`
	Version       int              `json:"version"`
	Players       []playerSnapshot `json:"players"`
	Items         []Item           `json:"items"`
	Hints         []Hint           `json:"public_hints"`
	History       []RoundResult    `json:"history"`
	Decisions     []Decision       `json:"decisions"`
	Settlement    *Settlement      `json:"settlement"`
}

type playerSnapshot struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Token     string   `json:"token"`
	Deck      []string `json:"deck"`
	Used      []string `json:"used"`
	Hints     []Hint   `json:"hints"`
	Bid       *int64   `json:"bid"`
	CardRound int      `json:"card_round"`
	Revision  int      `json:"revision"`
	Avatar    *int     `json:"avatar"`
}

func (g *Game) Snapshot() ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := gameSnapshot{SchemaVersion: 3, PlayerCount: g.maxPlayers, Config: g.gameConfig, Cards: g.cardConfig,
		Phase: g.phase, Round: g.round, Version: g.version, Players: []playerSnapshot{}, Items: g.items,
		Hints: g.hints, History: g.history, Decisions: g.decisions, Settlement: g.settlement}
	for _, p := range g.players {
		s.Players = append(s.Players, playerSnapshot{p.id, p.name, p.token, p.deck, p.used, p.hints, p.bid, p.cardRound, p.revision, p.avatar})
	}
	return json.Marshal(s)
}

// Restore validates a snapshot and builds independent state with a fresh mutex.
// It never generates items, clues, tokens or settlement randomness.
func Restore(data []byte) (*Game, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var s gameSnapshot
	if err := d.Decode(&s); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, errors.New("expected exactly one game snapshot")
	}
	if s.SchemaVersion != 3 {
		return nil, errors.New("unsupported game snapshot schema")
	}
	g, err := NewWithConfig(s.PlayerCount, s.Config, s.Cards)
	if err != nil {
		return nil, err
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid game snapshot: %w", err)
	}
	g.phase, g.round, g.version = s.Phase, s.Round, s.Version
	g.items, g.hints, g.history = s.Items, s.Hints, s.History
	g.decisions, g.settlement = s.Decisions, s.Settlement
	for _, p := range s.Players {
		g.players = append(g.players, &player{p.ID, p.Name, p.Token, p.Deck, p.Used, p.Hints, p.Bid, p.CardRound, p.Revision, p.Avatar})
	}
	return g, nil
}

func validSecret(value string, length int) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == length
}

func (s gameSnapshot) validate() error {
	if len(s.Players) > s.PlayerCount || s.Version < 1 {
		return errors.New("invalid player count or version")
	}
	known := map[string]bool{}
	for _, card := range s.Cards.Cards {
		known[card.ID] = true
	}
	names, tokens := map[string]bool{}, map[string]bool{}
	avatars := map[int]bool{}
	pending := 0
	for i, p := range s.Players {
		if s.Phase == "lobby" {
			if p.Avatar != nil {
				return errors.New("avatar assigned before start")
			}
		} else {
			if p.Avatar == nil || *p.Avatar < 0 || *p.Avatar >= 7 || avatars[*p.Avatar] {
				return errors.New("invalid or duplicate avatar")
			}
			avatars[*p.Avatar] = true
		}
		if p.ID != fmt.Sprintf("p%d", i+1) || utf8.RuneCountInString(p.Name) < 1 || utf8.RuneCountInString(p.Name) > 32 || names[p.Name] || !validSecret(p.Token, 32) || tokens[p.Token] {
			return errors.New("invalid player identity")
		}
		names[p.Name], tokens[p.Token] = true, true
		deck := map[string]bool{}
		if len(p.Deck) > 5 {
			return errors.New("invalid deck")
		}
		for _, id := range p.Deck {
			if !known[id] || deck[id] {
				return errors.New("unknown or duplicate card")
			}
			deck[id] = true
		}
		used := map[string]bool{}
		lastRound := 0
		if len(p.Hints) != len(p.Used) || p.Revision != len(p.Used) {
			return errors.New("inconsistent private card history")
		}
		for j, id := range p.Used {
			h := p.Hints[j]
			if !deck[id] || used[id] || h.Kind != id || h.Source != id || h.Data == nil || h.Round <= lastRound || h.Round > s.Round {
				return errors.New("invalid private card history")
			}
			used[id], lastRound = true, h.Round
		}
		if p.CardRound != lastRound {
			return errors.New("invalid card allowance")
		}
		if p.Bid != nil {
			if *p.Bid < 0 || *p.Bid > MaxBid {
				return errors.New("invalid pending bid")
			}
			pending++
		}
	}
	if s.Phase == "lobby" {
		if s.Round != 0 || len(s.Items) != 0 || len(s.Hints) != 0 || len(s.History) != 0 || len(s.Decisions) != 0 || s.Settlement != nil || pending != 0 || s.Version != 1+len(s.Players) {
			return errors.New("invalid lobby state")
		}
		return nil
	}
	if s.Phase != "bidding" && s.Phase != "settled" || len(s.Players) != s.PlayerCount || s.Round < 1 || s.Round > Rounds {
		return errors.New("invalid active game state")
	}
	if err := s.validateItems(); err != nil {
		return err
	}
	hintCount := 1
	if s.Round >= 3 {
		hintCount = 2
	}
	if len(s.Hints) != hintCount {
		return errors.New("invalid public hint count")
	}
	for i, h := range s.Hints {
		if h.Round != 1+2*i || h.Source != "public" || h.Data == nil || !contains([]string{"quality_average", "random_shape", "random_quality", "item_count"}, h.Kind) || i == 1 && h.Kind == s.Hints[0].Kind {
			return errors.New("invalid public hint")
		}
	}
	historyCount := s.Round - 1
	if s.Phase == "settled" {
		historyCount = s.Round
	}
	if len(s.History) != historyCount {
		return errors.New("invalid public bid history")
	}
	for i, r := range s.History {
		if r.Round != i+1 || len(r.Bids) != s.PlayerCount {
			return errors.New("incomplete published round")
		}
		for j, bid := range r.Bids {
			if bid.PlayerID != s.Players[j].ID || bid.Amount < 0 || bid.Amount > MaxBid {
				return errors.New("invalid published bid")
			}
		}
		if EarlySettlement(r) && (s.Phase != "settled" || r.Round != s.Round) {
			return errors.New("continued after instant settlement")
		}
	}
	if err := s.validateDecisions(); err != nil {
		return err
	}
	if s.Phase == "bidding" {
		if s.Settlement != nil || pending == s.PlayerCount || s.Version != 2+len(s.Players)+historyCount*s.PlayerCount+pending {
			return errors.New("invalid bidding state")
		}
		return nil
	}
	if pending != s.PlayerCount || s.Version != 2+len(s.Players)+s.Round*s.PlayerCount || s.Round < Rounds && !EarlySettlement(s.History[s.Round-1]) {
		return errors.New("invalid settled state")
	}
	r := s.Settlement
	if r == nil || r.Round != s.Round || (r.Method != "instant" || s.Round == Rounds) && (r.Method != "final" || s.Round != Rounds) || !slices.Equal(r.Items, s.Items) || len(r.Payoffs) != s.PlayerCount {
		return errors.New("invalid settlement")
	}
	var total int64
	for _, item := range s.Items {
		total += item.Price
	}
	high := int64(-1)
	ties := []string{}
	for i, bid := range s.History[s.Round-1].Bids {
		if *s.Players[i].Bid != bid.Amount {
			return errors.New("final bid differs from settlement history")
		}
		if bid.Amount > high {
			high, ties = bid.Amount, []string{bid.PlayerID}
		} else if bid.Amount == high {
			ties = append(ties, bid.PlayerID)
		}
	}
	if !contains(ties, r.WinnerID) || !slices.Equal(ties, r.Tied) || r.Paid != high || r.Total != total || r.Profit != total-high {
		return errors.New("inconsistent settlement totals or winner")
	}
	for _, p := range s.Players {
		expected := int64(0)
		if p.ID == r.WinnerID {
			expected = r.Profit
		}
		if payoff, ok := r.Payoffs[p.ID]; !ok || payoff != expected {
			return errors.New("inconsistent payoff")
		}
	}
	return nil
}

func (s gameSnapshot) validateDecisions() error {
	expected := map[string]bool{}
	for _, p := range s.Players {
		for _, h := range p.Hints {
			expected[fmt.Sprintf("%d:%s:card", h.Round, p.ID)] = true
		}
		for _, r := range s.History {
			expected[fmt.Sprintf("%d:%s:bid", r.Round, p.ID)] = true
		}
		if s.Phase == "bidding" && p.Bid != nil {
			expected[fmt.Sprintf("%d:%s:bid", s.Round, p.ID)] = true
		}
	}
	seen := map[string]bool{}
	lastRound := 0
	for _, d := range s.Decisions {
		key := fmt.Sprintf("%d:%s:%s", d.Round, d.PlayerID, d.Action)
		if ValidateReason(d.Reason) != nil || d.Round < lastRound || d.Round > s.Round || !expected[key] || seen[key] || d.Action == "card" && seen[fmt.Sprintf("%d:%s:bid", d.Round, d.PlayerID)] {
			return errors.New("invalid decision history")
		}
		lastRound, seen[key] = d.Round, true
	}
	if len(seen) != len(expected) {
		return errors.New("missing decision reason")
	}
	return nil
}

func (s gameSnapshot) validateItems() error {
	if len(s.Items) == 0 {
		return errors.New("empty box")
	}
	w, h := s.Config.Box.Width, s.Config.Box.Height
	grid := make([]bool, w*h)
	ids := map[string]bool{}
	for _, item := range s.Items {
		width, height := item.BottomRight.X-item.TopLeft.X, item.BottomRight.Y-item.TopLeft.Y
		if !validSecret(item.ID, 8) || ids[item.ID] || item.TopLeft.X < 0 || item.TopLeft.Y < 0 || item.BottomRight.X > w || item.BottomRight.Y > h || width < 1 || height < 1 || width > s.Config.Items.MaxWidth || height > s.Config.Items.MaxHeight {
			return errors.New("invalid item identity or bounds")
		}
		ids[item.ID] = true
		validPrice := false
		for _, q := range qualities {
			if item.Quality == q.ID && item.Price >= q.Min && item.Price <= q.Max {
				validPrice = true
			}
		}
		if !validPrice {
			return errors.New("invalid item quality or price")
		}
		for y := item.TopLeft.Y; y < item.BottomRight.Y; y++ {
			for x := item.TopLeft.X; x < item.BottomRight.X; x++ {
				if grid[y*w+x] {
					return errors.New("overlapping items")
				}
				grid[y*w+x] = true
			}
		}
	}
	empty := false
	for y := range h {
		count := 0
		for x := range w {
			if grid[y*w+x] {
				count++
			}
		}
		if count == 0 {
			empty = true
		} else if count != w || empty {
			return errors.New("invalid filled-row layout")
		}
	}
	return nil
}
