# 卡牌与物品

## 卡牌

入席时用 `-cards CARD_ID,CARD_ID` 携带最多五张不同卡，省略或传空值则不带卡。每轮报价前最多用一张，每张单局限用一次，报价后不能补用。

`self.deck` 是携带卡牌，`self.used` 是已消耗卡牌，`self.card_used_this_round` 表示本轮是否已用卡。卡牌名称和说明见 `bidbrawl cards -room ROOM_ID`，结果保留在 `self.hints`。

## 线索与坐标

第 1、3 轮各发布一条公共线索，见 `public_hints`；已揭晓报价见 `history`。技能结果和当前暗标只对本人可见。

箱内物品为矩形，数量、位置、品质和价格在揭示前隐藏。物品间无空隙或重叠，空白只在底部。坐标以格为单位，左上角为 `(0,0)`，x 向右、y 向下；右下角为不包含的边界，例如 `(2,3) → (6,7)` 占 4×4 格。价值区间 `[下界, 上界)` 包含下界、不包含上界。

## 查询目录

```text
bidbrawl handbook -topic cards
bidbrawl handbook -topic cards -live -room ROOM_ID
```

第一条在本页后附上内置默认尺寸和卡牌目录；第二条附上当前房间的实际配置。`-live` 不带 `-room` 时读取宿主配置。也可分别使用 `config -room ROOM_ID`、`cards -room ROOM_ID` 查询。

[返回目录](../HANDBOOK.md)：`bidbrawl handbook`。
