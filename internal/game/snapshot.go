package game

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
	"unicode/utf8"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/hints"
)

// Snapshots are private storage records, never public observations. They retain
// randomness, pending bids, private hints and seat tokens so a restart cannot
// reroll a box, repeat a skill or change an already settled tie.
const SnapshotSchemaVersion = 11

type gameSnapshot struct {
	RoundDeadline  time.Time        `json:"round_deadline"`
	LoadoutChanges int              `json:"loadout_changes"`
	Warehouse      string           `json:"warehouse"`
	SchemaVersion  int              `json:"schema_version"`
	PlayerCount    int              `json:"player_count"`
	Config         Config           `json:"game_config"`
	Cards          cards.Config     `json:"card_config"`
	HintConfig     hints.Config     `json:"hint_config"`
	PlannedHints   []Hint           `json:"planned_hints"`
	Phase          string           `json:"phase"`
	Round          int              `json:"round"`
	Version        int              `json:"version"`
	Players        []playerSnapshot `json:"players"`
	Items          []Item           `json:"items"`
	Hints          []Hint           `json:"public_hints"`
	History        []RoundResult    `json:"history"`
	Decisions      []Decision       `json:"decisions"`
	Settlement     *Settlement      `json:"settlement"`
}

type playerSnapshot struct {
	WithdrawRound int              `json:"withdraw_round"`
	TicketCost    int64            `json:"ticket_cost"`
	Specialty     string           `json:"specialty"`
	Category      string           `json:"category"`
	Signals       []Hint           `json:"intelligence"`
	Withdrawn     bool             `json:"withdrawn"`
	CardCost      int64            `json:"card_cost"`
	CardPrices    map[string]int64 `json:"card_prices"`
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Token         string           `json:"token"`
	Deck          []string         `json:"deck"`
	Used          []string         `json:"used"`
	Hints         []Hint           `json:"hints"`
	Bid           *int64           `json:"bid"`
	CardRound     int              `json:"card_round"`
	Revision      int              `json:"revision"`
	Avatar        *int             `json:"avatar"`
	Inspection    []InspectionItem `json:"initial_inspection"`
	Assets        int64            `json:"assets"`
}

func (g *Game) Snapshot() ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := gameSnapshot{SchemaVersion: SnapshotSchemaVersion, RoundDeadline: g.roundDeadline, LoadoutChanges: g.loadoutChanges, Warehouse: g.warehouse, PlayerCount: g.maxPlayers, Config: g.gameConfig, Cards: g.cardConfig, HintConfig: g.hintConfig, PlannedHints: g.plannedHints,
		Phase: g.phase, Round: g.round, Version: g.version, Players: []playerSnapshot{}, Items: g.items,
		Hints: g.hints, History: g.history, Decisions: g.decisions, Settlement: g.settlement}
	for _, p := range g.players {
		s.Players = append(s.Players, playerSnapshot{WithdrawRound: p.withdrawRound, TicketCost: p.ticketCost, ID: p.id, Name: p.name, Token: p.token, Deck: p.deck, Used: p.used, Hints: p.hints, Bid: p.bid, CardRound: p.cardRound, Revision: p.revision, Avatar: p.avatar, Inspection: p.inspection, Assets: p.assets, Specialty: p.specialty, Category: p.category, Signals: p.signals, Withdrawn: p.withdrawn, CardCost: p.cardCost, CardPrices: p.cardPrices})
	}
	return json.Marshal(s)
}

func decodeSnapshot(data []byte) (gameSnapshot, error) {
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return gameSnapshot{}, err
	}
	if header.SchemaVersion > SnapshotSchemaVersion {
		return gameSnapshot{}, fmt.Errorf("snapshot schema %d exceeds supported schema %d: %w", header.SchemaVersion, SnapshotSchemaVersion, ErrFutureSnapshot)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var s gameSnapshot
	if err := d.Decode(&s); err != nil {
		return s, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return s, errors.New("expected exactly one game snapshot")
	}
	return s, nil
}

// Restore validates the latest format without generating or replaying effects.
func Restore(data []byte) (*Game, error) {
	s, err := decodeSnapshot(data)
	if err != nil {
		return nil, err
	}
	if s.SchemaVersion != SnapshotSchemaVersion {
		return nil, errors.New("unsupported game snapshot schema")
	}
	g, err := NewWithHints(s.PlayerCount, s.Config, s.Cards, s.HintConfig)
	if err != nil {
		return nil, err
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid game snapshot: %w", err)
	}
	g.warehouse = s.Warehouse
	g.loadoutChanges = s.LoadoutChanges
	g.phase, g.round, g.version = s.Phase, s.Round, s.Version
	g.roundDeadline = s.RoundDeadline
	g.items, g.hints, g.history = s.Items, s.Hints, s.History
	g.plannedHints = s.PlannedHints
	g.decisions, g.settlement = s.Decisions, s.Settlement
	for _, p := range s.Players {
		g.players = append(g.players, &player{withdrawRound: p.WithdrawRound, ticketCost: p.TicketCost, id: p.ID, name: p.Name, token: p.Token, deck: p.Deck, used: p.Used, hints: p.Hints, bid: p.Bid, cardRound: p.CardRound, revision: p.Revision, avatar: p.Avatar, inspection: p.Inspection, assets: p.Assets, specialty: p.Specialty, category: p.Category, signals: p.Signals, withdrawn: p.Withdrawn, cardCost: p.CardCost, cardPrices: p.CardPrices})
	}
	return g, nil
}

