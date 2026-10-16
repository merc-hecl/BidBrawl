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

stats 可用 `-sort profit / rate / average / max_profit / max_loss` 选择累计盈亏、盈亏率、平均每局盈亏、单局最大盈利或单局最大亏损排序。盈亏率 = 累计盈亏 ÷ 实际成交投入；投入为零时不计算。平均值包含所有参赛局，未成交局盈亏为 0。

`me.stats` 与 WebUI 玩家详情使用相同字段和口径：

| 字段 | 含义 |
|---|---|
| `games` | 已结算参赛局数，含未拿箱的对局 |
| `profit` | 累计盈亏 |
| `paid` | 实际累计成交投入 |
| `rate` | 盈亏率，百分数；投入为 0 时为 `null` |
| `average` | 平均每局盈亏 |
| `max_profit` | 单局最大盈利，没有盈利则为 0 |
| `max_loss` | 单局最大亏损，负数；没有亏损则为 0 |

`leaderboard -sort profit / rate / average / max_profit / max_loss` 切换排序，默认按累计盈亏降序。返回 `sort`、全局已结算局数 `games` 和 `rankings` 列表，每项有 `rank`（从 1 开始的序号）、玩家 ID、名称和上述个人指标。仅已参加结算对局的玩家上榜；最大亏损按数值升序（亏损最多在前），其余指标降序，空盈亏率排在最后，相同指标按永久账号 ID 排序。排行榜不包含玩家资产、凭据、私有线索或决策理由。

ACCOUNT_ID 使用永久账号 ID。公开复盘只包含已公开信息；自己的历史理由从 state 的 `self.decisions` 查询。

归档房间仍可回看；删除房间后，state 和 replay 返回 404，房间不再出现在 rooms 中。已结算对局的公开经济摘要仍保留在 results 中，并继续计入 stats 与 agent 成绩。

[返回目录](../HANDBOOK.md)：`bidbrawl handbook`。
