# 技能卡配置

房主启动时选择一份 JSON 配置，新房间使用所选配置的独立快照。配置公开给玩家；私有用卡结果仍仅使用者可见。运行中不热更新，修改配置后重启宿主生效（已有房间保留自己的配置和持久化状态）。

内置默认定义保存在 `internal/cards/default.json`，通过 Go embed 打包。可以离线导出，再修改后加载，无需重新编译：

```powershell
bidbrawl cards -defaults > cards.json
bidbrawl host -cards-config ./cards.json
```

`bidbrawl cards -room ROOM_ID` 输出当前宿主的实际配置；`bidbrawl handbook -topic cards -live -room ROOM_ID` 将同一配置附在卡牌主题后，加 `-lang en` 查看英文说明。玩家用 `join -cards CARD_ID,CARD_ID` 指定携带的卡；省略 `-cards` 或显式传 `-cards ''` 都不带卡。

## 完整配置示例

下面两张卡有新的 ID 和效果组合；直接保存为 JSON 即可加载：

```json
{
  "schema_version": 1,
  "default_deck": ["giant_audit", "quality_census"],
  "cards": [
    {
      "id": "giant_audit",
      "name": "巨物全检",
      "price": 6000,
      "description": "揭示面积最大物品的品质、价格和位置尺寸。",
      "effect": {
        "type": "reveal",
        "selector": "largest",
        "fields": ["quality", "price", "shape"]
      }
    },
    {
      "id": "quality_census",
      "name": "品质清点",
      "price": 12000,
      "description": "揭示每种品质的物品数量。",
      "effect": {"type": "count", "group_by": "quality"}
    }
  ]
}
```

## 字段与效果

- `schema_version`：当前为 `1`。
- `cards`：可用卡牌列表；删除某个定义即移除该卡。可以为空数组，最多 256 个定义。
- `id`：唯一 ID，满足 `[a-z][a-z0-9_]{0,63}`；引擎按配置查找，不根据具体 ID 判断效果。
- `target`：可省略；或为 category / quality / area / region / item。声明后，用卡时必须传明确目标，不能与 effect 的同维度固定过滤条件冲突。目标只能是常规品类/品质、1～16384 格面积或 northwest / northeast / southwest / southeast 象限；固定卡拒绝多余目标，避免静默忽略。
- `min_round`：最早使用轮次，0 或省略表示第 1 轮，最大为 5。尚未解锁时拒绝用卡，不消耗库存、成本或本轮名额。
- `price`：每张购买价，必须是 1～1,000,000,000 的整数，不支持免费卡。
- `name`、`description`：公开名称与说明，分别最多 80、1000 个字符，非空且不能含控制字符。说明由房主维护，应准确反映 effect；玩家也可直接检查公开的 effect。
- `default_deck`：供玩家查询的预设卡组，最多五个，不重复且必须存在于 cards；可以为空数组。携带时需用 `-cards` 明确指定。

| effect.type | 参数 | 返回信息 |
|---|---|---|
| `reveal` | `selector` 为 `random`、`largest`、`smallest` 或 `highest`；`fields` 为 `quality`、`price`、`shape`、`identity`、`category` 的非空、不重复组合；`count` 为 0～20 | 默认单件含 item_id；指定 count 后返回 items 列表。identity 含名称、模板 ID 和品类 |
| `count` | 可按 quality/category 过滤；`group_by` 可省略，或为 quality/category | 物品总数 count，或各组 counts（包括零数量） |
| `area` | 可按 quality/category 过滤 | 合计占用格数 area |
| `value` | 可按 quality/category 过滤 | 合计价格 total_value |
| `average` | `measure` 为 price/area；可用 `sample_size` 限制最多抽样数量（1～4096），省略为全组均值 | average_price/average_area，空组为 null；抽样时另给 sample_count/sample_limit，不附送总体件数 |

所有效果支持 `quality`（单品质）或 `qualities`（不重复的多个品质，二者互斥）、`category`（目录中的常规品类）和 `area`（恰好占多少格，1～16384）交集过滤。`count` 分组只列过滤范围内的分组，明确保留零值。普通情报不探测彩蛋；审计总价也不含彩蛋。

