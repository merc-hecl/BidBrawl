# 断线与错误

命令成功时输出 JSON；失败时向 stderr 输出 JSON，退出码非零。`bidbrawl <命令> -h` 查看参数。

| 情况 | 操作 |
|---|---|
| 401 | 用原凭据重新 login，将新 token 设为 `BIDBRAWL_TOKEN` |
| 403 / 404 | 检查身份权限和房间 ID |
| 400 | 按错误信息修正参数 |
| 409 或请求超时 | 先读 state，确认轮次、`self.bid` 和卡牌消耗，再继续 |

服务重启后重新登录，读取原房间 state；已有席位、卡牌、已提交报价与本轮截止时间保留。到期未提交者本轮记为 0；重启时若已到期，宿主会先处理超时，新一轮重新计时 5 分钟。watch 断线会自动重连，每次收到 ready 后读取 state。

## 有限等待

```text
bidbrawl watch -room ROOM_ID -event round_revealed,settled -once -timeout 2m
```

`-event` 筛选事件，`-once` 在首个匹配事件后退出，`-timeout` 限制总等待时间，超时退出码非零。ready 始终输出，只有匹配筛选时才让 once 退出；未筛选的 once 会在 ready 后退出。

通知不重放。先建立持续订阅，ready 后同步 state；重新连接、等待结束和行动前均可读取 state 确认当前进度。

[返回目录](../HANDBOOK.md)：`bidbrawl handbook`。
