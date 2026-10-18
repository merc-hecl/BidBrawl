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
	TemplateID  string `json:"template_id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
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
	Specialty string `json:"specialty"`
	Category  string `json:"category,omitempty"`
	Withdrawn bool   `json:"withdrawn"`
}
type Self struct {
	TicketCost        int64            `json:"ticket_cost"`
	Intelligence      []Hint           `json:"intelligence"`
	Specialty         string           `json:"specialty"`
	Category          string           `json:"category,omitempty"`
	CardCost          int64            `json:"card_cost"`
	Withdrawn         bool             `json:"withdrawn"`
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
	Source      string `json:"source,omitempty"`
	Name        string `json:"name,omitempty"`
	Category    string `json:"category,omitempty"`
	TemplateID  string `json:"template_id,omitempty"`
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
	TicketCosts     map[string]int64 `json:"ticket_costs"`
	TotalTicketCost int64            `json:"total_ticket_cost"`
	TotalCardCost   int64            `json:"total_card_cost"`
	CardCosts       map[string]int64 `json:"card_costs"`
	GrossProfit     int64            `json:"gross_profit"`
	CashChanges     map[string]int64 `json:"cash_changes"`
	WinnerID        string           `json:"winner_id"`
	Paid            int64            `json:"paid"`
	Total           int64            `json:"total_value"`
	Profit          int64            `json:"profit"`
	Payoffs         map[string]int64 `json:"payoffs"`
	Tied            []string         `json:"tied_players"`
	Items           []Item           `json:"items"`
	Round           int              `json:"round"`
	Method          string           `json:"method"`
}
type Rules struct {
	PublicHintDefinitions  []hints.Definition `json:"public_hint_definitions"`
	TicketFee              int64              `json:"ticket_fee"`
	Specialties            []Specialty        `json:"specialties"`
	Categories             []Category         `json:"categories"`
	Catalog                []ItemTemplate     `json:"catalog"`
	Warehouse              string             `json:"warehouse"`
	Warehouses             []Warehouse        `json:"warehouses"`
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
	PlayerCosts        map[string]PlayerCosts      `json:"player_costs,omitempty"`
	Context            map[string]any              `json:"context"`
	Intelligence       map[string][]Hint           `json:"intelligence,omitempty"`
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
	Items              []Item                      `json:"items,omitempty"`
}
type PlayerCosts struct {
	CardCost   int64 `json:"card_cost"`
	TicketCost int64 `json:"ticket_cost"`
}
type player struct {
	cardPrices          map[string]int64
	specialty, category string
	signals             []Hint
	withdrawRound       int
	ticketCost          int64
	withdrawn           bool
	cardCost            int64
	id, name, token     string
	deck, used          []string
	hints               []Hint
	bid                 *int64
	cardRound           int
	revision            int
	avatar              *int
	inspection          []InspectionItem
	assets              int64
}
type Game struct {
	loadoutChanges                            int
	warehouse                                 string
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
	if geometry.Specialties == nil {
		geometry.Specialties = cloneSpecialties(Specialties)
	}
	if geometry.Catalog == nil {
		geometry.Catalog = append([]ItemTemplate{}, builtinCatalog.Items...)
	}
	return &Game{warehouse: "mixed", maxPlayers: maxPlayers, width: geometry.Box.Width, height: geometry.Box.Height, phase: "lobby", version: 1, players: []*player{}, hints: []Hint{}, history: []RoundResult{}, decisions: []Decision{}, cardConfig: cfg.Clone(), gameConfig: geometry.Clone(), hintConfig: hintConfig.Clone()}, nil
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
	p := &player{specialty: "analysis", signals: []Hint{}, id: fmt.Sprintf("p%d", len(g.players)+1), name: name, token: secret(), deck: append([]string{}, deck...), used: []string{}, hints: []Hint{}, avatar: &avatar, assets: g.gameConfig.InitialAssets}
	if findSpecialty(g.gameConfig.Specialties, p.specialty) == nil {
		p.specialty = g.gameConfig.Specialties[0].ID
	}
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
			if !ok || balance < g.gameConfig.TicketFee {
				return errors.New("invalid player assets")
			}
		}
	}
	if balances == nil {
		for _, p := range g.players {
			if p.assets < g.gameConfig.TicketFee {
				return errors.New("insufficient cash for ticket")
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
	for _, p := range g.players {
		p.assets -= g.gameConfig.TicketFee
		p.ticketCost = g.gameConfig.TicketFee
	}
	// A fresh partial Fisher-Yates shuffle gives each player the same per-item
	// inclusion probability without letting earlier players consume the pool.
	for _, p := range g.players {
		pool := scoutableItems(items)
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
	for _, p := range g.players {
		g.giveIntelligence(p)
	}
	g.hints = append(g.hints, g.plannedHints[0])
	g.version++
	return nil
}

func (g *Game) generate() []Item { return g.generateCatalog() }

func isEasterEgg(item Item) bool {
	return item.Quality == "easter_egg" || item.Category == "easter_egg"
}

// Scouting never includes eggs, even in aggregate counts, area or value.
func scoutableItems(items []Item) []Item {
	pool := make([]Item, 0, len(items))
	for _, item := range items {
		if !isEasterEgg(item) {
			pool = append(pool, item)
		}
	}
	return pool
}

func (g *Game) planHints(items []Item) ([]Hint, error) {
	request := hints.Request{ProtocolVersion: 1, BoxWidth: g.width, BoxHeight: g.height, Items: []hints.Item{}, Categories: map[string]string{}}
	for _, c := range Categories {
		if c.ID != "easter_egg" {
			request.Categories[c.ID] = c.Name
		}
	}
	for _, i := range items {
		if isEasterEgg(i) {
			continue
		}
		request.Items = append(request.Items, hints.Item{ID: i.ID, Category: i.Category, Quality: i.Quality, Price: i.Price,
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
func (g *Game) cardEffect(e cards.Effect) map[string]any { return g.effect(e, nil) }
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
	if p.withdrawn {
		return errors.New("already withdrawn")
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
			if round < definition.MinRound {
				return fmt.Errorf("card unlocks in round %d", definition.MinRound)
			}
			effect = definition.Effect
			break
		}
	}
	p.hints = append(p.hints, Hint{Round: round, Kind: card, Source: card, Data: g.effect(effect, p)})
	for _, d := range g.cardConfig.Cards {
		if d.ID == card {
			price, ok := p.cardPrices[card]
			if !ok {
				price = d.Price
			}
			p.cardCost += price
		}
	}
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
	if amount < 1 || amount > p.assets {
		return fmt.Errorf("bid must be a positive integer up to your available cash; use withdraw to exit (%d)", p.assets)
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
	return g.resolveRound()
}

// Withdraw is an explicit decision; zero is reserved for published automatic skips.
func (g *Game) Withdraw(token string, round int, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.find(token)
	if p == nil {
		return ErrUnauthorized
	}
	if err := ValidateReason(reason); err != nil {
		return err
	}
	if g.phase != "bidding" || round != g.round {
		return errors.New("not the current bidding round")
	}
	if p.withdrawn || p.bid != nil {
		return errors.New("already withdrawn or bid this round")
	}
	p.withdrawn = true
	p.withdrawRound = round
	zero := int64(0)
	p.bid = &zero
	g.decisions = append(g.decisions, Decision{round, p.id, "withdraw", reason})
	g.version++
	return g.resolveRound()
}

func (g *Game) resolveRound() error {
	for {
		result := RoundResult{Round: g.round, Bids: []Bid{}}
		for _, p := range g.players {
			if p.bid == nil {
				return nil
			}
			result.Bids = append(result.Bids, Bid{p.id, *p.bid})
		}
		g.history = append(g.history, result)
		allZero := true
		for _, b := range result.Bids {
			if b.Amount > 0 {
				allZero = false
			}
		}
		if g.round == Rounds || EarlySettlement(result) || allZero {
			g.settle(result)
			return nil
		}
		g.round++
		if g.round == 3 {
			g.hints = append(g.hints, g.plannedHints[1])
		}
		for _, p := range g.players {
			p.bid = nil
			if p.withdrawn {
				zero := int64(0)
				p.bid = &zero
				g.decisions = append(g.decisions, Decision{g.round, p.id, "bid", "已弃权，自动跳过。"})
				g.version++
			} else {
				g.giveIntelligence(p)
			}
		}
	}
}

// A unique positive highest bid must meet the exact rational threshold.
// Positive ties continue until the final round; resolveRound handles all-zero unsold.
func EarlySettlement(r RoundResult) bool {
	numerator := map[int]int64{1: 20, 2: 16, 3: 13, 4: 11}[r.Round]
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
	winner := ""
	if high > 0 {
		winner = ties[random(len(ties))]
	}
	total := g.total()
	payoffs := map[string]int64{}
	costs, cash := map[string]int64{}, map[string]int64{}
	tickets := map[string]int64{}
	var ticketTotal int64
	var costTotal int64
	for _, p := range g.players {
		costs[p.id] = p.cardCost
		tickets[p.id] = p.ticketCost
		ticketTotal += p.ticketCost
		costTotal += p.cardCost
		cash[p.id] = 0
		payoffs[p.id] = -p.cardCost - p.ticketCost
	}
	gross := int64(0)
	if winner != "" {
		gross = total - high
		payoffs[winner] += gross
		cash[winner] = -high
	}
	method := "final"
	if r.Round < Rounds {
		method = "instant"
	}
	if winner == "" {
		method = "unsold"
	}
	g.settlement = &Settlement{TicketCosts: tickets, TotalTicketCost: ticketTotal, TotalCardCost: costTotal, CardCosts: costs, GrossProfit: gross, CashChanges: cash, WinnerID: winner, Paid: high, Total: total, Profit: gross - costTotal - ticketTotal, Payoffs: payoffs, Tied: ties, Items: g.items, Round: r.Round, Method: method}
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
	v := View{Version: g.version, Phase: g.phase, Round: g.round, Players: []PlayerInfo{}, Context: g.publicContext(), PublicHints: g.hints, History: g.history, Settlement: g.settlement,
		Rules: Rules{TicketFee: g.gameConfig.TicketFee, Specialties: g.gameConfig.Specialties, Categories: Categories, Catalog: catalogForWarehouse(g.gameConfig, g.warehouse), Warehouse: g.warehouse, Warehouses: Warehouses, Rounds: Rounds, Width: g.width, Height: g.height, CoordinateUnit: "cell", GameConfig: g.gameConfig.Clone(), PlayerCount: g.maxPlayers, Qualities: CatalogQualities(g.gameConfig), Cards: g.cardConfig.Cards, CardsSchemaVersion: g.cardConfig.SchemaVersion, DefaultDeck: g.cardConfig.DefaultDeck, PublicHintPlugins: g.hintConfig.IDs(),
			Description: "2–7 players. Five sealed-offer rounds; after each round all bids are published together. A unique positive highest bid settles at >= 2/1.6/1.3/1.1 times the runner-up in rounds 1–4. Round 5 chooses the highest bid, uniformly among ties. Offers must be positive; withdraw explicitly before bidding to permanently exit. Everyone withdrawing ends unsold. Tickets are charged once on start, nonrefundable and with no extra exit fine. Winner pays cash and receives items in their warehouse; sell to system outside bidding to obtain cash. Profit deducts consumed card costs and tickets for every player. Buy cards before bidding; one carried unused card per round. Configure a specialty in preparation; private intelligence grows by round. Reasons are private. Public clues arrive in rounds 1 and 3. Cash, not total inventory wealth, limits bids."}}
	v.Rules.EarlyMultipliers = []float64{2, 1.6, 1.3, 1.1, 0}
	v.Rules.PublicHintDefinitions = g.hintConfig.Definitions()
	v.Rules.InitialInspectionCount = InitialInspectionCount

	if observer {
		v.Items = g.items
		v.PlayerCosts = map[string]PlayerCosts{}
		v.Intelligence = map[string][]Hint{}
		v.Decisions = g.observerDecisions()
		v.InitialInspections = make(map[string][]InspectionItem, len(g.players))
		v.ItemReveals = make(map[string][]ItemReveal, len(g.players))
		for _, p := range g.players {
			v.PlayerCosts[p.id] = PlayerCosts{CardCost: p.cardCost, TicketCost: p.ticketCost}
			v.Version += p.revision
			v.InitialInspections[p.id] = p.inspection
			v.Intelligence[p.id] = append(append([]Hint{}, p.signals...), p.hints...)
			var err error
			v.ItemReveals[p.id], err = g.itemReveals(p)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, p := range g.players {
		v.Players = append(v.Players, PlayerInfo{ID: p.id, Name: p.name, Submitted: p.bid != nil, Avatar: p.avatar, Specialty: p.specialty, Category: p.category, Withdrawn: g.publishedWithdrawal(p.id)})
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
			balance += g.settlement.CashChanges[p.id]
		}
		v.Self = &Self{TicketCost: p.ticketCost, Intelligence: p.signals, Specialty: p.specialty, Category: p.category, CardCost: p.cardCost, Withdrawn: p.withdrawn, ID: p.id, Deck: p.deck, Used: p.used, Hints: p.hints, Bid: p.bid, CardUsedThisRound: g.phase == "bidding" && p.cardRound == g.round, MaxBid: balance, Assets: balance, Decisions: own, InitialInspection: p.inspection, ItemReveals: reveals}
	}
	return json.Marshal(v)
}

func (g *Game) itemReveals(p *player) ([]ItemReveal, error) {
	reveals := []ItemReveal{}
	for _, hint := range append(append([]Hint{}, p.hints...), p.signals...) {
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
			list, _ := hint.Data["items"].([]any)
			if typed, ok := hint.Data["items"].([]map[string]any); ok {
				for _, d := range typed {
					list = append(list, d)
				}
			}
			for _, d := range list {
				data, _ := json.Marshal(d)
				var r ItemReveal
				if json.Unmarshal(data, &r) == nil && r.ItemID != "" {
					for _, i := range g.items {
						if i.ID == r.ItemID {
							a, b := i.TopLeft, i.BottomRight
							r.TopLeft, r.BottomRight = &a, &b
						}
					}
					r.Round = hint.Round
					r.Source = hint.Source
					reveals = append(reveals, r)
				}
			}
			continue
		}
		for _, item := range g.items {
			if item.ID == reveal.ItemID {
				reveal.TopLeft, reveal.BottomRight = &item.TopLeft, &item.BottomRight
				break
			}
		}
		reveal.Round = hint.Round
		reveal.Source = hint.Source
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

func (g *Game) publishedWithdrawal(id string) bool {
	for _, r := range g.history {
		for _, b := range r.Bids {
			if b.PlayerID == id && b.Amount == 0 {
				return true
			}
		}
	}
	return false
}
