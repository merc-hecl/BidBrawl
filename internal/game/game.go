// Package game implements the authoritative auction, independent of transports and LLMs.
package game

import (
	"bidbrawl/internal/cards"
	"bidbrawl/internal/hints"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const Rounds = 5
const InitialInspectionCount = 3

// Point is an integer grid boundary: x counts columns, y counts rows.
// Item bounds are half-open; (0,0) to (1,1) occupies exactly one cell.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type Item struct {
	ID          string `json:"id"`
	Quality     string `json:"quality"`
	Price       int64  `json:"price"`
	TopLeft     Point  `json:"top_left"`
	BottomRight Point  `json:"bottom_right"`
}

func (i Item) area() int { return (i.BottomRight.X - i.TopLeft.X) * (i.BottomRight.Y - i.TopLeft.Y) }

type Quality struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Min    int64  `json:"min_price"`
	Max    int64  `json:"max_price"`
	Weight int    `json:"weight"`
}

type Hint struct {
	Round  int            `json:"round"`
	Kind   string         `json:"kind"`
	Source string         `json:"source"`
	Data   map[string]any `json:"data"`
}
type Bid struct {
	PlayerID string `json:"player_id"`
	Amount   int64  `json:"amount"`
}
type RoundResult struct {
	Round int   `json:"round"`
	Bids  []Bid `json:"bids"`
}
type PlayerInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Submitted bool   `json:"submitted"`
	Avatar    *int   `json:"avatar,omitempty"`
}
type Self struct {
	ID                string           `json:"id"`
	Deck              []string         `json:"deck"`
	Used              []string         `json:"used"`
	Hints             []Hint           `json:"hints"`
	Bid               *int64           `json:"bid,omitempty"`
	CardUsedThisRound bool             `json:"card_used_this_round"`
	MinBid            int64            `json:"min_bid"`
	MaxBid            int64            `json:"max_bid"`
	Assets            int64            `json:"assets"`
	Decisions         []Decision       `json:"decisions"`
	InitialInspection []InspectionItem `json:"initial_inspection"`
	ItemReveals       []ItemReveal     `json:"item_reveals"`
}

// InspectionItem deliberately has no price field.
type InspectionItem struct {
	ID          string `json:"id"`
	Quality     string `json:"quality"`
	TopLeft     Point  `json:"top_left"`
	BottomRight Point  `json:"bottom_right"`
}

