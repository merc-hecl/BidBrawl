# 宿主与开发指南

[English overview](../README.md) · [中文项目介绍](../README.zh-CN.md)

本指南包含安装、宿主部署、配置、接口、事件订阅与开发验证。玩法和玩家操作见[游玩手册](../internal/manual/HANDBOOK.md)。

## 桌面客户端

桌面版使用 MyGo，内置完整 WebUI 和 Go 宿主。打开 BidBrawl 即可建房、邀请、开局、观战、复盘和查看统计；无需终端或输入 Token。CLI 与桌面版使用相同的游戏服务和数据库，参赛 Agent 仍使用自己的身份登录，桌面权限不会注入玩家命令。

- Windows：运行 `BidBrawl Setup … .exe`，按当前用户安装桌面程序、快捷方式和 `bidbrawl` 命令。安装后重开终端应用；卸载只删除程序及安装包添加的命令注册，保留账号、凭据和对局。
- macOS：使用 `.pkg` 同时安装 `/Applications/BidBrawl.app` 与 `/usr/local/bin/bidbrawl`；单独拖入 `.app` 或使用 DMG 只安装桌面程序。当前发布包没有 Developer ID 签名和公证，Gatekeeper 可能阻止安装。
- Linux：使用 `.deb` 安装桌面程序、依赖和 `/usr/bin/bidbrawl`。桌面依赖 GTK 3、WebKitGTK 4.1；托盘使用 AppIndicator。

默认监听 `127.0.0.1:8787`，不自动建房。桌面菜单可重新连接、打开数据目录及修改宿主设置（监听地址、数据目录和自定义配置文件）；保存设置会确认后重启宿主。Windows / Linux 按 Alt 或 F10 打开桌面菜单。局域网连接可在设置中改为 `0.0.0.0:8787`；其他电脑上的 CLI 仍需指定可达的 `-url`。

有托盘时关闭主窗口会收进托盘，宿主继续运行；重复启动恢复已有窗口。选择“停止宿主并退出”后，程序结束 SSE、等待请求完成并关闭数据库。没有托盘的桌面环境下，关闭最后一个窗口会询问是否停止宿主。桌面设置保存在用户配置目录的 `BidBrawl/desktop.json`；`BIDBRAWL_DESKTOP_CONFIG` 可指定设置文件。运行程序不会安装命令或修改 shell 配置。Windows WebUI 依赖 WebView2 Runtime，Windows 10 如未安装需先安装该运行时。

从源码构建（Go 1.27.2+；不需要 Node.js、Bun 或前端编译）：

```text
go run ./cmd/build-desktop
go run ./cmd/build-desktop -platform windows/amd64,linux/amd64,darwin/arm64
```

成品位于 `build/desktop/<系统>-<架构>/`。构建器使用固定的 MyGo v0.6.2，分别构建桌面程序和 CLI；Windows 首次打包由 MyGo 下载并校验 NSIS。macOS 的 DMG、签名、公证和包含 CLI 的 PKG 需要在 macOS 构建；其他平台可交叉编译 `.app`。当前未配置在线更新源，使用安装包更新；更新或卸载前先从托盘停止桌面宿主。

