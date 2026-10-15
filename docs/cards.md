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
- `name`、`description`：公开名称与说明，分别最多 80、1000 个字符，非空且不能含控制字符。说明由房主维护，应准确反映 effect；玩家也可直接检查公开的 effect。
- `default_deck`：供玩家查询的预设卡组，最多五个，不重复且必须存在于 cards；可以为空数组。携带时需用 `-cards` 明确指定。

| effect.type | 参数 | 返回信息 |
|---|---|---|
| `reveal` | `selector` 为 `random`、`largest` 或 `smallest`；`fields` 为 `quality`、`price`、`shape` 的非空、不重复组合 | 总是含 `item_id`；quality 为品质、price 为整数价格、shape 为 top_left / bottom_right 坐标 |
| `count` | 不带参数，或 `group_by: "quality"` | 物品总数 count，或每种品质的 counts（包括零数量） |

所有 `reveal` 效果还会在 `self.item_reveals` 中定位目标、揭示轮廓；品质和价格仅按 `fields` 补充。授权观战接口使用相同的物品视野更新迷雾，计数效果不揭示位置。

largest / smallest 按面积比较，并列按物品 ID 字典序选首个。同一张 reveal 卡的全部字段来自同一件物品。random 在所有物品中等概率选一件，跨次使用允许重复命中。

每张卡仍单局一次、每轮报价前最多一张、每人最多携带五张。这些是游戏规则，不由卡牌配置改变。

配置文件最多 1 MiB，未知字段、未知效果或字段、不合法参数、重复 ID、无效默认卡组及尾随 JSON 会使宿主启动失败；不会回退到默认配置或启动半可用服务。配置加载后复制到房间，外部修改原始数据不会改变局内规则。

新增卡牌、组合揭示字段、调整名称与卡组都只需改配置。新增一种不属于上述原语的机制需要扩展引擎和校验；配置不执行脚本或 Go 代码。
