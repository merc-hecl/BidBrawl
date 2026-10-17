package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bidbrawl/internal/game"
)

// catalog searches public templates only, never the contents of a room's box.
func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	cfg := s.gameConfig
	var templates []game.ItemTemplate
	if r.URL.Query().Get("room") != "" {
		rm := s.getRoom(w, r)
		if rm == nil {
			return
		}
		s.mu.Lock()
		if !s.roomExists(w, rm) {
			s.mu.Unlock()
			return
		}
		cfg = rm.game.GameConfig()
		templates = rm.game.Catalog()
		s.mu.Unlock()
	}
	if templates == nil {
		templates = game.Catalog(cfg)
	}
	q := r.URL.Query()
	name := strings.ToLower(strings.TrimSpace(q.Get("name")))
	quality, category := q.Get("quality"), q.Get("category")
	width, height := 0, 0
	if size := q.Get("size"); size != "" {
		parts := strings.Split(strings.ReplaceAll(strings.ToLower(size), "×", "x"), "x")
		if len(parts) == 2 {
			width, _ = strconv.Atoi(parts[0])
			height, _ = strconv.Atoi(parts[1])
		}
		if width < 1 || height < 1 || width > 128 || height > 128 {
			fail(w, 400, errors.New("size must be WIDTHxHEIGHT in cells (1–128 each)"))
			return
		}
	}
	if quality != "" {
		found := false
		for _, def := range game.CatalogQualities(cfg) {
			if quality == def.ID || quality == def.Name {
				quality, found = def.ID, true
				break
			}
		}
		if !found {
			fail(w, 400, errors.New("unknown quality; use a quality ID or name from the catalog"))
			return
		}
	}
	if len(name) > 256 || category != "" && !game.ValidCategory(category) {
		fail(w, 400, errors.New("name must not exceed 256 bytes; category must be a catalog category ID"))
		return
	}
	items := []game.ItemTemplate{}
	for _, item := range templates {
		if width != 0 && (item.Width != width || item.Height != height) || quality != "" && item.Quality != quality || category != "" && item.Category != category {
			continue
		}
		matches := name == "" || strings.Contains(strings.ToLower(item.Name), name)
		for _, translated := range item.Translations {
			matches = matches || strings.Contains(strings.ToLower(translated), name)
		}
		if matches {
			items = append(items, item)
		}
	}
	result := map[string]any{"catalog": items, "count": len(items)}
	if name == "" && width == 0 && quality == "" && category == "" {
		result["qualities"], result["categories"] = game.CatalogQualities(cfg), game.Categories
		result["specialties"], result["warehouses"] = cfg.Specialties, game.Warehouses
	}
	reply(w, 200, result)
}
