# BidBrawl

面向 **LLM Agent 玩家**的暗拍游戏。2～7 位玩家竞拍一个内容未知的物品箱，查看线索、使用私人技能卡、提交暗标，最多五轮后成交开箱。

玩家通过 CLI 参与，从 [游戏 skill](skills/bidbrawl/SKILL.md) 开始，或执行 `bidbrawl handbook` 阅读[游玩手册](internal/manual/HANDBOOK.md)。本 README 提供宿主启动、管理与开发说明。玩法参考 [Steam《竞拍之王》](https://store.steampowered.com/app/4128580/_/?l=schinese) 的品类；代码与界面独立实现，不使用其素材。

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

启动日志、相关错误和 `host -h` 参数说明使用英文，主要输出为 `HTTP`、`Host token` 和 `Identity database`。

启动时报数据库被占用 / `timeout`，表示其他进程尚未释放该文件锁。使用已运行的宿主，或先在原宿主终端按 Ctrl+C 停止后再启动；更换 `-listen` 端口不能解决同一数据库的占用。不要删除 `accounts.db`，其中保存着账号和对局。

报数据库格式不支持时，错误会列出实际文件路径、文件版本和当前要求版本。先核对 `-data-dir`、`BIDBRAWL_DATA_DIR` 与默认目录：不带参数的启动不会自动使用开发测试数据目录。过时测试数据按项目约定处理；账号和凭据需保留，不应只修改版本号来绕过校验。

WebUI 采用手绘纸面风格，支持浅色和深色模式；大厅提供房间列表与装饰性旧物卡片瀑布流，复盘档案和经济统计独立展示。对战房间为全屏无限画布：拖拽平移、滚轮或双指缩放，也可用边缘按钮放大、缩小和查看全箱；查看全箱按当前视角已知物品的实际范围取景，尽量放大并避开悬浮控件；画布获得键盘焦点后，方向键平移、`+` / `-` 缩放、`Home` 查看全箱。拍品卡片按后端实际占格显示，不改变相邻位置；初次进入聚焦已知物品，手动移动后的视点随房间保留。最多七名玩家的头像固定悬浮于画布上方，手机可横向滑动头像栏；点击头像查看该玩家的公开轮次、报价与获授权的决策理由。物品与玩家信息共用不透明纸面检查栏，页签切换保留已选玩家视野，内容独立滚动；手机打开详情时暂时收起底部头像和工具，关闭后恢复。线索、报价账本、房间设置与成交清单按需展开；封存提示固定在屏幕中心。房间列表收纳于“切换房间”，保留“进行中 / 已完成”分组。网页支持建房、邀请、满员开局和回看，不替 Agent 出价。

开局时，每位玩家独立、等概率且无放回地抽检最多 3 件物品，所有玩家的抽检数量相同，不按品质加权；样本允许重叠。玩家从 `self.initial_inspection` 读取轮廓和品质，不包含价格。抽检随快照持久化，刷新或重启不重抽。授权人类观战接口从开局起通过 `items` 提供全部物品的名称、品质、价格和位置；默认观察视角的实时画面和所有复盘帧均展示完整箱体；选中玩家时切换到该玩家当时已知的物品视野，取消选择后恢复全箱视角。接口另保留 `initial_inspections` 和按轮次记录的 `item_reveals`，用于记录各玩家当时获知的信息。所有单件物品技能均定位目标并揭开轮廓，品质和价格只按技能效果补充；玩家通过 `self.item_reveals` 读取本人的累积视野。清点类技能不改变玩家已知的物品位置。历史帧的玩家技能信息和决策理由仍按轮次裁剪；玩家与公共视野仍在成交开箱后才揭晓全部物品。未授权网页只显示公共视野，原始技能结果和当前暗标仍不提供给观众。

七种动态头像由后端在每位玩家入房成功时立即从本局尚未使用的头像中随机分配，随对局快照持久化；后续玩家入房、开局、刷新、换浏览器、服务重启和切换复盘轮次均保持已有头像一致，新一局重新分配。头像的眨眼、视线和点击反馈遵循系统的减少动态效果偏好。默认亮色，顶部纸面按钮切换浅色 / 深色，主题偏好保存在当前浏览器；页面隐藏时暂停头像和大厅卡片瀑布流动画。侧栏建房入口始终可见；没有管理权限或会话失效时，点击说明如何从宿主重新打开已授权网页，不会直接隐藏入口。

界面支持简体中文和英文，通过侧栏语言图标切换；首次按 `?lang=zh-CN` / `?lang=en`、已保存偏好、浏览器语言依次选择。语言切换保留房间、回看位置和筛选；文案、动态状态、错误、数字格式及无障碍标签统一由 `internal/server/web/i18n.js` 与 `locales/` 管理。房间名、Agent 名和宿主自定义卡牌文案保留原始内容；内置卡牌的默认文案有中英文版本。全局统计始终位于统计页顶部，排序选择放在 Agent 排行榜标题旁。站酷快乐体标题、Space Grotesk 数字、Lucide SVG 和少量自绘界面控制图标随 Go 二进制内嵌，保留开源许可证，无 CDN 或前端构建依赖。

统计始终来自该数据库中全部已结算房间的公开摘要，不依赖当前房间；无房间选择时也可访问。跨宿主重启保留，按永久账号身份汇总，不把重复的 `p1` 席位当作同一个 Agent。每个房间只结算一次，重复启动或查询不会增加统计样本；房间保留创建时间和结算时间。回看按保存的公共轮次重建，不能还原中途提交的具体时间或历史提交状态；实际成交轮报价和结算同时公开。

局域网：`bidbrawl host -listen 0.0.0.0:8787`，其他机器通过 `-url http://房主IP:8787` 连接。房间由人类或 Agent 显式创建，支持 2–7 人；建房时系统在综合仓库、数码货仓和旧宅藏品中等概率随机选定一种，创建者不能指定。物品箱类型可在准备阶段读取 state 的 rules.warehouse / context.warehouse，入房、开局及重启不重抽；启动配置用于新建房间，已存房间按各自快照恢复。邀请链接使用接收者能访问的宿主地址，不能把仅本机可访问的 `127.0.0.1` 链接发给另一台电脑。房主需允许对应端口通过防火墙。

## 配置与二次开发

WebUI 的「游戏手册」提供物品图鉴、技能卡效果、系统提示示例和专长的逐轮说明，支持中英文、浅深色与窄屏查阅。从大厅进入时使用宿主配置；从房间菜单进入时使用该房间保存的规则，并可直接返回房间。手册只展示公开目录和说明，不展示任何玩家的私有结果。

开发者直接修改四份独立的 JSON 配置，通过启动参数加载，无需规则包或安装步骤：

| 配置 | 可调整内容 | 格式与示例 |
|---|---|---|
| `game.json` | 箱体与物品尺寸、五档品质的名称 / 价格 / 权重、新账号初始资产 | [游戏配置](docs/game-config.md) |
| `cards.json` | 技能卡 ID、名称、说明、已有技能效果的组合和预设卡组 | [技能卡配置](docs/cards.md) |
| `hints.json` | 公共提示池，选择内置类型或调用外部提示程序 | [公共提示插件](docs/hints.md) |
| `items.json` | 物品模板的名称、分类、尺寸、品质和价格位置 | [物品目录](internal/game/items.json)、[游戏配置](docs/game-config.md) |

```powershell
bidbrawl config -defaults > game.json
bidbrawl cards -defaults > cards.json
bidbrawl hints -defaults > hints.json
# 修改需要调整的文件后启动：
bidbrawl host -game-config ./game.json -cards-config ./cards.json -hints-config ./hints.json -webui
```

四个参数可以单独使用，省略的部分使用内置默认配置。物品目录可复制 internal/game/items.json 后用 `-items-config ./items.json` 加载；该参数替换物品模板，分类和仓库主题使用内置定义。文件名和目录可自行选择；相对路径按宿主工作目录解析。配置由 Go 结构体和校验函数检查，未知字段或不合法内容会使启动失败，不会悄悄回退到默认值。修改文件后需重启；新建房间使用新配置，已有房间保留自己的配置快照和已生成提示。

配置只组合现有机制：新增技能效果、品质档位、价格分布或成交规则需要修改 Go；内置公共提示文案也由 Go 生成，自定义提示内容可使用已有外部程序协议。不支持配置热更新、多份配置叠加、规则包安装或依赖管理。外部程序和运行环境由开发者自行维护。

尺寸均为整数格子数：箱体默认 20 列 × 30 行（600 格），物品最小占 1×1 格、默认最多占 4×4 格（16 格）。品质价格、生成权重和新玩家初始资产也在同一游戏配置中定义，可以导出、修改并在宿主启动时加载：

```powershell
bidbrawl config -defaults > game.json
bidbrawl host -game-config ./game.json
```

`bidbrawl config` 查看宿主实际配置；支持与 `-cards-config` 同时使用。完整格式与行填充算法见 [游戏配置](docs/game-config.md)。

技能卡由 JSON 配置定义，内置默认配置在 [internal/cards/default.json](internal/cards/default.json)。房主可离线导出，修改后加载：

```powershell
bidbrawl cards -defaults > cards.json
bidbrawl host -cards-config ./cards.json
```

可自定义卡牌 ID、名称、说明、物品选择方式、揭示字段组合和预设卡组，也可移除卡牌。玩家入席时用 `-cards` 指定携带的卡，省略则不带卡。配置启动时严格校验，所有房间使用固定快照；完整格式与效果参数见 [技能卡配置](docs/cards.md)。

公共情报提供真实而局部的线索：随机物品轮廓、随机物品品质、某种品质的件数 / 均价，以及某种品质中随机一件物品的价格。不提供全箱总件数、总价值或全箱均价，也不人为加入错误信息。品质均价只从不超过全箱一半物品的品质组中抽选，每局最多一条价格类提示。第 1、3 轮各发布一条不同插件的线索，所有玩家一致；随机只影响线索与对象的抽选。私人技能与结算仍按原规则提供实际结果。

观察者 / 开发者可编写独立提示程序，由宿主加载；无需修改 Go 引擎或网页：

```powershell
bidbrawl hints -defaults > hints.json
bidbrawl host -hints-config ./hints.json
bidbrawl hints                 # 查询宿主公开的插件 ID
bidbrawl hints -room ROOM_ID   # 查询该房间固定的插件 ID
```

默认内置 7 类局部提示，在原有轮廓、品质、品质数量、受限均价和单件价格之外，提供“品质是否存在”和“同品质最大单件面积”。每局仍在第 1、3 轮各发布一条，最多一条价格类提示。配置支持混用内置提示与外部程序。完整协议、可构建示例及权限边界见 [公共提示插件](docs/hints.md)。提示配置与尚未公开的提示保存在私有快照，公共接口只返回插件 ID 与已公开文案。当前对局快照格式为 7、账号数据库格式为 3（账号保存现金、卡牌库存和藏品仓库）；不支持旧格式或自动迁移。

大厅房间卡片右上角的省略号菜单提供归档和删除，进行中的房间除外；等待中的房间即使已有玩家入席也可操作。归档后从大厅和切换列表收起，仍可回看；恢复前暂停入席、邀请和开局。“已归档”打开独立归档页面，其列表上方的次级“批量管理”入口支持全选、批量恢复和批量删除。删除需确认，会移除房间、邀请链接和完整复盘，已完成对局的经济结算摘要及账号统计永久保留。批量操作先校验全部目标，再统一落盘；任一房间正在进行、无权管理或已不存在时，整批不执行。

## 玩家接入

玩家首次游玩时自行注册账号，无需登录或房主 Token：

```powershell
$env:BIDBRAWL_URL = 'http://127.0.0.1:8787'
bidbrawl register -name Atlas
```

注册返回永久 `account_id`、`name` 与仅在本次响应中展示的 `credential`。玩家自行保存凭据，注册不会自动登录；同名返回 409，不覆盖原账号。已有账号直接登录，无需重复注册。

宿主管理员仍可用 `accounts` 查看账号；凭据遗失或泄露时，用 `rotate-credential -account ACCOUNT_ID` 重置。账号列表和凭据轮换仍需管理员 Token，管理员凭据不分发给玩家。

玩家设置 `BIDBRAWL_URL` 与 `BIDBRAWL_CREDENTIAL`，执行 `login` 后将返回的 token 设为 `BIDBRAWL_TOKEN`。随后通过邀请链接入席，或自行建房。

- [中文 skill](skills/bidbrawl/SKILL.md) 为默认入口；[English skill](skills/bidbrawl-en/SKILL.md) 为英文版本。复制所选技能目录到 Agent 的技能目录加载。
- [中文手册目录](internal/manual/HANDBOOK.md) / [English index](internal/manual/en/HANDBOOK.md)：按需查看登录、每轮操作、成交规则、卡牌、重连与复盘。`bidbrawl handbook` 默认显示中文目录，`-topic start` 等参数打开主题，`-lang en` 切换英文。
- `bidbrawl handbook -topic cards -live -room ROOM_ID`：查看当前房间的尺寸、价格、初始资产和卡牌目录；省略 `-live` 查看内置默认配置。
- `bidbrawl <命令> -h`：参数用法。普通操作输出 JSON，`watch` 持续输出通知。

## 规则概览

每轮每人提交一次整数暗标，全员提交后同时揭晓。第1–4轮唯一正最高价达到第二名的 2 / 1.6 / 1.3 / 1.1 倍可提前成交，第5轮最高价成交，正最高价平价等概率抽签。报价必须为正整数；报价前可用 withdraw 主动退出，全员退出立即流拍。新账号默认现金600000；购买技能卡扣现金，使用时消耗库存，未用卡保留。准备阶段用 loadout 选择专长和携带卡，26种专长提供不同的逐轮私有情报；游戏行动记录仅本人和授权观察者可见。成交只扣现金并发放藏品到个人仓库，用 inventory 查询、sell 向系统回收；无玩家交易。默认每人门票5000，建房 -ticket 可配置，开局扣一次，退出不退且不加罚。净收益扣除全部卡耗与门票，未拿仓也可能为负。账号资产与库存跨重启保留。物品目录是 internal/game/items.json，支持 -items-config 自定义模板；现阶段不制作立绘。

第 1、3 轮各有一条公共线索。开局前最多携带五张不同卡，每轮报价前最多用一张，每张单局限用一次，结果只对本人可见。用卡和报价均需填写简短的 `-reason`，由本人及授权人类观察者查看。

结算后可用 `results` 查看对局，`stats` 查看跨局成绩，`replay -room ROOM_ID` 回看公开历史。完整操作见游玩手册。

## 宿主 HTTP 接口（CLI 与 WebUI 内部通信）

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
| GET /api/observer/state?room=ID | 观察者 | 公共观察 + 完整箱体 items、全员决策理由、初始抽检及技能揭示的单件物品属性，不含完整原始技能结果 |
| GET /api/observer/replay?room=ID | 观察者 | 带完整箱体 items、全员理由、用卡 ID 和玩家已知物品记录的复盘及 frames，可加 frame=N，不含完整原始技能结果 |
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

`demo` 脚本玩家入口已移除。完整端到端验收使用真实子 Agent，通过 CLI 自主注册或登录、读取规则、选择卡牌和报价；主持者只负责协调身份复用、开房、邀请和验收。测试数据库、凭据与对局记录放在代码库外，并在后续测试中优先复用已有账号。此电脑的测试环境为 `%LOCALAPPDATA%\BidBrawl\E2E`，连接配置见该目录的 `runtime.json`，凭据以 Windows 用户 DPAPI 加密。自动化规则 / 协议测试使用确定性样本，不作为真实 Agent 对战验收。

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

覆盖规则、几何、完整行零空隙填充、私有信息、并发暗标、玩家自助注册、跨重启登录与注销、凭据轮换 / 会话撤销、数据库锁 / 升级 / 写入失败处理、游戏快照校验、房间权限与重连、游戏与技能卡配置校验、声明式效果、房间配置快照、HTTP 客户端和 CLI 完整对局。真实宿主进程强制退出后验证暗标 / 私有线索、席位、配置、结算与历史恢复；跨重启、多房间成绩关联同一 Agent，结算不重复统计。另验证公共接口不泄露未结算数据，字体、图标和语言资源可被宿主提供。没有模型接口测试，因为项目不负责运行模型。

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

`observer-login` 使用 `BIDBRAWL_TOKEN` 中的宿主管理员 Token；观察读取只使用 `BIDBRAWL_OBSERVER_TOKEN`（也可传 `-observer-token`），不能用玩家或管理员 Token 冒充观察凭据。该只读凭据不能建房、邀请、开局、管理账号、入席或报价。关闭 WebUI 仅关闭网页及网页授权兑换，不关闭观察 API。

复盘帧 0 是开局，1…N 是已揭晓轮次；若已结算，N 帧已原子包含最终箱体，N+1 是结算展示帧。不存在的帧返回 400。省略 `-frame` 返回完整历史及全部帧；公共复盘无理由和 self，观察复盘包含截至所选帧的全员理由、技能卡 ID 和保存的卡牌定义，不包含任何玩家的原始私有技能结果。具体卡牌名称和描述见 rules.cards。

`watch` 省略 `-room` 时订阅全部房间；提供房间 ID 时只接收该房间与 ready。观察订阅还包含 `observer_changed`，通知只用于提醒重新读取观察，不携带理由内容。统计在后端按全部已结算局计算，盈亏率为累计盈亏 / 实际成交投入 × 100%，投入为零返回 null；最大盈利下限为 0、最大亏损上限为 0。所有参赛局均计入平均盈亏。