// A single-item skill locates its target. Quality and price are included only
// when the skill learned them; aggregate results do not locate any items.
type ItemReveal struct {
	Round       int    `json:"round"`
	ItemID      string `json:"item_id"`
	Quality     string `json:"quality,omitempty"`
	Price       *int64 `json:"price,omitempty"`
	TopLeft     *Point `json:"top_left,omitempty"`
	BottomRight *Point `json:"bottom_right,omitempty"`
}
type Decision struct {
	Round    int    `json:"round"`
	PlayerID string `json:"player_id"`
	Action   string `json:"action"`
	Reason   string `json:"reason"`
}
type ObserverDecision struct {
	Decision
	CardID string `json:"card_id,omitempty"`
}
type Settlement struct {
	WinnerID string           `json:"winner_id"`
	Paid     int64            `json:"paid"`
	Total    int64            `json:"total_value"`
	Profit   int64            `json:"profit"`
	Payoffs  map[string]int64 `json:"payoffs"`
	Tied     []string         `json:"tied_players"`
	Items    []Item           `json:"items"`
	Round    int              `json:"round"`
	Method   string           `json:"method"`
}
type Rules struct {
	Rounds                 int                `json:"rounds"`
	Width                  int                `json:"box_width"`
	Height                 int                `json:"box_height"`
	CoordinateUnit         string             `json:"coordinate_unit"`
	GameConfig             Config             `json:"game_config"`
	PlayerCount            int                `json:"player_count"`
	Qualities              []Quality          `json:"qualities"`
	Cards                  []cards.Definition `json:"cards"`
	CardsSchemaVersion     int                `json:"cards_schema_version"`
	DefaultDeck            []string           `json:"default_deck"`
	PublicHintPlugins      []string           `json:"public_hint_plugins"`
	Description            string             `json:"description"`
	EarlyMultipliers       []float64          `json:"early_settlement_multipliers"`
	InitialInspectionCount int                `json:"initial_inspection_count"`
}
type View struct {
	Version            int                         `json:"version"`
	Phase              string                      `json:"phase"`
	Round              int                         `json:"round"`
	Rules              Rules                       `json:"rules"`
	Players            []PlayerInfo                `json:"players"`
	PublicHints        []Hint                      `json:"public_hints"`
	History            []RoundResult               `json:"history"`
	Decisions          []ObserverDecision          `json:"decisions,omitempty"`
	Self               *Self                       `json:"self,omitempty"`
	Settlement         *Settlement                 `json:"settlement,omitempty"`
	InitialInspections map[string][]InspectionItem `json:"initial_inspections,omitempty"`
	ItemReveals        map[string][]ItemReveal     `json:"item_reveals,omitempty"`
}
type player struct {
	id, name, token string
	deck, used      []string
	hints           []Hint
	bid             *int64
	cardRound       int
	revision        int
	avatar          *int
	inspection      []InspectionItem
	assets          int64
}
type Game struct {
	mu                                        sync.Mutex
	maxPlayers, width, height, round, version int
	phase                                     string
	players                                   []*player
	items                                     []Item
	hints                                     []Hint
	history                                   []RoundResult
	decisions                                 []Decision
	settlement                                *Settlement
	cardConfig                                cards.Config
	gameConfig                                Config
	hintConfig                                hints.Config
	plannedHints                              []Hint
}

var ErrUnauthorized = errors.New("invalid player token")

func New(maxPlayers int) (*Game, error) {
	return NewWithCards(maxPlayers, cards.Default())
}

func NewWithCards(maxPlayers int, cfg cards.Config) (*Game, error) {
	return NewWithConfig(maxPlayers, DefaultConfig(), cfg)
}

func NewWithConfig(maxPlayers int, geometry Config, cfg cards.Config) (*Game, error) {
	return NewWithHints(maxPlayers, geometry, cfg, hints.Default())
}

