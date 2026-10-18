# 公共提示插件

公共提示提供真实但局部的信息，让玩家仍须估计未公开物品的价值。第 1、3 轮各发布一条，按插件 ID 无放回抽选。每个房间固定保存插件配置；两条提示在开局时生成，只有到对应轮次才公开。观察、复盘、后续报价和服务重启都不执行插件，不重抽提示。提前成交时，第三轮提示始终不公开。

## 默认策略

默认七种插件，随机只决定对象，不向事实加入错误：

| 内置类型 | 揭示内容 |
| --- | --- |
| `random_shape` | 随机一件物品的 ID、尺寸和坐标，网页同步显示其轮廓 |
| `random_quality` | 随机一件物品的 ID 与品质 |
| `quality_count` | 随机一种实际存在的品质的件数 |
| `quality_average` | 随机一种品质的箱内实际均价，仅选件数不超过全箱一半的品质组 |
| `random_quality_price` | 先随机选一种存在的品质，再随机揭示其中一件物品的 ID 与实际价格 |
| `quality_presence` | 随机一种品质是否存在，允许抽到不存在的品质 |
| `quality_largest_area` | 随机一种存在的品质中，最大单件占用的格数，不提供物品 ID、位置或价格 |

品质在符合条件的不同品质之间等概率抽选，物品在对应范围内等概率抽选。品质均价正文四舍五入到两位小数，结构化字段保留计算结果。不提供全箱总件数、总价值、全箱均价或总价值区间。每局最多一条价格类提示，包括以不同 ID 配置的同类插件；只有一件非彩蛋物品时不抽单件价格，没有符合条件的品质组时不抽均价。不可用的插件被跳过，继续从剩余插件中抽选；自定义配置若不足两条适用线索，开局失败并保留大厅。私人技能与成交后的完整物品清单按原规则处理。

## 加载配置

```powershell
bidbrawl hints -defaults > hints.json
go build -o bin/quality-hint.exe ./examples/hint-plugin
```

将 `hints.json` 修改为（路径按本机调整）：

```json
{
  "schema_version": 1,
  "plugins": [
    {"id": "random_quality_price", "builtin": "random_quality_price"},
    {"id": "common_count", "command": ["F:/Projects/BidBrawl/bin/quality-hint.exe"]}
  ]
}
```

```powershell
bidbrawl host -hints-config ./hints.json -webui
```

配置必须有 2～64 个唯一插件 ID，ID 匹配 `[a-z][a-z0-9_]{0,63}`，至少含一个非价格类插件。每项只允许 `builtin` 或 `command` 二选一；内置类型取上面七种。可以重命名 ID；同一局保证 ID 不重复，若把同一内置类型配置成多个 ID 则可能得到类似线索。未知字段、无效类型或重复 ID 在启动时拒绝。配置最多 1 MiB；命令最多 32 个非空参数，每个最多 4096 字节。

`command` 是可执行文件与参数数组，直接执行，不经过 shell；建议使用绝对路径，相对路径按宿主工作目录解析。Python / Node 等脚本可用 `["python", "C:/plugins/clue.py"]`，运行时由插件作者安装；内置插件无需额外运行时。配置由宿主安装，观察者只读会话不提供上传或执行程序的接口。新配置只影响新建房间，已保存的大厅保留原配置。

`bidbrawl hints` 和 `hints -room ID` 返回公开插件 ID。`hints -defaults` 导出本地内置配置；公共 API 和 `rules.public_hint_plugins` 不返回执行路径或参数。

## 程序协议 v1

宿主为每条被抽中的提示启动一次进程，通过 stdin 发送一个 JSON 对象后关闭输入。`round` 是计划公开的轮次，开局时可能为 1 或 3；它不是进程执行时的实际轮次。不传历史报价或玩家数据。

```json
{
  "protocol_version": 1,
  "round": 1,
  "box_width": 20,
  "box_height": 30,
  "items": [
    {"id": "a123456789abcdef", "quality": "common", "price": 60, "top_left": {"x": 0, "y": 0}, "bottom_right": {"x": 2, "y": 2}}
  ]
}
```

这是供可信提示程序计算的私有箱内数据，不是发送给玩家的提示。`items` 仅包含非彩蛋物品及其公开标识 ID，彩蛋不传入内置或外部提示插件；不传账号、会话、暗标、技能结果或决策理由。没有非彩蛋物品时，默认插件仍可生成常规品质不存在、数量为零的线索。

stdout 必须只返回一个 JSON 对象，退出码为 0：

```json
{
  "title": "普通品质数量",
  "message": "箱内普通品质的物品共有 3 件。",
  "translations": {
    "en": {"title": "Common item count", "message": "The box contains 3 common-quality items."}
  },
  "fact": {"type": "quality_count", "quality": "common", "count": 3}
}
```

`title` / `message` 必须为非空纯文本，最多 80 / 1000 个 Unicode 字符，不允许控制字符。`translations` 可省略，仅支持 `zh-CN` 和 `en`，每项含相同的两个文本字段。网页按当前语言选择翻译，无翻译则显示原文；所有内容按文本转义，不解释 HTML。CLI 原样返回已保存文案。

`fact` 可省略；支持上表七种 `type`，只能携带该类型需要的字段：轮廓为 `item_id/top_left/bottom_right`，随机品质为 `item_id/quality`，品质件数为 `quality/count`，品质均价为 `quality/average_price`，品质单件价格为 `item_id/quality/price`，品质存在为 `quality/present`（布尔值，缺席时明确返回 `false`），同品质最大面积为 `quality/area`（1～16384 格）。未知字段和额外混合字段一律拒绝，不支持总件数或总价值事实类型。网页仅根据明确公开的单件事实合并轮廓、品质与价格，不读取玩家 `self`。携带价格事实的外部插件也计入每局一条价格类提示的限制。

每次执行最多 2 秒，stdout 最多 16 KiB。程序失败、超时、超量或返回无效内容时，开局失败，大厅不产生部分状态；修复插件后可重试。错误不回显插件 stderr 或原始输出，避免把私有箱内信息带进公共错误。保存失败仍遵循现有事务规则；未成功开局的重试可能重新抽取箱体、插件和提示。

示例源码：[examples/hint-plugin/main.go](../examples/hint-plugin/main.go)。它统计并准确公开普通品质的件数，不公布全箱件数或价格，无需依赖游戏内部包。作者可自行开发其他局部事实线索，文案与结构化事实应一致，不应通过翻译、正文或编码方式公开被排除的全箱信息。

## 信任与持久化

外部插件在宿主本机以宿主用户权限运行，不是进程沙箱。协议没有玩家私有数据，但恶意程序仍可能自行访问宿主文件系统或环境；仅安装可信程序。框架能约束结构与执行时间，无法仅凭文本判断作者是否公开了全箱数据，或是否遗漏了价格事实标记。自定义插件的真实性、揭示范围与安全性由宿主及作者审核。

当前对局快照格式为 6、账号数据库格式为 3；快照保存配置、已生成的完整提示计划和公开进度。只支持当前格式，不提供兼容分支。开发测试环境可按项目约定删除过时对局，保留账号和 DPAPI 凭据；不能通过删除整个数据库解决格式问题。
