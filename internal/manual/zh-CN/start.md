# 登录与入席

向房主领取地址与邀请链接，首次游玩时自行注册账号。以下大写占位符均替换为实际值。

## 注册

在命令执行环境中设置 `BIDBRAWL_URL` 为房主地址，再执行：

```text
bidbrawl register -name NAME
```

注册无需登录或房主 Token，返回永久 `account_id`、`name` 和登录凭据 `credential`，不会自动登录。妥善保存自己的凭据，不向其他玩家提供；已有账号直接登录，无需重复注册。同名账号会返回 409，不会覆盖原账号。凭据遗失时由房主管理员重置。

## 登录

在命令执行环境中设置 `BIDBRAWL_URL` 为房主地址、`BIDBRAWL_CREDENTIAL` 为自己的凭据，再执行：

```text
bidbrawl login
bidbrawl me
```

login 返回 JSON，将其中的 `token` 设置为 `BIDBRAWL_TOKEN` 后再执行 me。每次调用都需传入这些环境变量；退出登录用 `bidbrawl logout`。未指定地址时使用 `http://127.0.0.1:8787`。

`bidbrawl me` 查询自己的账号信息：`account_id` 为永久账号 ID，`name` 为名称，`assets` 为可用现金（整数金币）。`stats` 包含与 WebUI 一致的个人历史统计，字段见[成绩与复盘](results.md)。本命令必须登录，只查询当前会话本人，不接受 `-account`。无需先加入房间，结算后重新查询即可看到最新资产与统计。

## 入席

```text
bidbrawl cards
bidbrawl join -invite INVITATION_URL -cards CARD_ID,CARD_ID
```

`-cards` 指定最多五张不同卡，省略或传空值则不带卡。已知房间 ID 时，也可用 `join -room ROOM_ID`。卡牌必须先购买。准备阶段可以通过 loadout 调整，开局后锁定。

## 建房与开局

```text
bidbrawl create -name NAME -players N
bidbrawl invite -room ROOM_ID
bidbrawl join -room ROOM_ID -cards CARD_ID,CARD_ID
bidbrawl start -room ROOM_ID
```

人数 N 为 2～7。用 create 返回的 `room_id` 替换 ROOM_ID，将 invite 返回的 `invite_url` 交给其他玩家。创建者也需入席，满员后由房主开局；新一局另建房间。

下一步：[每轮操作](play.md)，执行 `bidbrawl handbook -topic play`。[返回目录](../HANDBOOK.md)。

## 购买、专长与仓库

```text
bidbrawl catalog
bidbrawl buy -card item_count -quantity 1
bidbrawl inventory
bidbrawl join -invite INVITATION_URL -specialty category -category household -cards item_count
bidbrawl loadout -room ROOM_ID -specialty late -cards item_count
bidbrawl sell -items STORED_ITEM_ID
bidbrawl sell -all
bidbrawl relief
```

共有 26 种专长：14 种侧重品类（家居、数码、工具、书画、珠宝、载具、古籍、乐器、玩具、钟表、军品、器械、古董、运动），12 种侧重跨品类的尺寸、品质、占格、样本或分区情报。用 `catalog` 查看 `specialties` 中各 ID、description 和逐轮 steps；指定房间用 `catalog -room ROOM_ID`。可用 `-category` 缩小到该专长支持的子类；region 选 northwest / northeast / southwest / southeast，默认 northwest。专长可重复选择，选择公开、结果私有。

购买与系统回收在非竞拍时间可用，包括房间准备阶段。买卡扣现金、使用消耗，未用卡保留，最多携带五张不同卡、每轮最多使用一张。库存中卡牌以实际购入价记录成本。仓库的出售 ID 见 inventory.items[].id；藏品无玩家市场、无保管费。

建房时系统从 10 种物品箱中等概率随机选定一种：综合、家居生活、数码办公、工业工具、艺术古董、珠宝矿藏、交通能源、图书文档、文娱收藏、野外装备。创建者不能指定。准备阶段可用 `state -room ROOM_ID` 查看 `rules.warehouse` 或 `context.warehouse`；入房、开局及重启不重抽。

综合物品箱没有品类偏好；其余主题箱各偏好 2–3 个品类，合计覆盖全部 20 个常规品类。例如文娱收藏箱偏好乐器音响、玩具模型和纪念收藏；野外装备箱偏好运动户外、医疗器械和军品装备。完整箱型及偏好见 `config` 的 warehouses（favored_categories）或 state 的 rules.warehouses，网页规则也显示本箱偏好品类。偏好品类中每个可放入模板的权重为其他同品质模板的三倍，其他品类仍会出现；品质权重、价格区间和箱体尺寸不因箱型改变。

默认门票 5,000，建房可用 `-ticket N` 设置（0 为免费）。准备阶段展示，成功开局才扣一次。建议现金是参考，实际门票余额是开局条件。

现金、藏品与卡牌合计价值不足 10,000 时，每个上海日期可手动领取一次 50,000 救济金，竞拍中不可领取。

未指定专长时默认珍品计数（analysis）。购买响应超时后，先核对 inventory 和现金，再决定是否重试，避免重复购买。

领价追踪根据上一轮唯一正最高价触发；跟价研判要求上一轮正报价严格低于最高价。并列最高不满足任一条件。仅使用已经揭晓的报价判断，当前暗标不参与；提前成交或弃权后没有后续专长奖励。
