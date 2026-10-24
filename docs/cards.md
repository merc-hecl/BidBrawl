# 技能卡配置

房主启动时选择 JSON 配置，新房间保存独立快照。配置公开，结果只供使用者与授权人类观察者读取。修改配置后重启宿主生效。

```text
bidbrawl cards -defaults
bidbrawl host -cards-config ./cards.json
bidbrawl cards -room ROOM_ID
bidbrawl handbook -topic cards -live -room ROOM_ID
```

默认配置在 `internal/cards/default.json`，共有 16 张卡。公开物品目录与模板价格保留，便于玩家依据线索估价；目录候选不等于箱内已经确认的物品。

## 价格情报边界

- 不支持整仓、品类、品质或分区的准确总价，也不支持精确价格均值或抽样均价。数量与均价不能组合还原总价。
- 公共提示与自动专长不揭示价格、名称或模板身份。身份可查公开目录得到价格，因此与精确价格受相同限制。
- 默认唯一精确鉴价卡为 `focused_appraisal`（定点鉴价）：25,000 金币，第 2 轮起，只鉴定本人已经观察到的一件物品。未拿箱也消耗卡牌。
- 自定义卡凡包含 `price` 或 `identity`，必须声明 `target=item`、`count` 不大于 1、`min_round` 不低于 2、购买价不低于 25,000。不能用别的 ID 绕过。
- 轮廓、品质与品类仍可交叉缩小目录候选；这保留玩家推理空间，不提供身份确认捷径。

## 配置示例

```json
{
  "schema_version": 1,
  "default_deck": ["chosen_appraisal", "quality_census"],
  "cards": [
    {
      "id": "chosen_appraisal",
      "name": "单件鉴价",
      "description": "第2轮起鉴定一件已观察物品的品质、价格与身份。",
      "price": 25000,
      "min_round": 2,
      "target": "item",
      "effect": {"type": "reveal", "selector": "random", "fields": ["quality", "price", "identity"]}
    },
    {
      "id": "quality_census",
      "name": "品质清点",
      "description": "第3轮起显示每种常规品质的数量。",
      "price": 12000,
      "min_round": 3,
      "effect": {"type": "count", "group_by": "quality"}
    }
  ]
}
```

## 字段与效果

`schema_version` 为 1。`cards` 与 `default_deck` 必须为数组，允许为空。最多 256 个定义，预设卡组最多五个不同且存在的 ID。玩家仍须通过 `join -cards` 明确选择；省略或空值都不带卡。

`id` 唯一，匹配 `[a-z][a-z0-9_]{0,63}`。`name` / `description` 为非空无控制字符文本，上限 80 / 1000 字符。购买价 `price` 为 1～1,000,000,000 的整数，精确鉴价受上述最低费用限制。`min_round` 为 0～5，0 或省略表示首轮。

| effect.type | 参数 | 返回信息 |
|---|---|---|
| `reveal` | `selector` 为 random / largest / smallest / highest；`fields` 为 quality / price / shape / identity / category 的非空、不重复组合；`count` 为 0～20 | 默认单件；多件返回 items。价格与身份仅允许付费单件目标 |
| `count` | 可选 `group_by=quality/category` | count 或各组 counts，保留零值 |
| `area` | 过滤后累加占格 | area |
| `average` | 仅 `measure=area`；可选 `sample_size=1～4096` | average_area，空组为 null；抽样另给 sample_count/sample_limit |

所有效果支持 `quality` 或 `qualities`（互斥）、`category`、`area`（恰好占格）、`region` 交集过滤。普通情报不探测彩蛋。价格聚合 `value` 和 `average` 的 `measure=price` 在配置解析时拒绝。

所有 reveal 定位目标并揭示轮廓，其他属性只按 fields 补充。largest / smallest 按面积，highest 按品质、面积降序比较，再按物品 ID 排序。random 从本人仍有未知请求字段的候选中无放回抽样；`unknown_only=true` 也让极值选择器先排除已知候选。候选耗尽返回空列表。

## 主动选择目标

`target` 可省略，或为 category / quality / area / region / item。使用时必须传 `-target`，与固定过滤条件冲突的定义无效。region 为 northwest / northeast / southwest / southeast，按物品左上角归区，中线为 floor(宽/2)、floor(高/2)。

```text
bidbrawl card -room ROOM_ID -round 2 -card category_shapes -target antiques -reason "补查古董轮廓"
bidbrawl card -room ROOM_ID -round 2 -card region_census -target southeast -reason "查已知高品质物所在分区"
bidbrawl card -room ROOM_ID -round 2 -card focused_appraisal -target OBSERVED_ITEM_ID -reason "支付鉴价费用核实关键候选"
```

item 目标必须来自本人的开局抽检、公共单件提示、专长或卡牌记录；公开模板 ID 不能代替实际物品 ID。错误目标、未解锁或已用卡拒绝且不耗库存、费用或轮次名额。合法空范围正常耗卡：数量/占格为零、平均占格为 null、揭示列表为空。完整鉴定过的目标也可能为空，应先检查已有信息。

先用 buy 购买，loadout 选择携带。购买时扣现金，使用时消耗库存，成本按实际购入价记录，未使用的卡保留。精确鉴价的实际购入价也必须达到 25,000，低价库存不能绕过限制。每人最多携带五张不同卡，每轮报价前最多用一张，每张单局一次，报价后不能补用。目标和结果冻结保存，不可换目标重查。

配置最多 1 MiB；未知字段、无效参数、重复 ID、非法卡组及尾随 JSON 均使加载失败。不提供旧价格情报的兼容分支。新增机制需要扩展引擎和校验，配置不执行代码。
