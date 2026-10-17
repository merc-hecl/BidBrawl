// Package manual embeds the handbook index and topic pages for offline CLI access.
package manual

import (
	"embed"
	"fmt"
	"strings"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
)

//go:embed HANDBOOK.md zh-CN/*.md en/*.md
var pages embed.FS

func Read(topic, lang string) (string, error) {
	if lang != "zh-CN" && lang != "en" {
		return "", fmt.Errorf("unknown handbook language %q; use zh-CN or en", lang)
	}
	path := lang + "/" + topic + ".md"
	switch topic {
	case "":
		path = lang + "/HANDBOOK.md"
		if lang == "zh-CN" {
			path = "HANDBOOK.md"
		}
	case "start", "play", "rules", "cards", "reconnect", "results":
	default:
		return "", fmt.Errorf("unknown handbook topic %q; run bidbrawl handbook for topics", topic)
	}
	data, err := pages.ReadFile(path)
	return string(data), err
}

// Catalog appends configuration data only when the cards topic is requested.
func Catalog(cfg cards.Config, geometry game.Config, lang string) string {
	var b strings.Builder
	if lang == "en" {
		fmt.Fprintf(&b, "\n## Box dimensions\n\nBox: %d columns × %d rows (%d cells). Items: at least 1×1 cell, at most %d columns × %d rows.\n", geometry.Box.Width, geometry.Box.Height, geometry.Box.Width*geometry.Box.Height, geometry.Items.MaxWidth, geometry.Items.MaxHeight)
		fmt.Fprintf(&b, "\n## Card catalog\n\nPreset: `%s`. To carry cards, explicitly list them with `-cards`; omitting it means no cards.\n\n", strings.Join(cfg.DefaultDeck, ","))
		b.WriteString("| Card ID | Name | Price | Description |\n|---|---|---|---|\n")
	} else {
		fmt.Fprintf(&b, "\n## 箱体尺寸\n\n箱体 %d 列 × %d 行，共 %d 格；物品最小 1×1 格，最大 %d 列 × %d 行。\n", geometry.Box.Width, geometry.Box.Height, geometry.Box.Width*geometry.Box.Height, geometry.Items.MaxWidth, geometry.Items.MaxHeight)
		fmt.Fprintf(&b, "\n## 卡牌目录\n\n预设卡组：`%s`，携带时需用 `-cards` 指定；省略则不带卡。\n\n", strings.Join(cfg.DefaultDeck, ","))
		b.WriteString("| 卡 ID | 名称 | 购买价 | 说明 |\n|---|---|---|---|\n")
	}
	escape := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
	defaults := cards.Default().Cards
	for _, card := range cfg.Cards {
		if lang == "en" {
			for _, original := range defaults {
				if card.ID == original.ID && card.Name == original.Name && card.Description == original.Description {
					if text, ok := englishCards[card.ID]; ok {
						card.Name, card.Description = text[0], text[1]
					}
					break
				}
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", card.ID, escape(card.Name), card.Price, escape(card.Description))
	}
	if len(cfg.Cards) == 0 {
		if lang == "en" {
			b.WriteString("\nNo cards are available.\n")
		} else {
			b.WriteString("\n本配置没有可用技能卡。\n")
		}
	}
	if lang == "en" {
		fmt.Fprintf(&b, "\n## Prices and assets\n\nNew accounts receive %d coins once. Cash is stored directly. Buy cards before bidding; consumed cards count as costs. Winning deducts the bid and deposits items into your warehouse; sell to the system outside bidding for cash. Your bid limit is self.max_bid (cash only). Only one unfinished room per account, including waiting rooms.\n\n| Quality ID | Price range (coins) | Generation weight |\n|---|---|---|\n", geometry.InitialAssets)
	} else {
		fmt.Fprintf(&b, "\n## 品质价格与资产\n\n新账号一次性获得 %d 金币。购买卡牌扣现金，使用时消耗库存；成交只扣报价、藏品入库，非竞拍时向系统出售才增加现金。报价上限见 self.max_bid，仅包含现金。同一账号同时只能加入一个未结算房间，包括等候中的房间。\n\n| 品质 ID | 单件价格范围（金币） | 生成权重 |\n|---|---|---|\n", geometry.InitialAssets)
	}
	for _, q := range geometry.Qualities {
		fmt.Fprintf(&b, "| %s | %d–%d | %d%% |\n", q.ID, q.Min, q.Max, q.Weight)
	}
	return b.String()
}

var englishCards = map[string][2]string{
	"sample_four":     {"Sample appraisal", "Fully appraise four items with unknown attributes."},
	"category_counts": {"Category census", "Show item counts by category."},
	"legendary_area":  {"Legendary scan", "Show total cells occupied by legendary items."},
	"legendary_value": {"Legendary valuation", "Show the total buyback value of legendary items."},
	"audit":           {"Full audit", "Show the exact total warehouse buyback value."},
	"largest_quality": {"Largest-item appraisal", "Reveals the quality of the largest item by area; ties use the first item in ID order."},
	"largest_price":   {"Largest-item valuation", "Reveals the actual price of the largest item by area."},
	"random_shape":    {"Shape scan", "Reveals the ID and two corner coordinates of one random item."},
	"random_quality":  {"Random appraisal", "Reveals the ID and quality of one random item."},
	"item_count":      {"Item count", "Reveals the total number of items."},
	"quality_counts":  {"Quality census", "Reveals the number of items in each quality tier."},
}
