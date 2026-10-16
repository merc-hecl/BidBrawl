package game

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

//go:embed default.json
var defaultConfigJSON []byte

// Dimensions counts grid columns and rows, never pixels or physical lengths.
type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ItemLimits caps the number of grid columns and rows occupied by one item.
type ItemLimits struct {
	MaxWidth  int `json:"max_width"`
	MaxHeight int `json:"max_height"`
}

type Config struct {
	SchemaVersion int        `json:"schema_version"`
	Box           Dimensions `json:"box"`
	Items         ItemLimits `json:"items"`
	Qualities     []Quality  `json:"qualities"`
	InitialAssets int64      `json:"initial_assets"`
}

func (c Config) Clone() Config {
	c.Qualities = slices.Clone(c.Qualities)
	return c
}

func DefaultConfig() Config {
	cfg, err := ParseConfig(bytes.NewReader(defaultConfigJSON))
	if err != nil {
		panic("invalid embedded game configuration: " + err.Error())
	}
	return cfg
}

func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	cfg, err := ParseConfig(f)
	if err != nil {
		return Config{}, fmt.Errorf("game config %s: %w", path, err)
	}
	return cfg, nil
}

func ParseConfig(r io.Reader) (Config, error) {
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return Config{}, err
	}
	if len(data) > limit {
		return Config{}, errors.New("game config exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var cfg Config
	if err := d.Decode(&cfg); err != nil {
		return Config{}, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return Config{}, errors.New("expected exactly one configuration object")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.SchemaVersion != 2 {
		return errors.New("game schema_version must be 2")
	}
	if c.Box.Width < 1 || c.Box.Width > 128 || c.Box.Height < 1 || c.Box.Height > 128 || c.Box.Width*c.Box.Height > 4096 {
		return errors.New("box column and row counts must be 1–128 with at most 4096 cells")
	}
	if c.Items.MaxWidth < 1 || c.Items.MaxWidth > 128 || c.Items.MaxHeight < 1 || c.Items.MaxHeight > 128 {
		return errors.New("item maximum column and row counts must be 1–128; minimum occupancy is 1x1 cells")
	}
	if c.InitialAssets < 1 || c.InitialAssets > 1_000_000_000_000 {
		return errors.New("initial_assets must be between 1 and 1000000000000")
	}
	ids := map[string]bool{"common": false, "uncommon": false, "rare": false, "epic": false, "legendary": false}
	if len(c.Qualities) != len(ids) {
		return errors.New("qualities must contain each of the five supported tiers exactly once")
	}
	weight := 0
	for _, q := range c.Qualities {
		seen, known := ids[q.ID]
		if !known || seen || strings.TrimSpace(q.Name) == "" || len([]rune(q.Name)) > 32 {
			return errors.New("invalid or duplicate quality ID or name")
		}
		ids[q.ID] = true
		if q.Min < 0 || q.Max < q.Min || q.Max > 1_000_000_000 || q.Weight < 0 || q.Weight > 100 {
			return errors.New("quality prices must satisfy 0 <= min_price <= max_price <= 1000000000; weight must be 0 to 100")
		}
		weight += q.Weight
	}
	if weight != 100 {
		return errors.New("quality weights must add up to 100")
	}
	return nil
}
