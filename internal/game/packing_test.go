package game

import (
	"math/rand/v2"
	"testing"
)

// Rasterize independently: every occupied row must contain exactly one item
// per cell from x=0 to the right edge, with all remaining rows empty.
func assertTiledRows(t *testing.T, items []Item, cfg Config) int {
	t.Helper()
	if len(items) == 0 {
		t.Fatal("empty generated box")
	}
	cells := make([]int, cfg.Box.Width*cfg.Box.Height)
	rows := 0
	for _, item := range items {
		w, h := item.BottomRight.X-item.TopLeft.X, item.BottomRight.Y-item.TopLeft.Y
		if w < 1 || h < 1 || w > cfg.Items.MaxWidth || h > cfg.Items.MaxHeight || item.TopLeft.X < 0 || item.TopLeft.Y < 0 || item.BottomRight.X > cfg.Box.Width || item.BottomRight.Y > cfg.Box.Height {
			t.Fatal("invalid item rectangle", item, cfg)
		}
		rows = max(rows, item.BottomRight.Y)
		for y := item.TopLeft.Y; y < item.BottomRight.Y; y++ {
			for x := item.TopLeft.X; x < item.BottomRight.X; x++ {
				cells[y*cfg.Box.Width+x]++
			}
		}
	}
	for y := 0; y < cfg.Box.Height; y++ {
		expected := 0
		if y < rows {
			expected = 1
		}
		for x := 0; x < cfg.Box.Width; x++ {
			if count := cells[y*cfg.Box.Width+x]; count != expected {
				t.Fatalf("hole, overlap or partial row at (%d,%d): occupancy %d, expected %d", x, y, count, expected)
			}
		}
	}
	return rows
}

func TestTilingDoesNotForceHorizontalBands(t *testing.T) {
	cfg := DefaultConfig()
	rng := rand.New(rand.NewPCG(42, 77))
	const rows, layouts = 24, 100
	seams := 0
	for trial := 0; trial < layouts; trial++ {
		items := tileGrid(cfg.Box.Width, rows, cfg.Items, rng.IntN)
		assertTiledRows(t, items, cfg)
		// A seam runs across the whole box iff no item crosses that grid line.
		for y := 1; y < rows; y++ {
			crossed := false
			for _, item := range items {
				if item.TopLeft.Y < y && item.BottomRight.Y > y {
					crossed = true
					break
				}
			}
			if !crossed {
				seams++
			}
		}
	}
	// The former bands required at least five seams per 24-row layout.
	if seams > layouts {
		t.Fatalf("too many full-width horizontal seams: %d in %d layouts", seams, layouts)
	}
	t.Logf("full-width internal horizontal seams: %d across %d layouts", seams, layouts)
}

func TestItemCountVariesEvenWithSameFilledRows(t *testing.T) {
	cfg := DefaultConfig()
	rng := rand.New(rand.NewPCG(19, 83))
	counts := map[int]bool{}
	samples := []int{}
	for trial := 0; trial < 100; trial++ {
		items := tileGrid(cfg.Box.Width, 24, cfg.Items, rng.IntN)
		if rows := assertTiledRows(t, items, cfg); rows != 24 {
			t.Fatal("comparison layouts must occupy the same 24 rows", rows)
		}
		counts[len(items)] = true
		if trial < 10 {
			samples = append(samples, len(items))
		}
	}
	if len(counts) < 2 {
		t.Fatal("random dimensions produced a fixed item count", counts)
	}
	t.Logf("same 24 filled rows: first ten item counts %v; %d distinct counts in 100 layouts", samples, len(counts))
}

func TestRowTiling(t *testing.T) {
	for _, cfg := range []Config{
		DefaultConfig(),
		geometryConfig(Dimensions{31, 7}, ItemLimits{7, 3}),
		geometryConfig(Dimensions{1, 8}, ItemLimits{4, 4}),
		geometryConfig(Dimensions{8, 1}, ItemLimits{4, 4}),
		geometryConfig(Dimensions{1, 1}, ItemLimits{4, 4}),
		geometryConfig(Dimensions{20, 30}, ItemLimits{1, 1}),
	} {
		g, _ := New(2)
		g.gameConfig, g.width, g.height = cfg, cfg.Box.Width, cfg.Box.Height
		for trial := 0; trial < 100; trial++ {
			rows := 1 + trial%cfg.Box.Height
			if actual := assertTiledRows(t, g.tileRows(rows), cfg); actual != rows {
				t.Fatal("wrong number of filled rows", actual, rows)
			}
		}
	}
}

func TestRowTilingMaximumCapacity(t *testing.T) {
	cfg := geometryConfig(Dimensions{64, 64}, ItemLimits{1, 1})
	g, _ := New(2)
	g.gameConfig, g.width, g.height = cfg, cfg.Box.Width, cfg.Box.Height
	items := g.tileRows(cfg.Box.Height)
	if len(items) != 4096 || assertTiledRows(t, items, cfg) != cfg.Box.Height {
		t.Fatal("unit items do not tile the maximum capacity")
	}
}