func NewWithHints(maxPlayers int, geometry Config, cfg cards.Config, hintConfig hints.Config) (*Game, error) {
	if maxPlayers < 2 || maxPlayers > 7 {
		return nil, errors.New("players must be between 2 and 7")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := geometry.Validate(); err != nil {
		return nil, err
	}
	if err := hintConfig.Validate(); err != nil {
		return nil, err
	}
	return &Game{maxPlayers: maxPlayers, width: geometry.Box.Width, height: geometry.Box.Height, phase: "lobby", version: 1, players: []*player{}, hints: []Hint{}, history: []RoundResult{}, decisions: []Decision{}, cardConfig: cfg.Clone(), gameConfig: geometry.Clone(), hintConfig: hintConfig.Clone()}, nil
}

func (g *Game) GameConfig() Config {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.gameConfig.Clone()
}

func (g *Game) CardConfig() cards.Config {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cardConfig.Clone()
}
func secret() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func random(n int) int {
	v, e := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if e != nil {
		panic(e)
	}
	return int(v.Int64())
}
func (g *Game) Join(name string, deck []string) (string, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "lobby" {
		return "", "", errors.New("game already started")
	}
	if len(g.players) >= g.maxPlayers {
		return "", "", errors.New("room is full")
	}
	if len(name) == 0 || utf8.RuneCountInString(name) > 32 {
		return "", "", errors.New("name must contain 1 to 32 characters")
	}
	if len(deck) > 5 {
		return "", "", errors.New("at most 5 different cards")
	}
	known := map[string]bool{}
	for _, c := range g.cardConfig.Cards {
		known[c.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range deck {
		if !known[id] || seen[id] {
			return "", "", fmt.Errorf("unknown or duplicate card: %s", id)
		}
		seen[id] = true
	}
	for _, p := range g.players {
		if p.name == name {
			return "", "", errors.New("name already in use")
		}
	}
	available := []int{0, 1, 2, 3, 4, 5, 6}
	for _, p := range g.players {
		for i, avatar := range available {
			if avatar == *p.avatar {
				available = append(available[:i], available[i+1:]...)
				break
			}
		}
	}
	avatar := available[random(len(available))]
	p := &player{id: fmt.Sprintf("p%d", len(g.players)+1), name: name, token: secret(), deck: append([]string{}, deck...), used: []string{}, hints: []Hint{}, avatar: &avatar, assets: g.gameConfig.InitialAssets}
	g.players = append(g.players, p)
	g.version++
	return p.id, p.token, nil
}
func (g *Game) Start() error { return g.StartWithAssets(nil) }

// StartWithAssets fixes each seat's account balance for this auction.
func (g *Game) StartWithAssets(balances map[string]int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "lobby" {
		return errors.New("game already started")
	}
	if len(g.players) != g.maxPlayers {
		return fmt.Errorf("waiting for all %d configured players", g.maxPlayers)
	}
	if balances != nil {
		if len(balances) != len(g.players) {
			return errors.New("missing player assets")
		}
		for _, p := range g.players {
			balance, ok := balances[p.id]
			if !ok || balance < 0 {
				return errors.New("invalid player assets")
			}
		}
	}
	// Generate both clues before changing live state. Plugin failure leaves the
	// lobby untouched; future clues remain private until their scheduled round.
	items := g.generate()
	planned, err := g.planHints(items)
	if err != nil {
		return err
	}
	g.items, g.plannedHints = items, planned
	if balances != nil {
		for _, p := range g.players {
			p.assets = balances[p.id]
		}
	}
	// A fresh partial Fisher-Yates shuffle gives each player the same per-item
	// inclusion probability without letting earlier players consume the pool.
	for _, p := range g.players {
		pool := append([]Item(nil), items...)
		count := min(InitialInspectionCount, len(pool))
		p.inspection = make([]InspectionItem, 0, count)
		for i := 0; i < count; i++ {
			j := i + random(len(pool)-i)
			pool[i], pool[j] = pool[j], pool[i]
			item := pool[i]
			p.inspection = append(p.inspection, InspectionItem{item.ID, item.Quality, item.TopLeft, item.BottomRight})
		}
	}
	g.phase = "bidding"
	g.round = 1
	g.hints = append(g.hints, g.plannedHints[0])
	g.version++
	return nil
}

func (g *Game) generate() []Item {
	items := g.tileRows(1 + random(g.height))
	for index := range items {
		i := &items[index]
		i.ID = secret()[:16]
		v := random(100)
		q := g.gameConfig.Qualities[0]
		for _, candidate := range g.gameConfig.Qualities {
			v -= candidate.Weight
			if v < 0 {
				q = candidate
				break
			}
		}
		i.Quality = q.ID
		i.Price = q.Min + int64(random(int(q.Max-q.Min+1)))
	}
	return items
}
func (g *Game) planHints(items []Item) ([]Hint, error) {
	request := hints.Request{ProtocolVersion: 1, BoxWidth: g.width, BoxHeight: g.height, Items: []hints.Item{}}
	for _, i := range items {
		request.Items = append(request.Items, hints.Item{ID: i.ID, Quality: i.Quality, Price: i.Price,
			TopLeft: hints.Point{X: i.TopLeft.X, Y: i.TopLeft.Y}, BottomRight: hints.Point{X: i.BottomRight.X, Y: i.BottomRight.Y}})
	}
	available := append([]hints.Plugin{}, g.hintConfig.Plugins...)
	result := []Hint{}
	priceUsed := false
	for _, round := range []int{1, 3} {
		var clue hints.Clue
		var plugin hints.Plugin
		for {
			if len(available) == 0 {
				return nil, errors.New("not enough applicable public hint plugins")
			}
			index := hints.Random(len(available))
			plugin = available[index]
			available = append(available[:index], available[index+1:]...)
			if priceUsed && (plugin.Builtin == "quality_average" || plugin.Builtin == "random_quality_price") {
				continue
			}
			request.Round = round
			var err error
			clue, err = plugin.Generate(context.Background(), request)
			if errors.Is(err, hints.ErrNotApplicable) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if clue.Fact != nil && clue.Fact.IsPrice() {
				if priceUsed {
					continue
				}
				priceUsed = true
			}
			break
		}
		encoded, err := json.Marshal(clue)
		if err != nil {
			return nil, err
		}
		var data map[string]any
		if err := json.Unmarshal(encoded, &data); err != nil {
			return nil, err
		}
		result = append(result, Hint{Round: round, Kind: plugin.ID, Source: "public", Data: data})
	}
	return result, nil
}
func (g *Game) total() int64 {
	var n int64
	for _, i := range g.items {
		n += i.Price
	}
	return n
}

// Evaluate approved primitives, never card IDs or executable configuration.
func (g *Game) cardEffect(effect cards.Effect) map[string]any {
	switch effect.Type {
	case "reveal":
		i := g.items[0]
		if effect.Selector == "random" {
			i = g.items[random(len(g.items))]
		} else {
			for _, candidate := range g.items {
				better := effect.Selector == "largest" && candidate.area() > i.area() || effect.Selector == "smallest" && candidate.area() < i.area()
				if better || candidate.area() == i.area() && candidate.ID < i.ID {
					i = candidate
				}
			}
		}
		data := map[string]any{"item_id": i.ID}
		for _, field := range effect.Fields {
			switch field {
			case "quality":
				data["quality"] = i.Quality
			case "price":
				data["price"] = i.Price
			case "shape":
				data["top_left"], data["bottom_right"] = i.TopLeft, i.BottomRight
			}
		}
		return data
	case "count":
		if effect.GroupBy == "" {
			return map[string]any{"count": len(g.items)}
		}
		counts := map[string]int{}
		for _, q := range g.gameConfig.Qualities {
			counts[q.ID] = 0
		}
		for _, i := range g.items {
			counts[i.Quality]++
		}
		return map[string]any{"counts": counts}
	}
	panic("unvalidated card effect")
}
func (g *Game) find(token string) *player {
	for _, p := range g.players {
		if p.token == token {
			return p
		}
	}
	return nil
}
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
func ValidateReason(reason string) error {
	if !utf8.ValidString(reason) || strings.TrimSpace(reason) == "" || utf8.RuneCountInString(reason) > 2000 {
		return errors.New("reason must contain 1 to 2000 characters")
	}
	return nil
}
func (g *Game) UseCard(token string, round int, card, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.find(token)
	if p == nil {
		return ErrUnauthorized
	}
	if err := ValidateReason(reason); err != nil {
		return err
	}
	if g.phase != "bidding" || g.round != round {
		return errors.New("not the current bidding round")
	}
	if p.bid != nil {
		return errors.New("cannot use a card after bidding")
	}
	if p.cardRound == round || contains(p.used, card) {
		return errors.New("card already used or round card allowance exhausted")
	}
	if !contains(p.deck, card) {
		return errors.New("card is not in your deck")
	}
	var effect cards.Effect
	for _, definition := range g.cardConfig.Cards {
		if definition.ID == card {
			effect = definition.Effect
			break
		}
	}
	p.hints = append(p.hints, Hint{Round: round, Kind: card, Source: card, Data: g.cardEffect(effect)})
	p.used = append(p.used, card)
	p.cardRound = round
	p.revision++
	g.decisions = append(g.decisions, Decision{round, p.id, "card", reason})
	return nil
}
func (g *Game) Submit(token string, round int, amount int64, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.find(token)
	if p == nil {
		return ErrUnauthorized
	}
	if err := ValidateReason(reason); err != nil {
		return err
	}
	if g.phase != "bidding" || g.round != round {
		return errors.New("not the current bidding round")
	}
	if amount < 0 || amount > p.assets {
		return fmt.Errorf("bid must be an integer between 0 and your total assets (%d)", p.assets)
	}
	if p.bid != nil {
		return errors.New("already bid this round")
	}
	p.bid = &amount
	g.decisions = append(g.decisions, Decision{round, p.id, "bid", reason})
	g.version++
	for _, other := range g.players {
		if other.bid == nil {
			return nil
		}
	}
	result := RoundResult{Round: g.round, Bids: []Bid{}}
	for _, other := range g.players {
		result.Bids = append(result.Bids, Bid{other.id, *other.bid})
	}
	g.history = append(g.history, result)
	if g.round == Rounds || EarlySettlement(result) {
		g.settle(result)
		return nil
	}
	g.round++
	for _, other := range g.players {
		other.bid = nil
	}
	if g.round == 3 {
		g.hints = append(g.hints, g.plannedHints[1])
	}
	return nil
}

// A unique positive highest bid must meet the exact rational threshold.
// Ties (including all-zero bids) continue until the ordinary final round.
func EarlySettlement(r RoundResult) bool {
	numerator := map[int]int64{1: 20, 2: 17, 3: 15}[r.Round]
	if numerator == 0 || len(r.Bids) < 2 {
		return false
	}
	high, second := int64(-1), int64(-1)
	for _, b := range r.Bids {
		if b.Amount > high {
			second, high = high, b.Amount
		} else if b.Amount > second {
			second = b.Amount
		}
	}
	return high > 0 && high > second && new(big.Int).Mul(big.NewInt(high), big.NewInt(10)).Cmp(new(big.Int).Mul(big.NewInt(second), big.NewInt(numerator))) >= 0
}
func (g *Game) settle(r RoundResult) {
	var high int64 = -1
	ties := []string{}
	for _, b := range r.Bids {
		if b.Amount > high {
			high = b.Amount
			ties = []string{b.PlayerID}
		} else if b.Amount == high {
			ties = append(ties, b.PlayerID)
		}
	}
	sort.Strings(ties)
	winner := ties[random(len(ties))]
	total := g.total()
	payoffs := map[string]int64{}
	for _, p := range g.players {
		payoffs[p.id] = 0
	}
	payoffs[winner] = total - high
	method := "final"
	if r.Round < Rounds {
		method = "instant"
	}
	g.settlement = &Settlement{WinnerID: winner, Paid: high, Total: total, Profit: total - high, Payoffs: payoffs, Tied: ties, Items: g.items, Round: r.Round, Method: method}
	g.phase = "settled"
}

// Observe serializes under the lock so no caller can mutate or race internal slices/maps.
// An empty token yields the same public observation for players, spectators, and host.
func (g *Game) Observe(token string) ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.observe(token, false)
}
func (g *Game) ObserveObserver() ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.observe("", true)
}
func (g *Game) observe(token string, observer bool) ([]byte, error) {
	v := View{Version: g.version, Phase: g.phase, Round: g.round, Players: []PlayerInfo{}, PublicHints: g.hints, History: g.history, Settlement: g.settlement,
		Rules: Rules{Rounds: Rounds, Width: g.width, Height: g.height, CoordinateUnit: "cell", GameConfig: g.gameConfig.Clone(), PlayerCount: g.maxPlayers, Qualities: append([]Quality{}, g.gameConfig.Qualities...), Cards: g.cardConfig.Cards, CardsSchemaVersion: g.cardConfig.SchemaVersion, DefaultDeck: g.cardConfig.DefaultDeck, PublicHintPlugins: g.hintConfig.IDs(),
			Description: "Bid on a hidden box with 2–7 players over up to 5 rounds. All bids are revealed together. In rounds 1/2/3, a unique positive highest bid settles at >= 2x/1.7x/1.5x the second-highest bid. Round 4 continues; round 5 settles at the highest bid, with uniform random choice among ties, including all-zero bids. The winner pays that round's bid and receives the box; payoff is total value minus price, others receive 0. Public clues appear in rounds 1 and 3. Before bidding, optionally use one carried, unused card; results are private. Each card/bid requires a reason. Read state for the current round and your submitted bid; watch for updates. Bids cannot exceed your current account assets, updated at settlement. An account may join only one unfinished room at a time, including waiting rooms. See bidbrawl handbook for commands."}}
	v.Rules.EarlyMultipliers = []float64{2, 1.7, 1.5, 0, 0}
	v.Rules.InitialInspectionCount = InitialInspectionCount
	v.Rules.Description += " At start, every player independently samples min(initial_inspection_count, item count) distinct items uniformly without replacement. Each item has the same inclusion probability for every player regardless of quality; players may sample overlapping items. Your self.initial_inspection contains only sampled item IDs, shapes and qualities, never prices."
	v.Rules.Description += " Every single-item skill also reveals its target's shape and position in self.item_reveals, together with only the quality or price fields learned by that skill. Count skills do not uncover positions."
	if observer {
		v.Decisions = g.observerDecisions()
		v.InitialInspections = make(map[string][]InspectionItem, len(g.players))
		v.ItemReveals = make(map[string][]ItemReveal, len(g.players))
		for _, p := range g.players {
			v.Version += p.revision
			v.InitialInspections[p.id] = p.inspection
			var err error
			v.ItemReveals[p.id], err = g.itemReveals(p)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, p := range g.players {
		v.Players = append(v.Players, PlayerInfo{p.id, p.name, p.bid != nil, p.avatar})
	}
	if token != "" {
		p := g.find(token)
		if p == nil {
			return nil, ErrUnauthorized
		}
		v.Version += p.revision
		own := []Decision{}
		for _, d := range g.decisions {
			if d.PlayerID == p.id {
				own = append(own, d)
			}
		}
		reveals, err := g.itemReveals(p)
		if err != nil {
			return nil, err
		}
		balance := p.assets
		if g.settlement != nil {
			balance += g.settlement.Payoffs[p.id]
		}
		v.Self = &Self{ID: p.id, Deck: p.deck, Used: p.used, Hints: p.hints, Bid: p.bid, CardUsedThisRound: p.cardRound == g.round, MaxBid: balance, Assets: balance, Decisions: own, InitialInspection: p.inspection, ItemReveals: reveals}
	}
	return json.Marshal(v)
}

func (g *Game) itemReveals(p *player) ([]ItemReveal, error) {
	reveals := []ItemReveal{}
	for _, hint := range p.hints {
		// Whitelist learned attributes from the durable result, then locate only
		// that target. Never copy an item's hidden quality or price.
		data, err := json.Marshal(hint.Data)
		if err != nil {
			return nil, err
		}
		var reveal ItemReveal
		if err := json.Unmarshal(data, &reveal); err != nil {
			return nil, err
		}
		if reveal.ItemID == "" {
			continue
		}
		for _, item := range g.items {
			if item.ID == reveal.ItemID {
				reveal.TopLeft, reveal.BottomRight = &item.TopLeft, &item.BottomRight
				break
			}
		}
		reveal.Round = hint.Round
		reveals = append(reveals, reveal)
	}
	return reveals, nil
}

// Card identity comes from the durable skill-use record; raw results stay private.
func (g *Game) observerDecisions() []ObserverDecision {
	type useKey struct {
		player string
		round  int
	}
	used := map[useKey]string{}
	for _, p := range g.players {
		for _, hint := range p.hints {
			used[useKey{p.id, hint.Round}] = hint.Source
		}
	}
	result := make([]ObserverDecision, 0, len(g.decisions))
	for _, d := range g.decisions {
		view := ObserverDecision{Decision: d}
		if d.Action == "card" {
			view.CardID = used[useKey{d.PlayerID, d.Round}]
		}
		result = append(result, view)
	}
	return result
}
