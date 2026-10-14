package manual

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
)

func TestHandbookPagesAndLinks(t *testing.T) {
	link := regexp.MustCompile(`\]\(([^)]+)\)`)
	err := fs.WalkDir(pages, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := pages.ReadFile(name)
		if err != nil {
			return err
		}
		text := string(data)
		if strings.Contains(text, "PowerShell") || strings.Contains(text, "$env:") || strings.Contains(text, "```bash") {
			t.Errorf("handbook binds instructions to a shell: %s", name)
		}
		for _, match := range link.FindAllStringSubmatch(text, -1) {
			target := path.Clean(path.Join(path.Dir(name), match[1]))
			if _, err := pages.ReadFile(target); err != nil {
				t.Errorf("broken link in %s: %s", name, match[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh-CN", "en"} {
		for _, topic := range []string{"", "start", "play", "rules", "cards", "reconnect", "results"} {
			text, err := Read(topic, lang)
			if err != nil || !strings.HasPrefix(text, "# ") {
				t.Fatalf("missing handbook page for %s/%s: %v", lang, topic, err)
			}
		}
	}
}

func TestCatalogTranslationAndCustomText(t *testing.T) {
	cfg := cards.Default()
	text := Catalog(cfg, game.DefaultConfig(), "en")
	if strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		t.Fatal("built-in English catalog contains untranslated text")
	}
	cfg.Cards[0].Name = "自定义|卡牌"
	cfg.Cards[0].Description = "自定义说明"
	text = Catalog(cfg, game.DefaultConfig(), "en")
	if !strings.Contains(text, `自定义\|卡牌`) || !strings.Contains(text, "自定义说明") {
		t.Fatal("custom card text was translated or not escaped")
	}
	cfg.Cards = nil
	if !strings.Contains(Catalog(cfg, game.DefaultConfig(), "en"), "No cards are available.") {
		t.Fatal("empty English catalog missing explanation")
	}
}
