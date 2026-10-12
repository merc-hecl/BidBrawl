// Package manual embeds the game handbook for offline CLI access.
package manual

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
)

//go:embed HANDBOOK.md
var Text string

// Render derives the card tutorial from the same configuration the engine uses.
func Render(cfg cards.Config, geometry game.Config, scope string) string {
	var b strings.Builder
	b.WriteString(Text)
	fmt.Fprintf(&b, "\n## 游戏配置（%s）\n\n尺寸单位均为格子数：箱体 %d 列 × %d 行，共 %d 格；物品最小占 1×1 格，最多占 %d 列 × %d 行。完整行无缝填充，剩余空白仅在底部。\n", scope, geometry.Box.Width, geometry.Box.Height, geometry.Box.Width*geometry.Box.Height, geometry.Items.MaxWidth, geometry.Items.MaxHeight)
	fmt.Fprintf(&b, "\n## 卡牌目录（%s）\n\n", scope)
	fmt.Fprintf(&b, "默认卡组：`%s`。\n\n", strings.Join(cfg.DefaultDeck, ","))
	b.WriteString("| 卡 ID | 名称 | 说明 | 实际效果 |\n|---|---|---|---|\n")
	escape := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
	for _, card := range cfg.Cards {
		effect, _ := json.Marshal(card.Effect)
		fmt.Fprintf(&b, "| %s | %s | %s | `%s` |\n", card.ID, escape(card.Name), escape(card.Description), effect)
	}
	if len(cfg.Cards) == 0 {
		b.WriteString("\n本配置没有可用技能卡。\n")
	}
	return b.String()
}
