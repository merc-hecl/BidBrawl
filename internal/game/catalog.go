package game

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// Catalog data is deliberately text-only. Category, footprint and quality narrow
// candidates; neither a large rectangle nor a small one guarantees a high value.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ItemTemplate struct {
	Translations map[string]string `json:"translations,omitempty"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Category     string            `json:"category"`
	Quality      string            `json:"quality"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	Price        int64             `json:"price"`
	ValuePercent int               `json:"value_percent"`
	FixedPrice   int64             `json:"fixed_price,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}
type Warehouse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Category        string `json:"favored_category,omitempty"`
	RecommendedCash int64  `json:"recommended_cash"`
}
type ItemCatalog struct {
	Categories []Category     `json:"categories"`
	Items      []ItemTemplate `json:"items"`
	Warehouses []Warehouse    `json:"warehouses"`
}

//go:embed items.json
var itemCatalogJSON []byte
var builtinCatalog = func() ItemCatalog {
	var c ItemCatalog
	if err := json.Unmarshal(itemCatalogJSON, &c); err != nil {
		panic(err)
	}
	return c
}()
var Categories = builtinCatalog.Categories
var Warehouses = builtinCatalog.Warehouses

func LoadCatalog(path string) ([]ItemTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c ItemCatalog
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("item catalog exceeds 1 MiB")
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("expected one item catalog")
	}
	if len(c.Items) == 0 {
		return nil, errors.New("empty item catalog")
	}
	return c.Items, nil
}

func ValidCategory(id string) bool {
	for _, c := range Categories {
		if c.ID == id {
			return true
		}
	}
	return false
}
func ValidWarehouse(id string) bool {
	for _, w := range Warehouses {
		if w.ID == id {
			return true
		}
	}
	return false
}
func (g *Game) SetWarehouse(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "lobby" || len(g.players) > 0 || !ValidWarehouse(id) {
		return errors.New("invalid warehouse or room already occupied")
	}
	g.warehouse = id
	return nil
}

func Catalog(cfg Config) []ItemTemplate {
	source := cfg.Catalog
	if source == nil {
		source = builtinCatalog.Items
	}
	out := make([]ItemTemplate, 0, len(source))
	for _, t := range source {
		if t.Width > cfg.Items.MaxWidth || t.Height > cfg.Items.MaxHeight {
			continue
		}
		for _, q := range cfg.Qualities {
			if q.ID == t.Quality {
				t.Price = q.Min + (q.Max-q.Min)*int64(t.ValuePercent)/100
			}
		}
		if t.FixedPrice > 0 {
			t.Price = t.FixedPrice
		}
		out = append(out, t)
	}
	return out
}

func (g *Game) generateCatalog() []Item {
	rows := 1 + random(g.height)
	skyline := make([]int, g.width)
	out := []Item{}
	catalog := Catalog(g.gameConfig)
	legendaryWeight := 0
	for _, q := range g.gameConfig.Qualities {
		if q.ID == "legendary" {
			legendaryWeight = q.Weight
		}
	}
	favored := ""
	for _, w := range Warehouses {
		if w.ID == g.warehouse {
			favored = w.Category
		}
	}
	for {
		x := 0
		for j := 1; j < g.width; j++ {
			if skyline[j] < skyline[x] {
				x = j
			}
		}
		y := skyline[x]
		if y == rows {
			return out
		}
		span := 1
		for x+span < g.width && skyline[x+span] == y {
			span++
		}
		eggs := []ItemTemplate{}
		for _, t := range catalog {
			if t.Category == "easter_egg" && t.Width <= span && t.Height <= rows-y {
				eggs = append(eggs, t)
			}
		}
		eggWeight := 0
		if len(eggs) > 0 {
			eggWeight = legendaryWeight
		}
		quality, isEgg := catalogTier(g.gameConfig.Qualities, eggWeight, random(100000+eggWeight))
		pool := []ItemTemplate{}
		for _, t := range catalog {
			if t.Category == "easter_egg" || t.Quality != quality || t.Width > span || t.Height > rows-y {
				continue
			}
			pool = append(pool, t)
			if t.Category == favored {
				pool = append(pool, t, t)
			}
		}
		if isEgg {
			pool = eggs
		}
		t := pool[random(len(pool))]
		out = append(out, Item{ID: secret()[:16], Quality: t.Quality, Price: t.Price, TopLeft: Point{x, y}, BottomRight: Point{x + t.Width, y + t.Height}, TemplateID: t.ID, Name: t.Name, Category: t.Category})
		for j := x; j < x+t.Width; j++ {
			skyline[j] = y + t.Height
		}
	}
}

// Ordinary quality weights are scaled by 1000. Eggs receive exactly one
// thousandth of the highest tier's probability, independent of template count.
func catalogTier(qualities []Quality, eggWeight, roll int) (string, bool) {
	if roll < eggWeight {
		return "legendary", true
	}
	roll = (roll - eggWeight) / 1000
	for _, q := range qualities {
		roll -= q.Weight
		if roll < 0 {
			return q.ID, false
		}
	}
	panic("invalid catalog draw")
}