func validSecret(value string, length int) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == length
}

func (s gameSnapshot) validate() error {
	if (s.Phase == "bidding") == s.RoundDeadline.IsZero() {
		return errors.New("invalid round deadline")
	}
	if !ValidWarehouse(s.Warehouse) || len(s.Players) > s.PlayerCount || s.Version < 1 {
		return errors.New("invalid player count or version")
	}
	known := map[string]bool{}
	for _, card := range s.Cards.Cards {
		known[card.ID] = true
	}
	names, tokens := map[string]bool{}, map[string]bool{}
	avatars := map[int]bool{}
	pending := 0
	items := make(map[string]Item, len(s.Items))
	for _, item := range s.Items {
		items[item.ID] = item
	}
	for i, p := range s.Players {
		if p.TicketCost != func() int64 {
			if s.Phase == "lobby" {
				return 0
			}
			return s.Config.TicketFee
		}() || p.Withdrawn != (p.WithdrawRound > 0) || p.WithdrawRound < 0 || p.WithdrawRound > s.Round || p.Assets < 0 || !validSelection(s.Config.Specialties, p.Specialty, p.Category) || p.CardCost < 0 {
			return errors.New("invalid player assets")
		}
		count := 0
		if s.Phase != "lobby" {
			count = min(InitialInspectionCount, len(scoutableItems(s.Items)))
		}
		if len(p.Inspection) != count {
			return errors.New("invalid initial inspection count")
		}
		seen := map[string]bool{}
		for _, observed := range p.Inspection {
			item, ok := items[observed.ID]
			if !ok || isEasterEgg(item) || seen[observed.ID] || observed != (InspectionItem{item.ID, item.Quality, item.TopLeft, item.BottomRight}) {
				return errors.New("invalid initial inspection item")
			}
			seen[observed.ID] = true
		}
		if p.Avatar == nil || *p.Avatar < 0 || *p.Avatar >= 7 || avatars[*p.Avatar] {
			return errors.New("invalid or duplicate avatar")
		}
		avatars[*p.Avatar] = true
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
			for _, d := range s.Cards.Cards {
				if d.ID != id {
					continue
				}
				target, _ := h.Data["target"].(string)
				if _, err := d.ResolveTarget(target); err != nil || h.Round < d.MinRound {
					return errors.New("invalid stored card target or unlock round")
				}
			}
			used[id], lastRound = true, h.Round
		}
		var spent int64
		for _, id := range p.Used {
			price, ok := p.CardPrices[id]
			if !ok {
				for _, d := range s.Cards.Cards {
					if d.ID == id {
						price = d.Price
					}
				}
			}
			spent += price
		}
		if spent != p.CardCost {
			return errors.New("card cost mismatch")
		}
		for _, h := range p.Signals {
			if h.Source != "specialty" || h.Kind != p.Specialty || h.Round < 1 || h.Round > s.Round || h.Data == nil {
				return errors.New("invalid specialty intelligence")
			}
		}
		if p.CardRound != lastRound {
			return errors.New("invalid card allowance")
		}
		if p.Bid != nil {
			if *p.Bid < 0 || *p.Bid > p.Assets || s.Phase == "bidding" && *p.Bid == 0 && !p.Withdrawn {
				return errors.New("invalid pending bid")
			}
			pending++
		}
	}
	if s.Phase == "lobby" {
		if len(s.PlannedHints) != 0 {
			return errors.New("clues generated before start")
		}
		if s.Round != 0 || len(s.Items) != 0 || len(s.Hints) != 0 || len(s.History) != 0 || len(s.Decisions) != 0 || s.Settlement != nil || pending != 0 || s.Version != 1+len(s.Players)+s.LoadoutChanges {
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
	if len(s.PlannedHints) != 2 {
		return errors.New("invalid planned hint count")
	}
	for i, h := range s.PlannedHints {
		if h.Round != 1+2*i || h.Source != "public" || h.Data == nil || !contains(s.HintConfig.IDs(), h.Kind) || i == 1 && h.Kind == s.PlannedHints[0].Kind {
			return errors.New("invalid public hint")
		}
		data, err := json.Marshal(h.Data)
		if err != nil {
			return err
		}
		_, err = hints.ParseClue(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if i < hintCount {
			planned, _ := json.Marshal(h)
			published, _ := json.Marshal(s.Hints[i])
			if !bytes.Equal(planned, published) {
				return errors.New("public hint differs from planned hint")
			}
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
			if bid.PlayerID != s.Players[j].ID || bid.Amount < 0 || bid.Amount > s.Players[j].Assets || s.Players[j].WithdrawRound > 0 && s.Players[j].WithdrawRound <= r.Round && bid.Amount != 0 {
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
		if s.Settlement != nil || pending == s.PlayerCount || s.Version != 2+len(s.Players)+historyCount*s.PlayerCount+pending+s.LoadoutChanges {
			return errors.New("invalid bidding state")
		}
		return nil
	}
	if pending != s.PlayerCount || s.Version != 2+len(s.Players)+s.Round*s.PlayerCount+s.LoadoutChanges || s.Round < Rounds && !EarlySettlement(s.History[s.Round-1]) && s.Settlement.Method != "unsold" {
		return errors.New("invalid settled state")
	}
	r := s.Settlement
	if r == nil || r.Round != s.Round || (r.Method != "instant" || s.Round == Rounds) && (r.Method != "final" || s.Round != Rounds) && r.Method != "unsold" || !slices.Equal(r.Items, s.Items) || len(r.Payoffs) != s.PlayerCount {
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
	var costTotal int64
	var ticketTotal int64
	for _, p := range s.Players {
		costTotal += p.CardCost
		ticketTotal += p.TicketCost
	}
	gross := int64(0)
	if high > 0 {
		gross = total - high
	}
	if (high > 0 && !contains(ties, r.WinnerID) || high == 0 && (r.WinnerID != "" || r.Method != "unsold")) || !slices.Equal(ties, r.Tied) || r.Paid != high || r.Total != total || r.GrossProfit != gross || r.TotalCardCost != costTotal || r.TotalTicketCost != ticketTotal || r.Profit != gross-costTotal-ticketTotal {
		return errors.New("inconsistent settlement totals or winner")
	}
	for _, p := range s.Players {
		expected := -p.CardCost - p.TicketCost
		if r.TicketCosts[p.ID] != p.TicketCost {
			return errors.New("invalid ticket cost")
		}
		if r.CardCosts[p.ID] != p.CardCost {
			return errors.New("invalid card cost")
		}
		cash := int64(0)
		if p.ID == r.WinnerID {
			cash = -r.Paid
			expected += r.GrossProfit
		}
		if r.CashChanges[p.ID] != cash {
			return errors.New("invalid cash change")
		}
		if payoff, ok := r.Payoffs[p.ID]; !ok || payoff != expected {
			return errors.New("inconsistent payoff")
		}
	}
	return nil
}

func (s gameSnapshot) validateDecisions() error {
	expected := map[string]bool{}
	for i, p := range s.Players {
		for _, h := range p.Hints {
			expected[fmt.Sprintf("%d:%s:card", h.Round, p.ID)] = true
		}
		for _, r := range s.History {
			action := "bid"
			if p.WithdrawRound == r.Round {
				action = "withdraw"
			} else if r.Bids[i].Amount == 0 && (p.WithdrawRound == 0 || p.WithdrawRound > r.Round) {
				action = "timeout"
			}
			expected[fmt.Sprintf("%d:%s:%s", r.Round, p.ID, action)] = true
		}
		if s.Phase == "bidding" && p.Bid != nil {
			action := "bid"
			if p.WithdrawRound == s.Round {
				action = "withdraw"
			} else if *p.Bid == 0 && !p.Withdrawn {
				action = "timeout"
			}
			expected[fmt.Sprintf("%d:%s:%s", s.Round, p.ID, action)] = true
		}
	}
	seen := map[string]bool{}
	lastRound := 0
	for _, d := range s.Decisions {
		key := fmt.Sprintf("%d:%s:%s", d.Round, d.PlayerID, d.Action)
		if ValidateReason(d.Reason) != nil || d.Round < lastRound || d.Round > s.Round || !expected[key] || seen[key] || d.Action == "card" && (seen[fmt.Sprintf("%d:%s:bid", d.Round, d.PlayerID)] || seen[fmt.Sprintf("%d:%s:withdraw", d.Round, d.PlayerID)]) {
			return errors.New("invalid decision history")
		}
		lastRound, seen[key] = d.Round, true
	}
	for _, p := range s.Players {
		if p.WithdrawRound > 0 && !seen[fmt.Sprintf("%d:%s:withdraw", p.WithdrawRound, p.ID)] {
			return errors.New("withdrawal requires an explicit player decision")
		}
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
		templateValid := false
		for _, t := range Catalog(s.Config) {
			if t.ID == item.TemplateID && t.Name == item.Name && t.Category == item.Category && t.Quality == item.Quality && t.Price == item.Price && t.Width == width && t.Height == height {
				templateValid = true
				break
			}
		}
		if !templateValid {
			return errors.New("invalid catalog item")
		}
		for _, q := range s.Config.Qualities {
			if item.Quality == q.ID && item.Price >= q.Min && item.Price <= q.Max {
				validPrice = true
			}
		}
		if item.Category == "easter_egg" && item.Price == 17500000 {
			validPrice = true
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
