package game

import (
	"encoding/json"
	"sync"
	"testing"
)

func view(t *testing.T, g *Game, token string) View {
	t.Helper()
	b, e := g.Observe(token)
	if e != nil {
		t.Fatal(e)
	}
	var v View
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func fixture(t *testing.T, n int) (*Game, []string) {
	t.Helper()
	g, e := New(n)
	if e != nil {
		t.Fatal(e)
	}
	tokens := []string{}
	for i := 0; i < n; i++ {
		_, token, e := g.Join(string(rune('A'+i)), []string{"largest_price", "random_shape", "item_count", "quality_counts", "value_band"})
		if e != nil {
			t.Fatal(e)
		}
		tokens = append(tokens, token)
	}
	if e = g.Start(); e != nil {
		t.Fatal(e)
	}
	return g, tokens
}

func TestLobbyValidation(t *testing.T) {
	room, _ := New(4)
	_, _, _ = room.Join("A", nil)
	_, _, _ = room.Join("B", nil)
	if e := room.Start(); e == nil {
		t.Fatal("started before configured players arrived")
	}
	for _, n := range []int{0, 1, 8} {
		if _, e := New(n); e == nil {
			t.Fatalf("accepted %d players", n)
		}
	}
	g, _ := New(2)
	if e := g.Start(); e == nil {
		t.Fatal("started empty game")
	}
	for _, deck := range [][]string{{"bad"}, {"item_count", "item_count"}, {"largest_price", "random_shape", "item_count", "quality_counts", "value_band", "largest_quality"}} {
		if _, _, e := g.Join("A", deck); e == nil {
			t.Fatal("invalid deck accepted")
		}
	}
	if _, _, e := g.Join("A", nil); e != nil {
		t.Fatal(e)
	}
	if e := g.Start(); e == nil {
		t.Fatal("started with one player")
	}
	if _, _, e := g.Join("A", nil); e == nil {
		t.Fatal("duplicate name accepted")
	}
	_, _, _ = g.Join("B", nil)
	if _, _, e := g.Join("C", nil); e == nil {
		t.Fatal("capacity exceeded")
	}
	if e := g.Start(); e != nil {
		t.Fatal(e)
	}
	if _, _, e := g.Join("D", nil); e == nil {
		t.Fatal("joined started game")
	}
	if e := g.Start(); e == nil {
		t.Fatal("started twice")
	}
}
func TestPrivateObservationsAndSealedBids(t *testing.T) {
	g, ts := fixture(t, 2)
	before, _ := g.Observe("")
	if e := g.UseCard(ts[0], 1, "largest_price", "protocol test"); e != nil {
		t.Fatal(e)
	}
	after, _ := g.Observe("")
	if string(before) != string(after) {
		t.Fatal("private card use changed public observation")
	}
	a, b := view(t, g, ts[0]), view(t, g, ts[1])
	if len(a.Self.Hints) != 1 || len(b.Self.Hints) != 0 {
		t.Fatal("private hint isolation failed")
	}
	if e := g.UseCard(ts[0], 1, "item_count", "protocol test"); e == nil {
		t.Fatal("two cards in round accepted")
	}
	if e := g.UseCard(ts[1], 1, "largest_quality", "protocol test"); e == nil {
		t.Fatal("non-deck card accepted")
	}
	if e := g.Submit(ts[0], 1, 987654, "protocol test"); e != nil {
		t.Fatal(e)
	}
	pub := view(t, g, "")
	if pub.Self != nil || pub.Settlement != nil || len(pub.History) != 0 {
		t.Fatal("sealed game leaked")
	}
	other := view(t, g, ts[1])
	if other.Self.Bid != nil {
		t.Fatal("other player sees own bid before submission")
	}
	if got := view(t, g, ts[0]); got.Self.Bid == nil || *got.Self.Bid != 987654 {
		t.Fatal("own bid not returned")
	}
	if e := g.UseCard(ts[0], 1, "item_count", "protocol test"); e == nil {
		t.Fatal("card after bid accepted")
	}
	if e := g.Submit(ts[0], 1, 1, "protocol test"); e == nil {
		t.Fatal("duplicate bid accepted")
	}
	if _, e := g.Observe("bad"); e == nil {
		t.Fatal("unknown token accepted")
	}
	if e := g.Submit("bad", 1, 1, "protocol test"); e == nil {
		t.Fatal("unauthorized bid accepted")
	}
	if e := g.Submit(ts[1], 1, 600000, "protocol test"); e != nil {
		t.Fatal(e)
	}
	pub = view(t, g, "")
	if pub.Round != 2 || len(pub.History) != 1 || pub.History[0].Bids[0].Amount != 987654 {
		t.Fatal("bids not simultaneously revealed")
	}
	if e := g.Submit(ts[0], 1, 1, "protocol test"); e == nil {
		t.Fatal("stale round accepted")
	}
	if e := g.UseCard(ts[0], 2, "largest_price", "protocol test"); e == nil {
		t.Fatal("card reused")
	}
	if e := g.Submit(ts[0], 2, 0, "protocol test"); e != nil {
		t.Fatal("decreasing bid should be allowed", e)
	}
}
func TestFiveRoundsSettlementAndHints(t *testing.T) {
	g, ts := fixture(t, 4)
	if n := len(view(t, g, "").PublicHints); n != 1 {
		t.Fatal(n)
	}
	for round := 1; round <= 5; round++ {
		if e := g.Submit(ts[0], round, -1, "protocol test"); e == nil {
			t.Fatal("negative bid accepted")
		}
		if e := g.Submit(ts[0], round, MaxBid+1, "protocol test"); e == nil {
			t.Fatal("excess bid accepted")
		}
		for i, token := range ts {
			amount := int64(10000 - i)
			if round == 5 {
				amount = int64(100 + i)
			}
			if e := g.Submit(token, round, amount, "protocol test"); e != nil {
				t.Fatal(e)
			}
		}
		v := view(t, g, "")
		expected := 1
		if round >= 2 {
			expected = 2
		}
		if len(v.PublicHints) != expected {
			t.Fatal("wrong reveal timing", round)
		}
		if round < 5 && v.Settlement != nil {
			t.Fatal("early settlement")
		}
	}
	v := view(t, g, "")
	r := v.Settlement
	if v.Phase != "settled" || len(v.History) != 5 || r == nil {
		t.Fatal("missing settlement")
	}
	if r.WinnerID != "p4" || r.Paid != 103 || r.Profit != r.Total-103 {
		t.Fatalf("bad settlement: %+v", r)
	}
	if v.PublicHints[0].Kind == v.PublicHints[1].Kind {
		t.Fatal("duplicate public hint kind")
	}
	var total int64
	for _, i := range r.Items {
		total += i.Price
	}
	if total != r.Total {
		t.Fatal("total mismatch")
	}
	if r.Payoffs["p1"] != 0 || r.Payoffs["p4"] != r.Profit {
		t.Fatal("wrong payoffs")
	}
	if e := g.Submit(ts[0], 5, 0, "protocol test"); e == nil {
		t.Fatal("post-settlement bid accepted")
	}
}
func TestGeometryAndPriceRanges(t *testing.T) {
	for trial := 0; trial < 100; trial++ {
		g, ts := fixture(t, 2)
		assertTiledRows(t, g.items, g.GameConfig())
		ids := map[string]bool{}
		occupied := map[Point]bool{}
		for _, i := range g.items {
			if ids[i.ID] {
				t.Fatal("duplicate id")
			}
			ids[i.ID] = true
			if i.TopLeft.X < 0 || i.TopLeft.Y < 0 || i.BottomRight.X > g.width || i.BottomRight.Y > g.height || i.area() <= 0 {
				t.Fatal("invalid rectangle", i)
			}
			if w, h := i.BottomRight.X-i.TopLeft.X, i.BottomRight.Y-i.TopLeft.Y; w < 1 || w > g.gameConfig.Items.MaxWidth || h < 1 || h > g.gameConfig.Items.MaxHeight {
				t.Fatal("invalid sampled dimensions", i)
			}
			for y := i.TopLeft.Y; y < i.BottomRight.Y; y++ {
				for x := i.TopLeft.X; x < i.BottomRight.X; x++ {
					occupied[Point{x, y}] = true
				}
			}
			valid := false
			for _, q := range qualities {
				if i.Quality == q.ID {
					valid = i.Price >= q.Min && i.Price <= q.Max
				}
			}
			if !valid {
				t.Fatal("invalid quality/price")
			}
			for _, j := range g.items {
				if i.ID != j.ID && i.TopLeft.X < j.BottomRight.X && i.BottomRight.X > j.TopLeft.X && i.TopLeft.Y < j.BottomRight.Y && i.BottomRight.Y > j.TopLeft.Y {
					t.Fatal("overlap")
				}
			}
		}
		// Flood-fill occupied cells independently of the placement algorithm.
		// Corner-only contact and separated groups cannot pass this check.
		if !occupied[Point{0, 0}] {
			t.Fatal("cluster does not start at the top left")
		}
		seen := map[Point]bool{{0, 0}: true}
		queue := []Point{{0, 0}}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, d := range []Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				next := Point{p.X + d.X, p.Y + d.Y}
				if occupied[next] && !seen[next] {
					seen[next] = true
					queue = append(queue, next)
				}
			}
		}
		if len(seen) != len(occupied) {
			t.Fatal("items are scattered into disconnected groups")
		}
		if e := g.UseCard(ts[0], 1, "value_band", "protocol test"); e != nil {
			t.Fatal(e)
		}
		v := view(t, g, ts[0])
		d := v.Self.Hints[0].Data
		if float64(g.total()) < d["lower_inclusive"].(float64) || float64(g.total()) >= d["upper_exclusive"].(float64) {
			t.Fatal("incorrect value band")
		}
	}
}
func TestConcurrentSubmissionsAndTies(t *testing.T) {
	g, ts := fixture(t, 7)
	for r := 1; r <= 5; r++ {
		var wg sync.WaitGroup
		errs := make(chan error, 7)
		for _, token := range ts {
			wg.Add(1)
			go func(token string) {
				defer wg.Done()
				_, e := g.Observe(token)
				if e != nil {
					errs <- e
				}
				if e := g.Submit(token, r, 0, "protocol test"); e != nil {
					errs <- e
				}
			}(token)
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			t.Fatal(e)
		}
	}
	v := view(t, g, "")
	if len(v.History) != 5 || len(v.Settlement.Tied) != 7 || v.Settlement.Paid != 0 || !contains(v.Settlement.Tied, v.Settlement.WinnerID) {
		t.Fatal("incorrect tie resolution")
	}
}
