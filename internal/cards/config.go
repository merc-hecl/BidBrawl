// Package cards defines the declarative, versioned skill-card configuration.
package cards

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed default.json
var defaultJSON []byte

type Effect struct {
	Type       string   `json:"type"`
	Selector   string   `json:"selector,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	GroupBy    string   `json:"group_by,omitempty"`
	Category   string   `json:"category,omitempty"`
	Quality    string   `json:"quality,omitempty"`
	Count      int      `json:"count,omitempty"`
	Qualities  []string `json:"qualities,omitempty"`
	Area       int      `json:"area,omitempty"`
	Measure    string   `json:"measure,omitempty"`
	SampleSize int      `json:"sample_size,omitempty"`
}

type Definition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Effect      Effect `json:"effect"`
	Price       int64  `json:"price"`
	MinRound    int    `json:"min_round,omitempty"`
}

type Config struct {
	SchemaVersion int          `json:"schema_version"`
	DefaultDeck   []string     `json:"default_deck"`
	Cards         []Definition `json:"cards"`
}

func Default() Config {
	cfg, err := Parse(bytes.NewReader(defaultJSON))
	if err != nil {
		panic("invalid embedded card configuration: " + err.Error())
	}
	return cfg
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	cfg, err := Parse(f)
	if err != nil {
		return Config{}, fmt.Errorf("cards config %s: %w", path, err)
	}
	return cfg, nil
}

func Parse(r io.Reader) (Config, error) {
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return Config{}, err
	}
	if len(data) > limit {
		return Config{}, errors.New("cards config exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var cfg Config
	if err := d.Decode(&cfg); err != nil {
		return Config{}, err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return Config{}, errors.New("expected exactly one configuration object")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (c Config) Validate() error {
	if c.SchemaVersion != 1 {
		return errors.New("schema_version must be 1")
	}
	if c.Cards == nil || c.DefaultDeck == nil {
		return errors.New("cards and default_deck must be arrays; use [] for none")
	}
	if len(c.Cards) > 256 {
		return errors.New("at most 256 card definitions")
	}
	known := map[string]bool{}
	for _, card := range c.Cards {
		if card.MinRound < 0 || card.MinRound > 5 {
			return errors.New("min_round must be 0 to 5")
		}
		if card.Price <= 0 || card.Price > 1_000_000_000 {
			return errors.New("card price must be 1 to 1000000000")
		}
		if !validID.MatchString(card.ID) || known[card.ID] {
			return fmt.Errorf("invalid or duplicate card id: %q", card.ID)
		}
		known[card.ID] = true
		if !validText(card.Name, 80) || !validText(card.Description, 1000) {
			return fmt.Errorf("card %s: name and description must be nonempty text (max 80 / 1000 characters)", card.ID)
		}
		if err := card.Effect.validate(); err != nil {
			return fmt.Errorf("card %s: %w", card.ID, err)
		}
	}
	if len(c.DefaultDeck) > 5 {
		return errors.New("default_deck may contain at most 5 different cards")
	}
	seen := map[string]bool{}
	for _, id := range c.DefaultDeck {
		if !known[id] || seen[id] {
			return fmt.Errorf("default_deck contains unknown or duplicate card: %s", id)
		}
		seen[id] = true
	}
	return nil
}

func validText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && strings.IndexFunc(s, unicode.IsControl) == -1
}

func (e Effect) validate() error {
	switch e.Type {
	case "reveal":
		if e.Selector != "random" && e.Selector != "largest" && e.Selector != "smallest" && e.Selector != "highest" {
			return errors.New("reveal selector must be random, largest, smallest or highest")
		}
		if len(e.Fields) < 1 || len(e.Fields) > 3 || e.GroupBy != "" {
			return errors.New("reveal needs 1–3 fields and does not accept group_by")
		}
		seen := map[string]bool{}
		for _, field := range e.Fields {
			if (field != "quality" && field != "price" && field != "shape" && field != "identity" && field != "category") || seen[field] {
				return fmt.Errorf("unknown or duplicate reveal field: %s", field)
			}
			seen[field] = true
		}
	case "count":
		if e.Selector != "" || len(e.Fields) != 0 || (e.GroupBy != "" && e.GroupBy != "quality" && e.GroupBy != "category") {
			return errors.New("count accepts only optional group_by=quality/category")
		}
	case "value", "area":
		if e.Selector != "" || len(e.Fields) != 0 || e.GroupBy != "" {
			return errors.New("value/area only accept filters")
		}
	case "average":
		if e.Selector != "" || len(e.Fields) != 0 || e.GroupBy != "" || (e.Measure != "price" && e.Measure != "area") {
			return errors.New("average requires measure=price/area and only accepts filters and sample_size")
		}
	default:
		return fmt.Errorf("unknown effect type: %q", e.Type)
	}
	if e.Type != "average" && (e.Measure != "" || e.SampleSize != 0) || e.SampleSize < 0 || e.SampleSize > 4096 {
		return errors.New("measure/sample_size require average; sample_size must be 0 to 4096")
	}
	if e.Area < 0 || e.Area > 16384 {
		return errors.New("area filter must be 0 to 16384")
	}
	if e.Quality != "" && len(e.Qualities) > 0 || len(e.Qualities) > 5 {
		return errors.New("use quality or qualities, with at most five tiers")
	}
	seen := map[string]bool{}
	for _, q := range append(append([]string{}, e.Qualities...), e.Quality) {
		if q == "" {
			continue
		}
		if seen[q] || !validQuality(q) {
			return errors.New("unknown or duplicate quality filter")
		}
		seen[q] = true
	}
	if e.Count != 0 && e.Type != "reveal" {
		return errors.New("count is only used by reveal")
	}
	if e.Count < 0 || e.Count > 20 {
		return errors.New("count must be 0 to 20")
	}
	if e.Category != "" && !ValidCategory(e.Category) {
		return errors.New("unknown category filter")
	}
	return nil
}

func validQuality(q string) bool {
	return q == "common" || q == "uncommon" || q == "rare" || q == "epic" || q == "legendary" || q == "easter_egg"
}

// ValidCategory mirrors the public catalog IDs, including cross-category specialties.
func ValidCategory(id string) bool {
	switch id {
	case "furniture", "household", "electronics", "computers", "tools", "machinery", "art", "antiques", "jewelry", "minerals", "transport", "energy", "books", "documents", "music", "toys", "sports", "medical", "military", "collectibles", "easter_egg":
		return true
	}
	return false
}

// Clone keeps a room's configuration independent of callers and other rooms.
func (c Config) Clone() Config {
	out := Config{SchemaVersion: c.SchemaVersion, DefaultDeck: append([]string{}, c.DefaultDeck...), Cards: append([]Definition{}, c.Cards...)}
	for i := range out.Cards {
		out.Cards[i].Effect.Qualities = append([]string(nil), c.Cards[i].Effect.Qualities...)
		if c.Cards[i].Effect.Fields != nil {
			out.Cards[i].Effect.Fields = append([]string{}, c.Cards[i].Effect.Fields...)
		}
	}
	return out
}
