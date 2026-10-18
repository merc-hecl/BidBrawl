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
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Categories  []string        `json:"categories,omitempty"`
	Tag         string          `json:"tag,omitempty"`
	Steps       []SpecialtyStep `json:"steps"`
}
type SpecialtyStep struct {
	Round      int          `json:"round"`
	Effect     cards.Effect `json:"effect"`
	Qualities  []string     `json:"qualities,omitempty"`
	Mode       string       `json:"mode,omitempty"`
	All        bool         `json:"all,omitempty"`
	Remember   string       `json:"remember,omitempty"`
	FromSample string       `json:"from_sample,omitempty"`
	SampleSize int          `json:"sample_size,omitempty"`
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
		if s.ID == "" || ids[s.ID] || s.Name == "" || len(s.Steps) == 0 {
			return errors.New("invalid specialty catalog")
		}
		ids[s.ID] = true
		for _, c := range s.Categories {
			if !ValidCategory(c) {
				return errors.New("invalid specialty category")
			}
		}
		for _, step := range s.Steps {
			if step.Round < 1 || step.Round > 5 || !slices.Contains([]string{"", "average", "presence"}, step.Mode) {
				return errors.New("invalid specialty step")
			}
			if step.SampleSize < 0 || step.SampleSize > 4096 || step.SampleSize > 0 && step.Mode != "average" {
				return errors.New("sample_size requires an average step and must be 1–4096")
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
		if !isEasterEgg(i) && (e.Category == "" || i.Category == e.Category) && (e.Quality == "" || i.Quality == e.Quality) && (len(e.Qualities) == 0 || slices.Contains(e.Qualities, i.Quality)) && (e.Area == 0 || i.area() == e.Area) {
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
				if e.Measure == "area" {
					sum += int64(i.area())
				} else {
					sum += i.Price
				}
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
			case "value":
				value += i.Price
			}
		}
		key := "count"
		if e.Type == "area" {
			key = "area"
		}
		if e.Type == "value" {
			key = "total_value"
		}
		if e.Type == "count" {
			return map[string]any{"count": int(value)}
		}
		return map[string]any{key: value, "category": e.Category, "quality": e.Quality}
	}
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
		if _, ok := d["price"]; ok {
			known[id]["price"] = true
		}
		if _, ok := d["top_left"]; ok {
			known[id]["shape"] = true
		}
	}
	if e.Selector == "random" && p != nil {
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

func (g *Game) giveIntelligence(p *player) {
	spec := findSpecialty(g.gameConfig.Specialties, p.specialty)
	if spec == nil {
		return
	}
	for _, step := range spec.Steps {
		if step.Round != g.round {
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
				east := item.TopLeft.X >= g.width/2
				south := item.TopLeft.Y >= g.height/2
				if east != strings.HasSuffix(zone, "east") || south != strings.HasPrefix(zone, "south") {
					continue
				}
			}
			pool = append(pool, item)
		}
		e := step.Effect
		if step.All {
			e.Count = len(pool)
		}
		if step.Mode == "average" {
			e = cards.Effect{Type: "average", Measure: "price", SampleSize: step.SampleSize}
		}
		d := g.effectOn(e, p, pool)
		if step.Mode == "presence" {
			d = map[string]any{"present": len(pool) > 0}
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
	if v, ok := d["average_price"]; ok {
		if v == nil {
			return "该范围内没有藏品，均价无结果。"
		}
		if count, ok := d["sample_count"]; ok {
			return fmt.Sprintf("抽样 %v 件的均价：%v 金币，并非全部藏品均价。", count, v)
		}
		return fmt.Sprintf("该范围藏品均价：%v 金币。", v)
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
	if v, ok := d["total_value"]; ok {
		return fmt.Sprintf("该范围藏品总价值：%v 金币。", v)
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
