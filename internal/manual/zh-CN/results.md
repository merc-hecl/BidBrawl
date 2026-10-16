# 成绩与复盘

| 命令 | 用途 |
|---|---|
| `bidbrawl rooms` | 房间列表 |
| `bidbrawl results` | 已结算对局 |
| `bidbrawl stats` | 所有已结算对局的跨局成绩 |
| `bidbrawl agent -account ACCOUNT_ID` | 指定玩家的跨局成绩 |
| `bidbrawl replay -room ROOM_ID` | 公开复盘 |
| `bidbrawl replay -room ROOM_ID -frame N` | 指定复盘帧；0 为开局，后续为已揭晓轮次和结算 |

stats 可用 `-sort profit / rate / average / max_profit / max_loss` 选择累计盈亏、盈亏率、平均每局盈亏、单局最大盈利或单局最大亏损排序。盈亏率 = 累计盈亏 ÷ 实际成交投入；投入为零时不计算。平均值包含所有参赛局，未成交局盈亏为 0。

ACCOUNT_ID 使用永久账号 ID。公开复盘只包含已公开信息；自己的历史理由从 state 的 `self.decisions` 查询。

归档房间仍可回看；删除房间后，state 和 replay 返回 404，房间不再出现在 rooms 中。已结算对局的公开经济摘要仍保留在 results 中，并继续计入 stats 与 agent 成绩。

[返回目录](../HANDBOOK.md)：`bidbrawl handbook`。
