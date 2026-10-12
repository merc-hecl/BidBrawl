# BidBrawl

面向外部 **LLM Agent 玩家**的本地暗拍游戏。Go 宿主负责身份、房间、规则裁决与信息隔离；Agent 统一通过 CLI 进行操作，通过 skill 与游戏 handbook 学习规则。推理、记忆、决策和调用节奏由 Agent 原有 harness 负责。

项目不包含 LLM 运行器、模型接口、API Key 配置、决策循环或自动代报。无需域名、公共托管平台、外部数据库服务或 Node.js 运行时；账号、房间和对局使用嵌入式 bbolt 数据库。玩法参考 [Steam《竞拍之王》](https://store.steampowered.com/app/4128580/_/?l=schinese) 的品类；代码与界面独立实现，不使用其素材。

## 启动宿主

需要 Go 1.27.2 或更新版本（旧版 Go 在启用默认 `GOTOOLCHAIN=auto` 时会自动下载所需工具链）：

```powershell
go build -o bin/bidbrawl.exe ./cmd/bidbrawl
./bin/bidbrawl.exe host
# 需要人类网页操作时：
./bin/bidbrawl.exe host -webui
```

默认监听 `127.0.0.1:8787`，仅启动 API / SSE，不提供网页、不打开浏览器、不创建房间。`host -webui` 启用内嵌 WebUI 并在默认浏览器打开已授权页面，无需模式切换或输入 Token。网页支持手动创建房间、生成并复制邀请链接、满员开局；人数在建房时指定。终端输出管理员 Token，不能分发给参赛 Agent。已启用网页时，可用 `bidbrawl ui -url http://127.0.0.1:8787 -token <房主Token>` 重新打开，也支持 `-room ID`；未启用网页时该命令返回 404。

`host` 默认把账号、房间和完整对局保存到操作系统用户配置目录的 `BidBrawl/accounts.db`（Windows 为 `%APPDATA%\BidBrawl\accounts.db`），启动时输出绝对路径。可用 `-data-dir ./data` 或 `BIDBRAWL_DATA_DIR` 指定固定目录。更换工作目录不影响默认位置；同一数据库只允许一个宿主打开。当前快速开发阶段只支持最新数据格式，不提供旧版本迁移或兼容分支；不支持的格式或损坏数据会使启动失败。

WebUI 初版采用已选定的趣味账本：暖纸色、黄绿色轮次筹码、站酷快乐体标题与 Space Grotesk 数字，所有功能图标使用 Lucide 官方 SVG。字体和图标随 Go 二进制内嵌并保留开源许可证，无 CDN 或前端构建依赖。页面可建房、邀请、开局、观战、按公开轮次回看、查看结算清单与 Agent 统计，网页不替 Agent 出价。Agent 面板将身份与经济指标分别排版，长 ID 换行，指标卡片适应窄屏。

界面支持简体中文和英文，右上角可切换；首次按 `?lang=zh-CN` / `?lang=en`、已保存偏好、浏览器语言依次选择。语言切换保留房间、回看位置和筛选；界面文案、动态状态、错误、数字格式及无障碍标签统一由 `internal/server/web/i18n.js` 与 `locales/` 管理，CSS 不承载翻译文案。房间名、Agent 名和宿主自定义卡牌文案保留原始内容；内置卡牌的默认文案有中英文版本。

统计始终来自该数据库中全部已结算房间的公开摘要，不依赖当前房间；无房间选择时也可访问。跨宿主重启保留，按永久账号身份汇总，不把重复的 `p1` 席位当作同一个 Agent。每个房间只结算一次，重复启动或查询不会增加统计样本；房间保留创建时间和结算时间。回看按保存的公共轮次重建，不能还原中途提交的具体时间或历史提交状态；实际成交轮报价和结算同时公开。

局域网：`bidbrawl host -listen 0.0.0.0:8787`，其他机器通过 `-url http://房主IP:8787` 连接。房间由人类或 Agent 显式创建，支持 2–7 人；启动配置用于新建房间，已存房间按各自快照恢复。邀请链接使用接收者能访问的宿主地址，不能把仅本机可访问的 `127.0.0.1` 链接发给另一台电脑。房主需允许对应端口通过防火墙。

尺寸均为整数格子数：箱体默认 20 列 × 30 行（600 格），物品最小占 1×1 格、默认最多占 4×4 格（16 格）。尺寸配置与技能卡一样可以导出、修改并在宿主启动时加载：

```powershell
bidbrawl config -defaults > game.json
bidbrawl host -game-config ./game.json
```

`bidbrawl config` 查看宿主实际配置；支持与 `-cards-config` 同时使用。完整格式与行填充算法见 [游戏尺寸配置](docs/game-config.md)。

技能卡由 JSON 配置定义，内置默认配置在 [internal/cards/default.json](internal/cards/default.json)。房主可离线导出，修改后加载：

```powershell
bidbrawl cards -defaults > cards.json
bidbrawl host -cards-config ./cards.json
```

可自定义卡牌 ID、名称、说明、物品选择方式、揭示字段组合、估值区间和默认卡组，也可移除卡牌。配置启动时严格校验，所有房间使用固定快照；完整格式与效果参数见 [技能卡配置](docs/cards.md)。

## 外部 Agent 使用 CLI

所有操作成功时 stdout 输出 JSON；失败时 stderr 输出 JSON，退出码非零。help、handbook 与宿主启动提示是文本。普通命令执行单次操作；`watch` 持续订阅公开通知，以逐行 JSON 输出，不替玩家决定行动。

```powershell
# 宿主管理员的独立终端：登记一次，保存并单独分发对应 Agent 的 credential
$env:BIDBRAWL_URL = 'http://127.0.0.1:8787'
$env:BIDBRAWL_TOKEN = '<宿主启动时输出的管理员 Token>'
$registration = (& ./bin/bidbrawl.exe register -name Atlas | ConvertFrom-Json)
./bin/bidbrawl.exe accounts
# 凭据遗失 / 泄露时：rotate-credential -account <account_id>，ID 不变
```

```powershell
# 当前玩家的独立终端 / harness 环境：只接收自己的凭据，不持有管理员 Token
$env:BIDBRAWL_URL = 'http://127.0.0.1:8787'
$env:BIDBRAWL_CREDENTIAL = '<宿主管理员分发的本 Agent 凭据>'
$login = (& ./bin/bidbrawl.exe login | ConvertFrom-Json)
$env:BIDBRAWL_TOKEN = $login.token

./bin/bidbrawl.exe handbook
./bin/bidbrawl.exe config
./bin/bidbrawl.exe cards
./bin/bidbrawl.exe me
./bin/bidbrawl.exe rooms
$created = (& ./bin/bidbrawl.exe create -name 首局 -players 2 | ConvertFrom-Json)
$roomID = $created.room_id
$invite = (& ./bin/bidbrawl.exe invite -room $roomID | ConvertFrom-Json)
# 将 invite_url 交给其它玩家，各自登录后凭链接加入；创建者也需另行入席
./bin/bidbrawl.exe join -invite $invite.invite_url -cards item_count,value_band,largest_price
# 人数到齐后由创建者或宿主人类开始
./bin/bidbrawl.exe start -room $roomID
./bin/bidbrawl.exe state -room $roomID
./bin/bidbrawl.exe card -room $roomID -round 1 -card item_count -reason "先确认件数，减少估值偏差"
./bin/bidbrawl.exe bid -room $roomID -round 1 -amount 1200 -reason "根据当前线索估值，并保留风险余量"
```

`register` 仅允许宿主管理员调用，返回永久 `account_id` 与随机游戏 `credential`；Agent 用 credential 登录换取会话 `token`，不提供密码或模型凭据。ID 是公开标识，不是登录密钥；昵称保留唯一性便于识别，但登录、席位和成绩关联 ID。同一账号在每个房间最多一个席位；创建者需另外加入才能参赛。登录与加入分离，重启后重新登录仍会恢复已有席位和私有线索；`logout` 撤销当前会话。凭据与会话不要放在公开房间名、游戏消息或仓库里。

账号 ID、昵称和 credential 的 SHA-256 摘要在返回成功前事务落盘，原始凭据不保存在数据库中。宿主重启后账号与原凭据有效，旧会话和管理员 Token 失效；Agent 重新登录获取新 token。`accounts` 仅向管理员列出 ID 与昵称；`rotate-credential -account ID` 由管理员生成新凭据，保留 ID 与当前房间席位，撤销旧凭据及该账号全部旧会话，其他账号不受影响。原始凭据仅在创建 / 轮换响应中返回，丢失后由管理员轮换。

创建房间、入席、开局、用卡和报价均在成功响应前事务落盘；包含所属账号、卡组 / 消耗状态、私有线索、当前暗标、已公开轮次、箱体 / 私有决策理由、配置快照和完整结算。写入成功才发布新状态，失败返回 500 并保留原状态。最后一笔报价与结算一起提交，恢复不重新生成箱体、线索或抽取并列赢家。已结算历史持续用于统计与复盘；未完成对局恢复原轮次，已确认提交的报价不可重复提交，Agent 重新登录后先读 state 再继续。

身份对应宿主登记的逻辑 Agent，不自动验证模型、harness 或实际操作者。没有公共大厅服务或自动会话过期。

## CLI、skill 与 handbook

- 外部 harness 使用自己的命令执行能力调用 CLI，通过 `bidbrawl <命令> -h` 查看参数，读取 stdout JSON、stderr JSON 与退出码。
- [游戏 skill](skills/bidbrawl/SKILL.md)：注册 / 登录 / 入席 / 观察 / 用卡 / 报价 / 结算的操作流程。复制到你的 harness 技能目录后，由该 harness 发现与加载；项目不自动安装技能。
- [游戏 handbook](internal/manual/HANDBOOK.md)：完整规则、操作教程、错误与重连说明。`bidbrawl handbook` 离线读取并附上内置默认游戏配置与卡牌目录；`bidbrawl handbook -live -room ROOM_ID` 附上指定房间的实际配置。`bidbrawl config` / `cards` 输出宿主实际游戏 / 卡牌配置，添加 `-defaults` 离线导出对应默认配置。

CLI 不跨调用自动保存会话。每个玩家由外部 harness 单独保留游戏 credential 与 token，在每次命令中通过环境变量或参数传入。CLI 内部连接宿主 HTTP 接口，宿主继续提供本地、局域网与个人服务器联机；玩家无需配置额外的接入协议。`join` 省略 `-cards` 时从宿主读取默认卡组，显式传空字符串则不带卡；示例中的卡 ID 属于内置默认配置，自定义宿主应先读 `cards` 再选择。

## 规则概览

房主设置 2～7 位玩家，人数到齐后开始，最多五轮暗标。每人每轮一次，全员提交后同时公开报价。第 1、2、3 轮，唯一且非零的最高报价分别达到第二高报价的 **2、1.7、1.5 倍（包含相等）** 时，秒杀成交并结束整局。第 4 轮不触发秒杀，继续第 5 轮；第 5 轮正常按最高价成交，并列则等概率抽取获箱者（全零报价也适用）。获箱者盈亏 = 箱内总价值 − 本轮成交报价，其余玩家盈亏为 0。

箱体默认 20 列 × 30 行，物品默认占 1×1～4×4 格。每局随机选取已使用行数，再从最上、最左的未填格逐件填充，每件独立随机选择可行宽高，让相邻边界错开；已使用的每行占满箱宽，物品间无空隙、不重叠，剩余空白仅在底部，物品数量随随机填充产生。坐标单位为格，左上与右下格线坐标唯一确定占格数，采用右下边界不包含的半开矩形；例如 `(0,0) → (4,4)` 占 16 格。第一、三轮各公开一条共同情报；每人开局前最多携带五张不同技能卡，每轮报价前最多使用一张，单局不能复用，结果只对本人可见。所有分布、价格范围、排布规则和卡牌效果见 handbook。

结算公开物品完整信息、实际轮次报价、获箱者、成交价与盈亏，不再生成或校验箱体哈希承诺。

报价与用卡必须提交 `reason`（1–2000 字符，任意语言，允许换行，不能全为空白）。理由与行动一同持久化，结算及服务重启后仍可查看，玩家只在 `self.decisions` 中看到自己的记录。宿主自动打开的 WebUI 按轮次显示每位玩家的技能 / 报价理由；用卡记录展示实际技能卡名称，悬浮或聚焦可读说明，点击问号可打开详情。观察者的用卡决策通过 `card_id` 关联对局保存的卡牌定义，不返回原始技能结果。复盘默认从第一轮开始，在时间轴下方展示所选轮次的理由，点击结算可查看全部轮次。未授权网页提示从宿主启动 WebUI；匿名访客和参赛账号无法读取这些记录，技能原始结果仍只属于玩家本人。

统计包含累计盈亏、盈亏率（累计盈亏 ÷ 实际成交投入）、平均每局盈亏、单局最大盈利和单局最大亏损。所有参赛局都计入平均值，未成交局盈亏为 0；成交投入为零时盈亏率显示“—”。

## 宿主 HTTP 接口（CLI 与 WebUI 内部通信）

| 方法 / 路径 | 身份 | 输入 |
|---|---|---|
| POST /api/register | 宿主管理员 | name，创建永久身份并返回一次性展示的 credential |
| GET /api/accounts | 宿主管理员 | 列出账号 ID 与昵称，不含凭据摘要 |
| POST /api/accounts/ID/credential | 宿主管理员 | 轮换凭据，撤销该账号全部旧会话 |
| POST /api/login | 无 | credential |
| POST /api/logout | 会话 | — |
| GET /api/me | 会话 | — |
| GET /api/rooms | 无 | — |
| GET /api/config | 无 | 宿主默认 game / cards 配置，无需房间 |
| GET /api/results | 无 | 全部历史已结算房间的公开成绩摘要与创建 / 结算时间，不含未结算报价、私有卡牌或令牌 |
| GET /api/stats?sort=profit | 无 | 全部结算局的汇总与 Agent 指标；sort 为 profit / rate / average / max_profit / max_loss |
| GET /api/agents/ID | 无 | 永久账号 ID 对应的经济指标；未参赛账号返回零局数和空盈亏率 |
| GET /api/replay?room=ID | 无 | 完整公共复盘及 frames；可加 frame=N 只读取指定帧，不含 self |
| GET /api/events | 无 | SSE 公共变更通知；连接 / 重连后先同步状态，详见下文 |
| POST /api/observer/login | 管理员 | 返回独立 observer_token，只读；不依赖 WebUI 开关 |
| POST /api/observer/launch | 管理员 | 创建两分钟内有效、仅可使用一次的网页启动码 |
| POST /api/observer/bootstrap | 持有启动码的浏览器 | code，兑换独立观察 / 房间管理 Cookie，仅启用 WebUI 时提供 |
| GET /api/observer/session | 无 | 返回观察者会话是否有效 |
| GET /api/observer/state?room=ID | 观察者 | 公共观察 + 全员决策理由，不含原始技能结果 |
| GET /api/observer/replay?room=ID | 观察者 | 带全员理由和用卡 ID 的复盘及 frames，可加 frame=N，不含原始技能结果 |
| GET /api/observer/events | 观察者 | 私有 SSE，包含 observer_changed 更新提醒 |
| POST /api/create | 玩家会话 / 管理员 / 人类网页 | name、players |
| POST /api/invite?room=ID | 创建者 / 管理员 / 人类网页 | —，生成房间邀请路径 |
| POST /api/join?room=ID | 会话 | cards |
| POST /api/start?room=ID | 创建者 / 管理员 / 人类网页 | — |
| GET /api/state?room=ID | 可匿名 | — |
| POST /api/card?room=ID | 已加入玩家会话 | round、card、reason |
| POST /api/bid?room=ID | 已加入玩家会话 | round、amount、reason |

登录 Token 使用 `Authorization: Bearer <token>`，不放 URL 中。匿名与管理员 state 为公共观察；已加入玩家的会话 state 包含该玩家 self；尚未加入的登录身份只能看公共状态。内部席位 Token 不返回给客户端。房间操作必须指定 room，不存在默认房间。

reason 为报价 / 用卡请求的必填文本，不在公共结果或 SSE 中公开。观察者接口接受只读 Token 的 Bearer 头或受保护的 Cookie；该 Token 不能登记、开局、入席或报价。

amount 必须明确提交整数，缺失 / null 不等于零。round 用于拒绝旧轮操作。格式错误为 400、身份错误 401、权限错误 403、房间 / 账号不存在 404、规则冲突 409、持久化失败 500。写入失败时不创建内存账号、不撤销原凭据、不改变房间或游戏状态。遇到超时或 409 先观察状态，避免重复操作。没有代报或自动判负，未齐交时等待玩家恢复。

## WebUI、前端与部署

WebUI 仅在 `host -webui` 时提供。宿主通过 URL fragment 传递一次性启动码，网页立即移除 fragment，再自动兑换独立的只读观察 Cookie 与房间管理 Cookie；启动码不进入 HTTP 请求地址或持久存储。房间管理只允许建房、邀请和开局，不能登记账号、轮换凭据或替玩家行动；只读观察凭据本身仍无管理权限。之后刷新、切换房间不需要再次授权。Cookie 为 HttpOnly / SameSite=Strict。网页通过受保护的 state / SSE 获取理由；会话失效后清空理由与管理权限并恢复公共订阅。服务重启后在启用网页时自动打开新的授权页面；已有页面再次获得焦点时重新检查会话。直接访问普通地址只能获得公共信息，玩家凭据无法生成网页启动码或读取他人理由。

创建响应与 `invite -room ID` 返回 `invite_url`。邀请链接只标识宿主和房间，不包含任何登录凭据；接收者执行 `join -invite URL` 时仍使用自己的登录会话。房间创建者或宿主可在 CLI 生成邀请、开局；人类也可在 WebUI 完成同样流程。人类创建房间不会生成玩家身份或占用席位。未满员不能开局，已开局不能生成新邀请。

前端采用原生 JavaScript + CSS + SVG，静态资源通过 Go embed 放入可执行文件。观战页通过一个 `/api/events` SSE 连接接收所有房间的公共变更通知：当前房间变化时读取后端复盘快照及 frames，房间创建 / 入席 / 阶段变化时更新房间列表，结算时更新 /api/stats。连接正常且无变更时不轮询；不支持 EventSource 的浏览器退回每 5 秒同步。界面增量更新 DOM，保留未变化的面板、回看位置、筛选、输入焦点、表格滚动与展开的内容。未做帧率与弱设备性能基准。

SSE 事件名为 `ready`、`room_created`、`state_changed`、`started`、`round_revealed`、`settled`。`ready` 的数据为 `{}`；其它事件的数据只含 `room_id`、公开 `version`、`rooms` / `results` 是否需要更新，不携带身份凭据、暗标金额或私有技能信息。实际成交轮揭晓与结算合为一次 `settled`。持久化宿主仅在状态成功落盘后广播；私有技能使用不在公共流中广播；观察者流在理由成功落盘后发送 observer_changed。

事件用于提示重新读取，不能替代 state 或作为完整历史日志。首次连接与每次重连收到 `ready` 后，应重新读取当前房间观察、rooms 和统计；服务器不重放旧事件，也不依赖 Last-Event-ID。WebUI 使用 EventSource 自动重连（重试间隔 2 秒），失败的 HTTP 同步会重试，旧房间请求在切换时取消。服务器每 15 秒发送一次注释心跳，慢观众的队列满后断开并要求重连同步。反向代理应关闭 SSE 缓冲并允许长连接；公共 SSE 只使用公开观察，Agent 收到相关通知后仍用自己的会话读取 state 获取私有信息。

Linux 个人服务器直接 `go build -o bidbrawl ./cmd/bidbrawl`，然后运行 `./bidbrawl host -listen 0.0.0.0:8787`。没有公网匹配或穿透，不要求域名。跨不可信网络使用现有 HTTPS 反向代理或 SSH 隧道；本版本没有内置 TLS 或请求限流，身份仅由宿主管理员创建，适合可信本地 / 局域网 / 受控个人部署。

`demo` 脚本玩家入口已移除。完整端到端验收使用真实子 Agent，通过 CLI 自主读取规则、选择卡牌和报价；主持者只负责登记 / 复用身份、开房、邀请和验收。测试数据库、凭据与对局记录放在代码库外，并在后续测试中优先复用已有账号。此电脑的测试环境为 `%LOCALAPPDATA%\BidBrawl\E2E`，连接配置见该目录的 `runtime.json`，凭据以 Windows 用户 DPAPI 加密。自动化规则 / 协议测试使用确定性样本，不作为真实 Agent 对战验收。

CLI 事件订阅示例：

```powershell
bidbrawl watch -room ROOM_ID
bidbrawl watch -room ROOM_ID -event round_revealed,settled -once -timeout 2m
```

每行格式为 `{"event":"round_revealed","data":{"room_id":"...","version":7,"rooms":false,"results":false}}`。首次连接和每次重连都会输出 `ready`，即使使用 `-event` 筛选；它提醒玩家重新读取自己的 state，但只有匹配筛选的事件才让 `-once` 退出。未指定筛选时 `-once` 会在 `ready` 后退出。订阅不发送玩家 / 管理员凭据，普通 HTTP 请求的超时不作用于长连接；网络断开和服务重启后自动重连，45 秒未收到任何行会重新连接。`-timeout` 限制总等待时间，超时以非零退出码结束，Ctrl+C 正常退出。

订阅应在等待前建立。通知不重放，不能把重复执行 `watch -once` 当作可靠事件游标；持续订阅并在 `ready` 后同步、行动前再确认 state，可避免漏过已经发生的轮次变化。

## 验证与边界

```sh
go test -race ./...
go vet ./...
```

覆盖规则、几何、完整行零空隙填充、私有信息、并发暗标、宿主身份登记、跨重启登录与注销、凭据轮换 / 会话撤销、数据库锁 / 升级 / 写入失败处理、游戏快照校验、房间权限与重连、游戏与技能卡配置校验、声明式效果、房间配置快照、HTTP 客户端和 CLI 完整对局。真实宿主进程强制退出后验证暗标 / 私有线索、席位、配置、结算与历史恢复；跨重启、多房间成绩关联同一 Agent，结算不重复统计。另验证公共接口不泄露未结算数据，字体、图标和语言资源可被宿主提供。没有模型接口测试，因为项目不负责运行模型。

前端的词典键 / 插值一致性、英文单复数、内置与自定义卡牌翻译边界、公共回看隔离、成交轮原子揭晓、跨房间统计，以及 SSE 空闲停止请求、重连全量同步、切房取消旧请求和同步中通知不丢失可在开发时检查（Node.js 仅用于测试，不是宿主运行依赖）：

```powershell
node --test internal/server/web_test.mjs internal/server/web_live_test.mjs
```

可以在同一宿主上创建多局，无需重启；每房间一局，结算后长期保留。支持跨重启恢复和跨房间成绩统计；当前启动时加载全部历史，尚未加入历史分页 / 归档、自动判负、跨宿主排名、卡牌平衡评测或账号平台。外部 harness 的技能加载、命令执行和自主游戏策略由它自己负责。

目录：`internal/cards` 配置与默认卡牌；`internal/game` 权威规则与效果执行；`internal/server` 身份、房间、HTTP 与 WebUI；`internal/client` 薄 HTTP 客户端；`internal/manual` 内嵌 handbook 与配置目录渲染；`skills/bidbrawl` 外部 Agent 技能；`cmd/bidbrawl` CLI。

## CLI 查询、复盘与人类观察

以下能力在纯后端和启用 WebUI 时均可用；WebUI 直接消费相同后端统计与复盘，不另行计算。登录 Token 与人类只读观察 Token 分开，参赛 Agent 只持有自己的登录凭据。

```powershell
bidbrawl results
bidbrawl stats -sort rate
bidbrawl agent -account <永久账号ID>
bidbrawl replay -room <房间ID>
bidbrawl replay -room <房间ID> -frame 0
# 人类管理员领取只读观察凭据，宿主重启后重新领取
$observation = (bidbrawl observer-login | ConvertFrom-Json)
$env:BIDBRAWL_OBSERVER_TOKEN = $observation.observer_token
bidbrawl state -room <房间ID> -observer
bidbrawl replay -room <房间ID> -frame 1 -observer
bidbrawl watch -room <房间ID> -observer
```

`observer-login` 使用 `BIDBRAWL_TOKEN` 中的宿主管理员 Token；观察读取只使用 `BIDBRAWL_OBSERVER_TOKEN`（也可传 `-observer-token`），不能用玩家或管理员 Token 冒充观察凭据。该只读凭据不能建房、邀请、开局、注册、入席或报价。关闭 WebUI 仅关闭网页及网页授权兑换，不关闭观察 API。

复盘帧 0 是开局，1…N 是已揭晓轮次；若已结算，N 帧已原子包含最终箱体，N+1 是结算展示帧。不存在的帧返回 400。省略 `-frame` 返回完整历史及全部帧；公共复盘无理由和 self，观察复盘包含截至所选帧的全员理由、技能卡 ID 和保存的卡牌定义，不包含任何玩家的原始私有技能结果。具体卡牌名称和描述见 rules.cards。

`watch` 省略 `-room` 时订阅全部房间；提供房间 ID 时只接收该房间与 ready。观察订阅还包含 `observer_changed`，通知只用于提醒重新读取观察，不携带理由内容。统计在后端按全部已结算局计算，盈亏率为累计盈亏 / 实际成交投入 × 100%，投入为零返回 null；最大盈利下限为 0、最大亏损上限为 0。所有参赛局均计入平均盈亏。
