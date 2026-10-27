package game

import (
	"bidbrawl/internal/cards"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

type Specialty struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Categories   []string        `json:"categories,omitempty"`
	Tag          string          `json:"tag,omitempty"`
	Style        string          `json:"style,omitempty"`
	ReserveDraws int             `json:"reserve_draws,omitempty"`
	ReportDraws  int             `json:"report_draws,omitempty"`
	Steps        []SpecialtyStep `json:"steps,omitempty"`
}
type SpecialtyStep struct {
	Condition  string       `json:"condition,omitempty"`
	Round      int          `json:"round"`
	Effect     cards.Effect `json:"effect"`
	Qualities  []string     `json:"qualities,omitempty"`
	Mode       string       `json:"mode,omitempty"`
	All        bool         `json:"all,omitempty"`
	Remember   string       `json:"remember,omitempty"`
	FromSample string       `json:"from_sample,omitempty"`
}

//go:embed specialties.json
var specialtiesJSON []byte
var Specialties = func() []Specialty {
	var v []Specialty
	if err := json.Unmarshal(specialtiesJSON, &v); err != nil {
		panic(err)
	}
	return v
}()

func cloneSpecialties(v []Specialty) []Specialty {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	var out []Specialty
	_ = json.Unmarshal(b, &out)
	return out
}
func findSpecialty(v []Specialty, id string) *Specialty {
	for i := range v {
		if v[i].ID == id {
			return &v[i]
		}
	}
	return nil
}
func validSpecialty(id string) bool { return findSpecialty(Specialties, id) != nil }
func validSelection(v []Specialty, id, category string) bool {
	s := findSpecialty(v, id)
	if s == nil {
		return false
	}
	if category == "" {
		return true
	}
	if id == "region" {
		return slices.Contains([]string{"northwest", "northeast", "southwest", "southeast"}, category)
	}
	return slices.Contains(s.Categories, category)
}
func validateSpecialties(v []Specialty) error {
	if v != nil && len(v) == 0 {
		return errors.New("empty specialty catalog")
	}
	ids := map[string]bool{}
	for _, s := range v {
		if s.ID == "" || ids[s.ID] || s.Name == "" {
			return errors.New("invalid specialty catalog")
		}
		ids[s.ID] = true
		if s.ReserveDraws != 0 && (s.Style != "quick" || s.ReserveDraws != 6 && s.ReserveDraws != 12) {
			return errors.New("reserve_draws must be 6 or 12 for quick style only")
		}
		if s.ReportDraws != 0 && (s.Style != "planner" || s.ReportDraws != 5 && s.ReportDraws != 6) {
			return errors.New("report_draws must be 5 or 6 for planner style only")
		}
		if s.Style != "" {
			if !slices.Contains(specialtyStyles, s.Style) || len(s.Steps) != 0 || len(s.Categories) != 0 || s.Tag != "" {
				return errors.New("invalid specialty style")
			}
			continue
		}
		if len(s.Steps) == 0 {
			return errors.New("specialty requires a style or scripted steps")
		}
		samples := map[string]bool{}
		lastRound := 0
		for _, c := range s.Categories {
			if !ValidCategory(c) {
				return errors.New("invalid specialty category")
			}
		}
		for _, step := range s.Steps {
			if step.Round < lastRound {
				return errors.New("specialty steps must be ordered by round")
			}
			lastRound = step.Round
			if step.All && step.Effect.Type != "reveal" {
				return errors.New("all is only used by reveal")
			}
			if step.Effect.RevealsPrice() {
				return errors.New("specialties cannot reveal prices or identities")
			}
			if step.Mode == "presence" && (step.Effect.Type != "count" || step.Effect.GroupBy != "") {
				return errors.New("presence mode requires an ungrouped count effect")
			}
			if step.FromSample != "" && !samples[step.FromSample] {
				return errors.New("from_sample must reference an earlier remembered sample")
			}
			if step.Remember != "" {
				if samples[step.Remember] || step.Effect.Type != "reveal" {
					return errors.New("remember requires a unique reveal sample")
				}
				samples[step.Remember] = true
			}
			seenQualities := map[string]bool{}
			for _, q := range step.Qualities {
				if !slices.Contains([]string{"common", "uncommon", "rare", "epic", "legendary"}, q) || seenQualities[q] {
					return errors.New("invalid specialty quality filter")
				}
				seenQualities[q] = true
			}
			if step.Condition != "" && step.Condition != "previous_unique_highest" && step.Condition != "previous_below_highest" {
				return errors.New("unknown specialty condition")
			}
			if step.Condition != "" && step.Round == 1 {
				return errors.New("previous-round condition cannot trigger in round 1")
			}
			if step.Round < 1 || step.Round > 5 || !slices.Contains([]string{"", "presence"}, step.Mode) {
				return errors.New("invalid specialty step")
			}
			cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{}, Cards: []cards.Definition{{ID: "effect_check", Name: "effect", Description: "effect", Price: 1, Effect: step.Effect}}}
			if err := cfg.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *Game) Configure(token, specialty, category string, deck []string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.find(token)
	if p == nil {
		return ErrUnauthorized
	}
	if g.phase != "lobby" {
		return errors.New("loadout is locked after start")
	}
	if !validSelection(g.gameConfig.Specialties, specialty, category) {
		return errors.New("choose a valid specialty and category")
	}
	if len(deck) > 5 {
		return errors.New("at most 5 different cards")
	}
	seen := map[string]bool{}
	for _, id := range deck {
		found := false
		for _, c := range g.cardConfig.Cards {
			if c.ID == id {
				found = true
			}
		}
		if !found || seen[id] {
			return errors.New("unknown or duplicate card")
		}
		seen[id] = true
	}
	p.specialty, p.category, p.deck = specialty, category, append([]string{}, deck...)
	g.version++
	g.loadoutChanges++
	return nil
}

// Effects return durable, explicitly whitelisted observations. Unknown-target
// sampling is relative to this player, including their opening inspection.
func (g *Game) effect(e cards.Effect, p *player) map[string]any { return g.effectOn(e, p, g.items) }
func (g *Game) effectOn(e cards.Effect, p *player, source []Item) map[string]any {
	pool := []Item{}
	for _, i := range source {
		if !isEasterEgg(i) && (e.Category == "" || i.Category == e.Category) && (e.Quality == "" || i.Quality == e.Quality) && (len(e.Qualities) == 0 || slices.Contains(e.Qualities, i.Quality)) && (e.Area == 0 || i.area() == e.Area) && (e.Region == "" || itemInRegion(i, e.Region, g.width, g.height)) && (e.ItemID == "" || i.ID == e.ItemID) {
			pool = append(pool, i)
		}
	}
	if e.Type != "reveal" {
		if e.Type == "average" {
			sample := pool
			if e.SampleSize > 0 {
				for n := 0; n < min(e.SampleSize, len(sample)); n++ {
					j := n + random(len(sample)-n)
					sample[n], sample[j] = sample[j], sample[n]
				}
				sample = sample[:min(e.SampleSize, len(sample))]
			}
			var sum int64
			for _, i := range sample {
				sum += int64(i.area())
			}
			var average any
			if len(sample) > 0 {
				average = float64(sum) / float64(len(sample))
			}
			d := map[string]any{"average_" + e.Measure: average}
			if e.SampleSize > 0 {
				d["sample_count"], d["sample_limit"] = len(sample), e.SampleSize
			}
			return d
		}
		if e.Type == "count" && e.GroupBy != "" {
			counts := map[string]int{}
			if e.GroupBy == "quality" {
				for _, q := range g.gameConfig.Qualities {
					if (e.Quality == "" || q.ID == e.Quality) && (len(e.Qualities) == 0 || slices.Contains(e.Qualities, q.ID)) {
						counts[q.ID] = 0
					}
				}
			} else {
				for _, c := range Categories {
					if c.ID != "easter_egg" && (e.Category == "" || e.Category == c.ID) {
						counts[c.ID] = 0
					}
				}
			}
			for _, i := range pool {
				key := i.Quality
				if e.GroupBy == "category" {
					key = i.Category
				}
				counts[key]++
			}
			return map[string]any{"counts": counts}
		}
		var value int64
		for _, i := range pool {
			switch e.Type {
			case "count":
				value++
			case "area":
				value += int64(i.area())
			}
		}
		key := "count"
		if e.Type == "area" {
			key = "area"
		}
		if e.Type == "count" {
			return map[string]any{"count": int(value)}
		}
		return map[string]any{key: value, "category": e.Category, "quality": e.Quality}
	}
	known := g.knownItemFields(p)
	if (e.Selector == "random" || e.UnknownOnly) && p != nil {
		fresh := []Item{}
		for _, i := range pool {
			for _, f := range e.Fields {
				if !known[i.ID][f] {
					fresh = append(fresh, i)
					break
				}
			}
		}
		pool = fresh
	}
	if e.Selector != "random" {
		sort.Slice(pool, func(a, b int) bool {
			ia, ib := pool[a], pool[b]
			if e.Selector == "highest" {
				rank := func(q string) int {
					for n, v := range []string{"common", "uncommon", "rare", "epic", "legendary"} {
						if v == q {
							return n
						}
					}
					return 0
				}
				if rank(ia.Quality) != rank(ib.Quality) {
					return rank(ia.Quality) > rank(ib.Quality)
				}
			}
			if ia.area() != ib.area() {
				if e.Selector == "smallest" {
					return ia.area() < ib.area()
				}
				return ia.area() > ib.area()
			}
			return ia.ID < ib.ID
		})
	}
	n := e.Count
	if n == 0 {
		n = 1
	}
	n = min(n, len(pool))
	results := []map[string]any{}
	for j := 0; j < n; j++ {
		idx := j
		if e.Selector == "random" {
			idx = j + random(len(pool)-j)
		}
		pool[j], pool[idx] = pool[idx], pool[j]
		i := pool[j]
		d := map[string]any{"item_id": i.ID}
		for _, f := range e.Fields {
			switch f {
			case "shape":
				d["top_left"], d["bottom_right"] = i.TopLeft, i.BottomRight
			case "quality":
				d["quality"] = i.Quality
			case "price":
				d["price"] = i.Price
			case "category":
				d["category"] = i.Category
			case "identity":
				d["name"], d["category"], d["template_id"] = i.Name, i.Category, i.TemplateID
			}
		}
		results = append(results, d)
	}
	if e.Count == 0 && len(results) == 1 {
		return results[0]
	}
	return map[string]any{"items": results, "count": len(results)}
}

// knownItemFields merges explicit personal and public observations only.
func (g *Game) knownItemFields(p *player) map[string]map[string]bool {
	known := map[string]map[string]bool{}
	if p != nil {
		for _, i := range p.inspection {
			known[i.ID] = map[string]bool{"shape": true, "quality": true}
		}
		for _, h := range append(append([]Hint{}, p.hints...), p.signals...) {
			mark := func(d map[string]any) {
				id, _ := d["item_id"].(string)
				if id == "" {
					return
				}
				if known[id] == nil {
					known[id] = map[string]bool{}
				}
				known[id]["shape"] = true // Every private item result also locates its target.
				for key := range d {
					if key == "top_left" {
						known[id]["shape"] = true
					}
					if key == "name" {
						known[id]["identity"] = true
					}
					known[id][key] = true
				}
			}
			mark(h.Data)
			if list, ok := h.Data["items"].([]any); ok {
				for _, v := range list {
					if d, ok := v.(map[string]any); ok {
						mark(d)
					}
				}
			}
			if list, ok := h.Data["items"].([]map[string]any); ok {
				for _, d := range list {
					mark(d)
				}
			}
		}
	}
	for _, h := range g.hints {
		d, ok := h.Data["fact"].(map[string]any)
		if !ok {
			continue
		}
		id, _ := d["item_id"].(string)
		if id == "" {
			continue
		}
		if known[id] == nil {
			known[id] = map[string]bool{}
		}
		if _, ok := d["quality"]; ok {
			known[id]["quality"] = true
		}
		if _, ok := d["top_left"]; ok {
			known[id]["shape"] = true
		}
	}
	return known
}

func (g *Game) giveIntelligence(p *player) {
	spec := findSpecialty(g.gameConfig.Specialties, p.specialty)
	if spec == nil {
		return
	}
	if spec.Style != "" {
		g.beginSpecialtyRound(p, spec)
		return
	}
	for _, step := range spec.Steps {
		if step.Round != g.round || !g.specialtyCondition(p, step.Condition) {
			continue
		}
		pool := []Item{}
		remembered := map[string]bool{}
		if step.FromSample != "" {
			for _, h := range p.signals {
				if h.Data["sample"] == step.FromSample {
					for _, id := range signalItemIDs(h.Data) {
						remembered[id] = true
					}
				}
			}
		}
		for _, item := range g.items {
			if isEasterEgg(item) {
				continue
			}
			if len(spec.Categories) > 0 && !slices.Contains(spec.Categories, item.Category) || p.category != "" && p.specialty != "region" && p.category != item.Category {
				continue
			}
			if len(step.Qualities) > 0 && !slices.Contains(step.Qualities, item.Quality) {
				continue
			}
			if step.FromSample != "" && !remembered[item.ID] {
				continue
			}
			if spec.Tag != "" {
				found := false
				for _, t := range g.gameConfig.Catalog {
					if t.ID == item.TemplateID && slices.Contains(t.Tags, spec.Tag) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if p.specialty == "region" {
				zone := p.category
				if zone == "" {
					zone = "northwest"
				}
				if !itemInRegion(item, zone, g.width, g.height) {
					continue
				}
			}
			pool = append(pool, item)
		}
		e := step.Effect
		if step.All {
			e.Count = len(pool)
		}
		d := g.effectOn(e, p, pool)
		if step.Mode == "presence" {
			count, _ := d["count"].(int)
			d = map[string]any{"present": count > 0}
		}
		if counts, ok := d["counts"].(map[string]int); ok && len(step.Qualities) > 0 {
			for q := range counts {
				if !slices.Contains(step.Qualities, q) {
					delete(counts, q)
				}
			}
		}
		if step.Remember != "" {
			d["sample"] = step.Remember
		}
		d["scope"] = map[string]any{"categories": spec.Categories, "qualities": step.Qualities, "tag": spec.Tag, "selection": p.category}
		g.addSignal(p, spec.Name, d)
	}
}

func itemInRegion(item Item, region string, width, height int) bool {
	return (item.TopLeft.X >= width/2) == strings.HasSuffix(region, "east") && (item.TopLeft.Y >= height/2) == strings.HasPrefix(region, "south")
}

// Eligibility uses only the preceding revealed round, never current sealed bids.
func (g *Game) specialtyCondition(p *player, condition string) bool {
	if condition == "" {
		return true
	}
	if len(g.history) == 0 || g.history[len(g.history)-1].Round != g.round-1 {
		return false
	}
	last := g.history[len(g.history)-1]
	var high, own int64
	leaders := 0
	for _, bid := range last.Bids {
		if bid.PlayerID == p.id {
			own = bid.Amount
		}
		if bid.Amount > high {
			high, leaders = bid.Amount, 1
		} else if bid.Amount == high {
			leaders++
		}
	}
	if condition == "previous_unique_highest" {
		return high > 0 && own == high && leaders == 1
	}
	return condition == "previous_below_highest" && own > 0 && own < high
}
func signalItemIDs(d map[string]any) []string {
	out := []string{}
	if id, ok := d["item_id"].(string); ok {
		out = append(out, id)
	}
	switch list := d["items"].(type) {
	case []map[string]any:
		for _, i := range list {
			if id, ok := i["item_id"].(string); ok {
				out = append(out, id)
			}
		}
	case []any:
		for _, v := range list {
			if i, ok := v.(map[string]any); ok {
				if id, ok := i["item_id"].(string); ok {
					out = append(out, id)
				}
			}
		}
	}
	return out
}
func (g *Game) addSignal(p *player, title string, d map[string]any) {
	d["title"] = title
	d["message"] = signalMessage(d)
	p.signals = append(p.signals, Hint{Round: g.round, Kind: p.specialty, Source: "specialty", Data: d})
}
func signalMessage(d map[string]any) string {
	if d["type"] == "sample_report" {
		topic, _ := d["topic"].(string)
		band, _ := d["band"].(string)
		label := map[string]string{"low": "低档（不足三分之一）", "mixed": "中档（三分之一至不足三分之二）", "high": "高档（至少三分之二）", "unavailable": "无可用结果"}[band]
		return fmt.Sprintf("%v 次有放回抽样：%s为%s。样本可能重复，不代表整箱比例。", d["draws"], topicName(topic), label)
	}
	if v, ok := d["average_area"]; ok {
		if v == nil {
			return "该范围内没有藏品，平均占格无结果。"
		}
		return fmt.Sprintf("该范围藏品平均占用 %v 格。", v)
	}
	if v, ok := d["present"].(bool); ok {
		if v {
			return "该范围内存在符合条件的藏品。"
		}
		return "该范围内不存在符合条件的藏品。"
	}
	if v, ok := d["counts"]; ok {
		return fmt.Sprintf("该范围藏品分组数量：%v。", v)
	}
	if list, ok := d["items"].([]map[string]any); ok {
		return fmt.Sprintf("本轮补充 %d 件藏品的私有信息，请查看结构化结果与已知物品。", len(list))
	}
	if v, ok := d["count"]; ok {
		return fmt.Sprintf("藏品数量：%v 件。", v)
	}
	if v, ok := d["area"]; ok {
		return fmt.Sprintf("藏品合计占用 %v 格。", v)
	}
	if v, ok := d["high_quality_count"]; ok {
		return fmt.Sprintf("史诗与传说合计 %v 件。", v)
	}
	if v, ok := d["high_quality_area"]; ok {
		return fmt.Sprintf("史诗与传说合计占用 %v 格。", v)
	}
	if v, ok := d["low_quality_value"]; ok {
		return fmt.Sprintf("普通与优良合计价格 %v 金币。", v)
	}

	return "本轮私有情报已更新。"
}

func (g *Game) publicContext() map[string]any {
	m := map[string]any{"warehouse": g.warehouse, "round": g.round, "remaining_rounds": Rounds - g.round, "early_multiplier": []float64{0, 2, 1.6, 1.3, 1.1, 0}[g.round], "ticket": g.gameConfig.TicketFee}
	return m
}
