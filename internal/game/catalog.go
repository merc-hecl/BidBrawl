package game

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
)

// Catalog data is deliberately text-only. Category, footprint and quality narrow
// candidates; neither a large rectangle nor a small one guarantees a high value.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ItemTemplate struct {
	QualityAppearanceProbability *float64          `json:"quality_appearance_probability,omitempty"`
	AppearanceProbability        *float64          `json:"appearance_probability,omitempty"`
	Translations                 map[string]string `json:"translations,omitempty"`
	ID                           string            `json:"id"`
	Name                         string            `json:"name"`
	Category                     string            `json:"category"`
	Quality                      string            `json:"quality"`
	Width                        int               `json:"width"`
	Height                       int               `json:"height"`
	Price                        int64             `json:"price"`
	ValuePercent                 int               `json:"value_percent"`
	FixedPrice                   int64             `json:"fixed_price,omitempty"`
	Tags                         []string          `json:"tags,omitempty"`
}
type Warehouse struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Categories      []string `json:"favored_categories,omitempty"`
	RecommendedCash int64    `json:"recommended_cash"`
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

// Config.Qualities contains the five weighted ordinary tiers. Eggs are a
// separate fixed-price tier, sampled relative to the legendary weight.
func CatalogQualities(cfg Config) []Quality {
	return append(append([]Quality{}, cfg.Qualities...), Quality{ID: "easter_egg", Name: "彩蛋", Min: 17500000, Max: 17500000})
}

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
func warehouseCategories(id string) []string {
	for _, box := range Warehouses {
		if box.ID == id {
			return box.Categories
		}
	}
	return nil
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
	return catalogForWarehouse(cfg, "mixed")
}

func (g *Game) Catalog() []ItemTemplate {
	g.mu.Lock()
	defer g.mu.Unlock()
	return catalogForWarehouse(g.gameConfig, g.warehouse)
}

func catalogForWarehouse(cfg Config, warehouse string) []ItemTemplate {
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
	// Baseline for one draw with the full candidate pool and sufficient space.
	// Later placements narrow that pool by their remaining width and height.
	favored := warehouseCategories(warehouse)
	totals := map[string]int{}
	for _, item := range out {
		if item.Width <= cfg.Box.Width && item.Height <= cfg.Box.Height {
			weight := 1
			if slices.Contains(favored, item.Category) && item.Quality != "easter_egg" {
				weight = 3
			}
			totals[item.Quality] += weight
		}
	}
	eggWeight := 0
	if totals["easter_egg"] > 0 {
		for _, q := range cfg.Qualities {
			if q.ID == "legendary" {
				eggWeight = q.Weight
			}
		}
	}
	eggProbability := float64(eggWeight) / float64(100000+eggWeight)
	for n := range out {
		item := &out[n]
		qualityProbability := 0.0
		if item.Quality == "easter_egg" {
			qualityProbability = eggProbability
		} else {
			for _, q := range cfg.Qualities {
				if q.ID == item.Quality {
					// Ordinary quality probabilities are conditional on a non-egg draw.
					qualityProbability = float64(q.Weight) / 100
				}
			}
		}
		item.QualityAppearanceProbability = &qualityProbability
		probability := 0.0
		item.AppearanceProbability = &probability
		if item.Width > cfg.Box.Width || item.Height > cfg.Box.Height {
			continue
		}
		if item.Quality == "easter_egg" {
			probability = qualityProbability / float64(totals[item.Quality])
			continue
		}
		weight := 1
		if slices.Contains(favored, item.Category) {
			weight = 3
		}
		probability = (1 - eggProbability) * qualityProbability * float64(weight) / float64(totals[item.Quality])
	}
	return out
}

func (g *Game) generateCatalog() []Item {
	return g.generateCatalogWith(random)
}

// Supplying draws lets distribution checks use reproducible lots without
// creating scripted players or changing production randomness.
func (g *Game) generateCatalogWith(choose func(int) int) []Item {
	rows := 1 + choose(g.height)
	skyline := make([]int, g.width)
	out := []Item{}
	catalog := Catalog(g.gameConfig)
	legendaryWeight := 0
	for _, q := range g.gameConfig.Qualities {
		if q.ID == "legendary" {
			legendaryWeight = q.Weight
		}
	}
	favored := warehouseCategories(g.warehouse)
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
		quality, isEgg := catalogTier(g.gameConfig.Qualities, eggWeight, choose(100000+eggWeight))
		pool := []ItemTemplate{}
		for _, t := range catalog {
			if t.Category == "easter_egg" || t.Quality != quality || t.Width > span || t.Height > rows-y {
				continue
			}
			pool = append(pool, t)
			if slices.Contains(favored, t.Category) {
				pool = append(pool, t, t)
			}
		}
		if isEgg {
			pool = eggs
		}
		t := pool[choose(len(pool))]
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
		return "easter_egg", true
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
