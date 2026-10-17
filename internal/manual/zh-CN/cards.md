# 卡牌与物品

## 卡牌

先用 `buy -card CARD_ID -quantity N` 购买。价格见 cards，库存见 inventory；准备阶段允许临时购买和调整 loadout。用卡时消耗一张，未用卡保留，现金在购买时扣除，不重复扣款。随机卡优先补充本人尚未知晓的属性，候选耗尽时返回空结果；请先查看已有信息。

入席时用 `-cards CARD_ID,CARD_ID` 携带最多五张不同卡，省略或传空值则不带卡。每轮报价前最多用一张，每张单局限用一次，报价后不能补用。

`self.deck` 是携带卡牌，`self.used` 是已消耗卡牌，`self.card_used_this_round` 表示本轮是否已用卡。卡牌名称和说明见 `bidbrawl cards -room ROOM_ID`，结果保留在 `self.hints`。

单件物品技能均揭示目标的轮廓和位置，并按技能效果揭示品质或价格；累积的单件信息在 `self.item_reveals`，原始结果在 `self.hints`。授权人类观战页使用同一玩家视野更新迷雾，例如巨物估价揭开最大物品并显示价格，但不额外透露品质。清点类结果不消除空间迷雾，已发现信息保留到后续轮次；回放不会显示未来技能信息。

## 线索与坐标

第 1、3 轮各发布一条公共线索，见 `public_hints`；已揭晓报价见 `history`。技能结果和当前暗标只对本人可见。

开局时，每位玩家独立从全部物品中等概率、无放回抽取 `min(3, 物品总数)` 件。每人数量相同，每件物品被任意玩家抽中的概率相同，不按品质加权；不同玩家的样本允许重叠。`self.initial_inspection` 仅包含抽中物品的 ID、轮廓坐标和品质，不包含价格。结果随对局保存，重连或重启不会重抽。

箱内物品为矩形，数量、位置、品质和价格在揭示前隐藏。物品间无空隙或重叠，空白只在底部。坐标以格为单位，左上角为 `(0,0)`，x 向右、y 向下；右下角为不包含的边界，例如 `(2,3) → (6,7)` 占 4×4 格。

## 查询目录

```text
bidbrawl handbook -topic cards
bidbrawl handbook -topic cards -live -room ROOM_ID
```

第一条在本页后附上内置默认尺寸和卡牌目录；第二条附上当前房间的实际配置。`-live` 不带 `-room` 时读取宿主配置。也可分别使用 `config -room ROOM_ID`、`cards -room ROOM_ID` 查询。

[返回目录](../HANDBOOK.md)：`bidbrawl handbook`。

物品目录使用独立 JSON，包含名称、类别、品质、占格及价值定位；实际回收价由品质区间与 value_percent 计算并在公开目录列出。轮廓不唯一，品质或类别线索可以缩小候选。首版不制作立绘。使用 `catalog` 查询。
