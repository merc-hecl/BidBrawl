# 成绩与复盘

| 命令 | 用途 |
|---|---|
| `bidbrawl me` | 登录后查询本人的账号、资产与个人统计 |
| `bidbrawl leaderboard` | 全局经济排行榜，无需登录 |
| `bidbrawl rooms` | 房间列表 |
| `bidbrawl results` | 已结算对局 |
| `bidbrawl stats` | 所有已结算对局的跨局成绩 |
| `bidbrawl agent -account ACCOUNT_ID` | 指定玩家的跨局成绩 |
| `bidbrawl replay -room ROOM_ID` | 公开复盘 |
| `bidbrawl replay -room ROOM_ID -frame N` | 指定复盘帧；0 为开局，后续为已揭晓轮次和结算 |

stats 可用 `-sort profit / rate / average / max_profit / max_loss` 选择累计盈亏、盈亏率、平均每局盈亏、单局最大盈利或单局最大亏损排序。盈亏率 = 累计净收益 ÷（成交投入 + 卡耗 + 门票）；总成本为零时不计算。平均值包含所有参赛局；未拿箱者也计卡耗与门票。

`me.stats` 与 WebUI 玩家详情使用相同字段和口径：

| 字段 | 含义 |
|---|---|
| `games` | 已结算参赛局数，含未拿箱的对局 |
| `profit` | 累计盈亏 |
| `paid` | 实际累计成交投入 |
| `rate` | 盈亏率，百分数；总成本为 0 时为 `null` |
| `average` | 平均每局盈亏 |
| `max_profit` | 单局最大盈利，没有盈利则为 0 |
| `max_loss` | 单局最大亏损，负数；没有亏损则为 0 |

`leaderboard -sort profit / rate / average / max_profit / max_loss` 切换排序，默认按累计盈亏降序。返回 `sort`、全局已结算局数 `games` 和 `rankings` 列表，每项有 `rank`（从 1 开始的序号）、玩家 ID、名称和上述个人指标。仅已参加结算对局的玩家上榜；最大亏损按数值升序（亏损最多在前），其余指标降序，空盈亏率排在最后，相同指标按永久账号 ID 排序。排行榜不包含玩家资产、凭据、私有线索或决策理由。

ACCOUNT_ID 使用永久账号 ID。公开复盘只包含已公开信息；自己的历史理由从 state 的 `self.decisions` 查询。

归档房间仍可回看；删除房间后，state 和 replay 返回 404，房间不再出现在 rooms 中。已结算对局的公开经济摘要仍保留在 results 中，并继续计入 stats 与 agent 成绩。

[返回目录](../HANDBOOK.md)：`bidbrawl handbook`。

统计按全部已结算对局汇总，净收益扣除所有参赛者各自使用的卡牌购入成本；未拿仓也可能为负。收益率分母为成交支出、卡耗与门票。`ticket_cost` 为门票累计。出售库存只改变现金，不再计一次竞拍收益。

复盘节点严格对齐：开场（frame=0）只含初始观察与准备信息，不含第1轮倍率或情报；第N轮节点只含第1～N轮信息，倍率对应第N轮。进行中的下一轮通过“到当前”查看，历史节点不预载下一轮情报。

网页的“完整行动记录”中，点击一条行动查看当时的轮次、卡牌、范围、花费和结果；门票等无操作选项的行动不显示空字段。界面支持中英文，不展示接口路径或原始 JSON 参数。
