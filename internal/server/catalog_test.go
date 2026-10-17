package server

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
)

func TestCatalogSearchUsesPublicTemplatesAndRoomPrices(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Catalog = []game.ItemTemplate{
		{ID: "vase", Name: "琉璃瓶 & A", Translations: map[string]string{"en": "Glass Vase & A"}, Category: "tools", Quality: "rare", Width: 2, Height: 1, ValuePercent: 50},
		{ID: "other", Name: "竖瓶", Category: "tools", Quality: "rare", Width: 1, Height: 2},
		{ID: "egg", Name: "彩蛋", Category: "easter_egg", Quality: "easter_egg", Width: 1, Height: 1, FixedPrice: 17500000},
	}
	for _, q := range cfg.Qualities {
		cfg.Catalog = append(cfg.Catalog, game.ItemTemplate{ID: "filler_" + q.ID, Name: "基础模板", Category: "tools", Quality: q.ID, Width: 1, Height: 1})
	}
	s := New(cfg, cards.Default())
	defer s.Close()
	created := checkedRequest(t, s, "POST", "/api/create", s.HostToken, `{"name":"Query room","players":2}`, 201)
	var room struct {
		ID string `json:"room_id"`
	}
	if err := json.Unmarshal(created, &room); err != nil {
		t.Fatal(err)
	}
	// Existing rooms retain the configuration they were created with.
	s.gameConfig.Qualities = append([]game.Quality{}, cfg.Qualities...)
	s.gameConfig.Qualities[2].Min, s.gameConfig.Qualities[2].Max = 10000, 20000
	for _, tc := range []struct {
		query, id string
		price     int64
	}{
		{"size=2x1&quality=rare&category=tools", "vase", 15000},
		{"size=1X2", "other", 10000},
		{"size=" + url.QueryEscape("2×1") + "&name=" + url.QueryEscape("琉璃瓶 & A"), "vase", 15000},
		{"name=" + url.QueryEscape("GLASS VASE & A"), "vase", 15000},
		{"quality=" + url.QueryEscape(cfg.Qualities[2].Name) + "&name=Glass", "vase", 15000},
		{"category=easter_egg", "egg", 17500000},
		{"quality=easter_egg", "egg", 17500000},
		{"quality=" + url.QueryEscape("彩蛋"), "egg", 17500000},
		{"room=" + room.ID + "&size=2x1", "vase", 3000},
		{"size=2x1&name=missing", "", 0},
		{"size=2x1&quality=common", "", 0},
	} {
		body := checkedRequest(t, s, "GET", "/api/catalog?"+tc.query, "", "", 200)
		var got struct {
			Catalog []game.ItemTemplate `json:"catalog"`
			Count   int                 `json:"count"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Catalog == nil || got.Count != len(got.Catalog) {
			t.Fatalf("invalid result shape: %s", body)
		}
		if tc.id == "" {
			if got.Count != 0 {
				t.Fatalf("unexpected matches for %s", tc.query)
			}
		} else if got.Count != 1 || got.Catalog[0].ID != tc.id || got.Catalog[0].Price != tc.price {
			t.Fatalf("query %s: %+v", tc.query, got)
		}
		if got.Count > 0 && (got.Catalog[0].AppearanceProbability == nil || *got.Catalog[0].AppearanceProbability <= 0) {
			t.Fatal("query omitted computed appearance probability")
		}
		if got.Count > 0 && (got.Catalog[0].QualityAppearanceProbability == nil || *got.Catalog[0].QualityAppearanceProbability < *got.Catalog[0].AppearanceProbability) {
			t.Fatal("query omitted full quality probability")
		}
		for _, secret := range []string{`"self"`, `"top_left"`, `"settlement"`, `"activities"`, s.HostToken, s.ObserverToken} {
			if strings.Contains(string(body), secret) {
				t.Fatal("private or actual game state escaped catalog")
			}
		}
	}
	for _, query := range []string{"size=0x2", "size=2", "size=2x-1", "size=129x1", "size=2x2x2", "size=1.5x2", "quality=unknown", "category=unknown", "name=" + strings.Repeat("a", 257)} {
		checkedRequest(t, s, "GET", "/api/catalog?"+query, "", "", 400)
	}
	checkedRequest(t, s, "GET", "/api/catalog?room=missing", "", "", 404)
}

func TestCatalogSearchActivityIsPrivateAndCountsMatches(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	a, _ := accountSession(t, s, "Searcher")
	b, _ := accountSession(t, s, "Other")
	for _, tok := range []string{a, b} {
		checkedRequest(t, s, "POST", "/api/join", tok, `{"cards":[]}`, 201)
	}
	checkedRequest(t, s, "GET", "/api/catalog?room=table&name=PRIVATE_QUERY&size=2x2&quality=rare", a, "", 200)
	var own struct {
		Self struct {
			Activities []activity `json:"activities"`
		} `json:"self"`
	}
	if err := json.Unmarshal(checkedRequest(t, s, "GET", "/api/state", a, "", 200), &own); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range own.Self.Activities {
		if e.Action == "catalog" {
			found = true
			if e.Params["name"] != "PRIVATE_QUERY" || e.Params["size"] != "2x2" || e.Params["quality"] != "rare" || e.Result["templates"] != float64(0) {
				t.Fatalf("lost query details: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("missing catalog activity")
	}
	for _, tok := range []string{"", b} {
		if strings.Contains(string(checkedRequest(t, s, "GET", "/api/state", tok, "", 200)), "PRIVATE_QUERY") {
			t.Fatal("query leaked to another viewer")
		}
	}
}