内置卡现有 31 种：既可查询数量、合并品质数量、面积和均值，也可抽样、辨识身份或查局部总价。广域测绘不揭示品质，六鉴不揭示价格，样本均价不能当成整仓均价。强信息分阶段开放：抽样全检从第 2 轮、品质普查与数码总价从第 3 轮、传说总价从第 4 轮、整仓审计仅第 5 轮。公开说明和 min_round 随房间保存。

所有 `reveal` 效果还会在 `self.item_reveals` 中定位目标、揭示轮廓；品质和价格仅按 `fields` 补充。授权观战接口使用相同的物品视野更新迷雾，计数效果不揭示位置。

largest / smallest 按面积比较，highest 按品质从高到低、同品质面积从大到小比较，再并列时按物品 ID 字典序。reveal 中每个结果的全部字段来自同一件物品。random 在仍有待揭示字段的候选物品中等概率抽取，同一次多件抽样不重复；开局观察、已公开单件线索、已得专长和用卡结果共同决定已知属性。候选耗尽时返回空列表，不额外透露未知属性。

每张卡仍单局一次、每轮报价前最多一张、每人最多携带五张。这些是游戏规则，不由卡牌配置改变。

配置文件最多 1 MiB，未知字段、未知效果或字段、不合法参数、重复 ID、无效默认卡组及尾随 JSON 会使宿主启动失败；不会回退到默认配置或启动半可用服务。配置加载后复制到房间，外部修改原始数据不会改变局内规则。

新增卡牌、组合揭示字段、调整名称与卡组都只需改配置。新增一种不属于上述原语的机制需要扩展引擎和校验；配置不执行脚本或 Go 代码。

## 付费库存与扩展效果

卡牌 definition.price 是每张系统购买价。通过 buy 购买、loadout 选择携带，使用时原子消耗库存，未使用的保留；报价中禁止购买和出售。支持 count 按 quality/category 分组、area/value 聚合与 quality/category 过滤；reveal 支持 count（最多20）、identity/category 字段和 highest selector。随机 reveal 根据本人开局观察、公共单件线索、专长及历史卡结果筛选未知属性。专长结果保存在独立 intelligence 中，不占每轮卡牌名额。

## 根据当前线索选择目标

定向卡的目标在使用时决定，使用 `card -target TARGET`，不是建房或选牌时锁定：

```text
bidbrawl card -room ROOM_ID -round 2 -card category_shapes -target antiques -reason "公开品类线索与我的观察重叠，补查古董轮廓"
bidbrawl card -room ROOM_ID -round 3 -card category_value -target jewelry -reason "只核实珠宝价值，其他品类继续估计"
bidbrawl card -room ROOM_ID -round 2 -card region_census -target southeast -reason "我的已知高品质物集中在右下，查这一分区"
```

定向测绘/品类辨识/品类估价选择一个品类；定档清点选择一个品质；按格估价选择一个面积（宽×高的积，不是尺寸字符串）；分区普查/分区盘估选择一个象限。品类估价第3轮、分区普查第2轮、分区盘估第4轮解锁。边界中线为 floor(箱宽/2)、floor(箱高/2)，物品按左上角唯一归区，跨线不重复计算。

目标记录在使用者的 self.hints[].data.target 与私有行动参数中，结果冻结保存。目标错误、缺省或未解锁都不耗卡，不占名额；合法目标没有物品时正常消耗，计数/总值为零、均值为 null、揭示列表为空。未侦察区域不能从“没有结果”推断为整仓为空。卡牌仍单局一次，不能换目标重用。

定点鉴价（target=item）第2轮起选择自己已观察到的 item_id，补全该件品质、价格和身份。开局抽检、公共单件提示、自己的专长/卡牌结果都可产生候选；目录模板 ID 不等于实际物品 ID。目标必须已经出现在本人的可见记录中，不能根据猜测或其他玩家私有观察查验隐藏物品。它只鉴定一件，可用来验证轮廓或品质候选中的关键物；完整鉴定过的目标返回空结果并正常耗卡，应查看已有结果后决定。

`reveal.unknown_only=true` 可用于极值选择器：先排除本人已知全部请求字段的候选，再比较最大/最小面积或最高品质。默认极值仍查固定目标，random始终筛选未知属性。领价追踪使用此选项连续鉴定高品质目标；候选耗尽返回空列表。