[Releases](https://github.com/merc-hecl/BidBrawl/releases) 提供 Windows x64、Linux x64/ARM64、macOS Intel/Apple Silicon 安装包和独立 CLI 压缩包，附 SHA-256 校验文件。Windows 安装包尚未配置 Authenticode 签名。发布维护流程见[发布指南](../.github/RELEASING.md)。

仅调试桌面窗口可以运行 `go run ./cmd/bidbrawl-desktop`，支持与宿主相同的 `-listen`、`-data-dir` 和配置文件参数。

## 启动无界面宿主

需要 Go 1.27.2 或更新版本（旧版 Go 在启用默认 `GOTOOLCHAIN=auto` 时会自动下载所需工具链）：

```powershell
go build -o bin/bidbrawl.exe ./cmd/bidbrawl
./bin/bidbrawl.exe host
# 需要人类网页操作时：
./bin/bidbrawl.exe host -webui
```

默认监听 `127.0.0.1:8787`，仅启动 API / SSE，不提供网页、不打开浏览器、不创建房间。`host -webui` 启用内嵌 WebUI 并在默认浏览器打开已授权页面，无需模式切换或输入 Token。网页支持手动创建房间、生成并复制邀请链接、满员开局；人数在建房时指定。终端输出管理员 Token，不能分发给参赛 Agent。已启用网页时，可用 `bidbrawl ui -url http://127.0.0.1:8787 -token <房主Token>` 重新打开，也支持 `-room ID`；未启用网页时该命令返回 404。

`host` 默认把账号、房间和完整对局保存到操作系统用户配置目录的 `BidBrawl/accounts.db`（Windows 为 `%APPDATA%\BidBrawl\accounts.db`），启动时输出绝对路径。可用 `-data-dir ./data` 或 `BIDBRAWL_DATA_DIR` 指定固定目录。更换工作目录不影响默认位置；同一数据库只允许一个宿主打开。

当前数据库 schema 为 5，对局快照 schema 为 12。从正式发布的 v0.1.0 开始支持单向升级：数据库 schema 3/4 在首次打开时先生成同目录 `accounts.db.pre-v5-*.bak` 一致性备份，日志输出路径，再在一个事务中转换有效的 schema 10/11 快照并更新数据库版本。失败回滚，不覆盖备份。账号、凭据、卡牌、已有报价、私有情报、冻结专长与结算结果保留。schema 10 进行中的本轮从升级时起获得完整5分钟；已有 schema 11 的截止时间不重置。旧房间不替换为五种新专长，不重新抽样或发放情报；新房间使用新默认规则。升级不重复扣费或结算。更高版本数据库或快照拒绝加载；发布前实验格式不提供迁移。

单个房间无法恢复时，原始记录仍留在数据库中，列表显示“无法恢复”，读取该房间返回 503，禁止继续操作或删除；其他可恢复房间继续加载。相关账号保留资产锁定；无法可靠识别参与者时保守锁定所有现存账号的资产操作，直到记录修复后重启。新注册账号不会被视为旧房间参与者。`inventory.recovery_blocked` 表明本人是否受影响。统计、成绩列表、排行榜和账号指标通过 `unavailable_rooms` 标记缺失房间数量，WebUI 提示统计可能不完整。数据库结构、账号或已保留成绩数据损坏仍会阻止启动。

启动日志、相关错误和 `host -h` 参数说明使用英文，主要输出为 `HTTP`、`Host token` 和 `Identity database`。

启动时报数据库被占用 / `timeout`，表示其他进程尚未释放该文件锁。使用已运行的宿主，或先在原宿主终端按 Ctrl+C 停止后再启动；更换 `-listen` 端口不能解决同一数据库的占用。不要删除 `accounts.db`，其中保存着账号和对局。

报数据库格式不支持时，先核对错误中的实际文件路径、版本，以及 `-data-dir`、`BIDBRAWL_DATA_DIR` 与默认目录；不应只修改版本号绕过校验。房间恢复错误会在日志中列出房间 ID 和原因。修复或恢复备份前必须停止宿主并保留当前数据库；备份对应升级前的状态，恢复后应使用匹配的程序版本，不应覆盖仍在运行的数据库。

## 配置与二次开发

WebUI 的「游戏手册」提供物品图鉴、技能卡效果、系统提示示例、专长的逐轮说明及特色物品箱介绍，支持中英文、浅深色与窄屏查阅。从大厅进入时使用宿主配置；从房间菜单进入时使用该房间保存的规则，并可直接返回房间。手册只展示公开目录和说明，不展示任何玩家的私有结果。

开发者直接修改四份独立的 JSON 配置，通过启动参数加载，无需规则包或安装步骤：

| 配置 | 可调整内容 | 格式与示例 |
|---|---|---|
| `game.json` | 箱体与物品尺寸、五档品质的名称 / 价格 / 权重、新账号初始资产 | [游戏配置](game-config.md) |
| `cards.json` | 技能卡 ID、名称、说明、已有技能效果的组合和预设卡组 | [技能卡配置](cards.md) |
| `hints.json` | 公共提示池，选择内置类型或调用外部提示程序 | [公共提示插件](hints.md) |
| `items.json` | 物品模板的名称、分类、尺寸、品质和价格位置 | [物品目录](../internal/game/items.json)、[游戏配置](game-config.md) |

```powershell
bidbrawl config -defaults > game.json
bidbrawl cards -defaults > cards.json
bidbrawl hints -defaults > hints.json
# 修改需要调整的文件后启动：
bidbrawl host -game-config ./game.json -cards-config ./cards.json -hints-config ./hints.json -webui
```

四个参数可以单独使用，省略的部分使用内置默认配置。物品目录可复制 internal/game/items.json 后用 `-items-config ./items.json` 加载；该参数替换物品模板，分类和物品箱类型使用内置定义。文件名和目录可自行选择；相对路径按宿主工作目录解析。配置由 Go 结构体和校验函数检查，未知字段或不合法内容会使启动失败，不会悄悄回退到默认值。修改文件后需重启；新建房间使用新配置，已有房间保留自己的配置快照和已生成提示。

配置只组合现有机制：新增技能效果、品质档位、价格分布或成交规则需要修改 Go；内置公共提示文案也由 Go 生成，自定义提示内容可使用已有外部程序协议。不支持配置热更新、多份配置叠加、规则包安装或依赖管理。外部程序和运行环境由开发者自行维护。

配置格式、内置数值与扩展协议以[游戏配置](game-config.md)、[技能卡配置](cards.md)和[公共提示插件](hints.md)为准。

大厅房间卡片右上角的省略号菜单提供归档和删除，进行中的房间除外；等待中的房间即使已有玩家入席也可操作。归档后从大厅和切换列表收起，仍可回看；恢复前暂停入席、邀请和开局。“已归档”打开独立归档页面，其列表上方的次级“批量管理”入口支持全选、批量恢复和批量删除。删除需确认，会移除房间、邀请链接和完整复盘，已完成对局的经济结算摘要及账号统计永久保留。批量操作先校验全部目标，再统一落盘；任一房间正在进行、无权管理或已不存在时，整批不执行。

## 常用 HTTP 接口（CLI 与 WebUI 内部通信）

| 方法 / 路径 | 身份 | 输入 |
|---|---|---|
| POST /api/register | 无 | name，玩家自助创建永久身份并返回一次性展示的 credential |
| GET /api/accounts | 宿主管理员 | 列出账号 ID 与昵称，不含凭据摘要 |
| POST /api/accounts/ID/credential | 宿主管理员 | 轮换凭据，撤销该账号全部旧会话 |
| POST /api/login | 无 | credential |
| POST /api/logout | 会话 | — |
| GET /api/me | 玩家登录会话 | 仅本人账号、assets 与 stats 个人经济指标；不支持指定其他账号 |
| GET /api/rooms | 无 | — |
| POST /api/rooms/manage | 房主 / 宿主管理员 / 网页管理会话 | action 为 archive / restore / delete，room_ids 为非空、不重复 ID 数组；房主仅可管理自己的房间，进行中房间不可操作 |
| GET /api/config | 无 | 宿主默认 game / cards 配置，无需房间 |
| GET /api/catalog | 无 | 可选 room（房间冻结配置）、size（宽x高，格）、quality（品质 ID / 名称）、name（中英文名称，部分匹配，忽略大小写）、category（品类 ID）；条件取交集，返回 catalog 模板与 count；不筛选时另附品质、品类、专长与物品箱类型，不涉及实际箱内物品 |
| GET /api/results | 无 | 全部历史已结算房间的公开成绩摘要与创建 / 结算时间，不含未结算报价、私有卡牌或令牌 |
| GET /api/stats?sort=profit | 无 | 全部结算局的汇总与 Agent 指标；sort 为 profit / rate / average / max_profit / max_loss |
| GET /api/agents/ID | 无 | 永久账号 ID 对应的经济指标；未参赛账号返回零局数和空盈亏率 |
| GET /api/replay?room=ID | 无 | 完整公共复盘及 frames；可加 frame=N 只读取指定帧，不含 self |
| GET /api/events | 无 | SSE 公共变更通知；连接 / 重连后先同步状态，详见下文 |
| POST /api/observer/login | 管理员 | 返回独立 observer_token，只读；不依赖 WebUI 开关 |
| POST /api/observer/launch | 管理员 | 创建两分钟内有效、仅可使用一次的网页启动码 |
| POST /api/observer/bootstrap | 持有启动码的浏览器 | code，兑换独立观察 / 房间管理 Cookie，仅启用 WebUI 时提供 |
| GET /api/observer/session | 无 | 返回观察者会话是否有效 |
| GET /api/observer/state?room=ID | 观察者 | 公共观察 + 完整箱体 items、后端汇总的箱内总值 total_value、全员决策理由、初始抽检及技能揭示的单件物品属性，不含完整原始技能结果 |
| GET /api/observer/replay?room=ID | 观察者 | 带完整箱体 items、箱内总值 total_value、全员理由、用卡 ID 和玩家已知物品记录的复盘及 frames，可加 frame=N，不含完整原始技能结果 |
| GET /api/observer/events | 观察者 | 私有 SSE，包含 observer_changed 更新提醒 |
| POST /api/create | 玩家会话 / 管理员 / 人类网页 | name、players、可选 ticket_fee；物品箱类型由系统随机选定，不接受 warehouse |
| POST /api/invite?room=ID | 创建者 / 管理员 / 人类网页 | —，生成房间邀请路径 |
| POST /api/join?room=ID | 会话 | cards |
| POST /api/start?room=ID | 创建者 / 管理员 / 人类网页 | — |
| GET /api/state?room=ID | 可匿名 | — |
| POST /api/card?room=ID | 已加入玩家会话 | round、card、reason |
| POST /api/bid?room=ID | 已加入玩家会话 | round、amount、reason |

登录 Token 使用 `Authorization: Bearer <token>`，不放 URL 中。匿名与管理员 state 为公共观察；已加入玩家的会话 state 包含该玩家 self；尚未加入的登录身份只能看公共状态。内部席位 Token 不返回给客户端。房间操作必须指定 room，不存在默认房间。

reason 为报价 / 用卡请求的必填文本，不在公共结果或 SSE 中公开。观察者接口接受只读 Token 的 Bearer 头或受保护的 Cookie；该 Token 不能管理账号、开局、入席或报价。自助注册是公开入口，参赛仍需使用自己的玩家账号登录。

amount 必须明确提交整数，缺失 / null 不等于零。round 用于拒绝旧轮操作。格式错误为 400、身份错误 401、权限错误 403、房间 / 账号不存在 404、规则冲突 409、持久化失败 500。写入失败时不创建内存账号、不撤销原凭据、不改变房间或游戏状态。遇到超时或 409 先观察状态，避免重复操作。没有代报或自动判负，未齐交时等待玩家恢复。

## WebUI、前端与部署

WebUI 仅在 `host -webui` 时提供。宿主通过 URL fragment 传递一次性启动码，网页立即移除 fragment，再自动兑换独立的只读观察 Cookie 与房间管理 Cookie；启动码不进入 HTTP 请求地址或持久存储。房间管理只允许建房、邀请和开局，不能查看账号列表、轮换凭据或替玩家行动；网页不提供玩家注册和登录界面，玩家通过 CLI 自助注册和登录。只读观察凭据本身仍无管理权限。之后刷新、切换房间不需要再次授权。Cookie 为 HttpOnly / SameSite=Strict。网页通过受保护的 state / SSE 获取理由；会话失效后清空理由与管理权限并恢复公共订阅。服务重启后在启用网页时自动打开新的授权页面；已有页面再次获得焦点时重新检查会话。直接访问普通地址只能获得公共信息，玩家凭据无法生成网页启动码或读取他人理由。

创建响应与 `invite -room ID` 返回 `invite_url`。邀请链接只标识宿主和房间，不包含任何登录凭据；接收者执行 `join -invite URL` 时仍使用自己的登录会话。房间创建者或宿主可在 CLI 生成邀请、开局；人类也可在 WebUI 完成同样流程。人类创建房间不会生成玩家身份或占用席位。未满员不能开局，已开局不能生成新邀请。

前端采用原生 JavaScript + CSS + SVG，静态资源通过 Go embed 放入可执行文件。观战页通过一个 `/api/events` SSE 连接接收所有房间的公共变更通知：当前房间变化时读取后端复盘快照及 frames，房间创建 / 入席 / 阶段变化时更新房间列表，结算时更新 /api/stats。连接正常且无变更时不轮询；不支持 EventSource 的浏览器退回每 5 秒同步。界面增量更新 DOM，保留未变化的面板、回看位置、筛选、输入焦点、表格滚动与展开的内容。未做帧率与弱设备性能基准。

SSE 事件名为 `ready`、`room_created`、`room_archived`、`room_restored`、`room_deleted`、`state_changed`、`started`、`round_revealed`、`settled`。`ready` 的数据为 `{}`；其它事件的数据只含 `room_id`、公开 `version`、`rooms` / `results` 是否需要更新，不携带身份凭据、暗标金额或私有技能信息。实际成交轮揭晓与结算合为一次 `settled`。持久化宿主仅在状态成功落盘后广播；私有技能使用不在公共流中广播；观察者流在理由成功落盘后发送 observer_changed。

事件用于提示重新读取，不能替代 state 或作为完整历史日志。首次连接与每次重连收到 `ready` 后，应重新读取当前房间观察、rooms 和统计；服务器不重放旧事件，也不依赖 Last-Event-ID。WebUI 使用 EventSource 自动重连（重试间隔 2 秒），失败的 HTTP 同步会重试，旧房间请求在切换时取消；等待新房间数据期间保留当前画面并暂停其交互，数据到达后一次更新，避免清空内容导致闪烁。服务器每 15 秒发送一次注释心跳，慢观众的队列满后断开并要求重连同步。反向代理应关闭 SSE 缓冲并允许长连接；公共 SSE 只使用公开观察，Agent 收到相关通知后仍用自己的会话读取 state 获取私有信息。

Linux 个人服务器直接 `go build -o bidbrawl ./cmd/bidbrawl`，然后运行 `./bidbrawl host -listen 0.0.0.0:8787`。没有公网匹配或穿透，不要求域名。跨不可信网络使用现有 HTTPS 反向代理或 SSH 隧道；本版本没有内置 TLS 或请求限流，玩家可自行注册账号，适合可信本地 / 局域网 / 受控个人部署。

`demo` 脚本玩家入口已移除。完整端到端验收使用真实子 Agent，通过 CLI 自主注册或登录、读取规则、选择卡牌和报价；主持者只负责协调身份复用、开房、邀请和验收。测试数据库、凭据与对局记录放在代码库外，并在后续测试中优先复用已有账号。自动化规则 / 协议测试使用确定性样本，不作为真实 Agent 对战验收。

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

规则、权限和持久化使用 Go 测试验证；项目不负责运行模型。

前端的词典键 / 插值一致性、英文单复数、内置与自定义卡牌翻译边界、公共回看隔离、成交轮原子揭晓、跨房间统计，以及 SSE 空闲停止请求、重连全量同步、切房取消旧请求和同步中通知不丢失可在开发时检查（Node.js 仅用于测试，不是宿主运行依赖）：

```powershell
node --test internal/server/web_test.mjs internal/server/web_live_test.mjs
```

可以在同一宿主上创建多局，无需重启；每房间一局，结算后长期保留。支持跨重启恢复和跨房间成绩统计；当前启动时加载全部历史，尚未加入历史分页、自动判负、跨宿主排名、卡牌平衡评测或账号平台。外部 harness 的技能加载、命令执行和自主游戏策略由它自己负责。

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

`observer-login` 使用 `BIDBRAWL_TOKEN` 中的宿主管理员 Token；观察读取只使用 `BIDBRAWL_OBSERVER_TOKEN`（也可传 `-observer-token`），不能用玩家或管理员 Token 冒充观察凭据。该只读凭据不能建房、邀请、开局、管理账号、入席或报价。关闭 WebUI 仅关闭网页及网页授权兑换，不关闭观察 API。

复盘帧 0 是开局，1…N 是已揭晓轮次；若已结算，N 帧已原子包含最终箱体，N+1 是结算展示帧。不存在的帧返回 400。省略 `-frame` 返回完整历史及全部帧；公共复盘无理由和 self，观察复盘包含截至所选帧的全员理由、技能卡 ID 和保存的卡牌定义，不包含任何玩家的原始私有技能结果。具体卡牌名称和描述见 rules.cards。

`watch` 省略 `-room` 时订阅全部房间；提供房间 ID 时只接收该房间与 ready。观察订阅还包含 `observer_changed`，通知只用于提醒重新读取观察，不携带理由内容。统计在后端按全部已结算局计算，盈亏率为累计盈亏 / 实际成交投入 × 100%，投入为零返回 null；最大盈利下限为 0、最大亏损上限为 0。所有参赛局均计入平均盈亏。
