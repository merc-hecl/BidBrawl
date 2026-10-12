package game

// tileRows fills the occupied prefix without imposing horizontal bands.
func (g *Game) tileRows(rows int) []Item {
	return tileGrid(g.width, rows, g.gameConfig.Items, random)
}

// Each column is filled continuously from the top to its skyline height.
// Placing at the leftmost lowest column, across an equally high run, preserves
// that invariant. At least a 1x1 placement is always available until full.
// choose supplies a uniform integer in [0,n); production uses crypto/rand.
func tileGrid(columns, rows int, limits ItemLimits, choose func(int) int) []Item {
	skyline := make([]int, columns)
	items := []Item{}
	for {
		x := 0
		for column := 1; column < columns; column++ {
			if skyline[column] < skyline[x] {
				x = column
			}
		}
		y := skyline[x]
		if y == rows {
			return items
		}
		span := 1
		for span < limits.MaxWidth && x+span < columns && skyline[x+span] == y {
			span++
		}
		w := 1 + choose(span)
		h := 1 + choose(min(limits.MaxHeight, rows-y))
		items = append(items, Item{TopLeft: Point{x, y}, BottomRight: Point{x + w, y + h}})
		for column := x; column < x+w; column++ {
			skyline[column] = y + h
		}
	}
}
