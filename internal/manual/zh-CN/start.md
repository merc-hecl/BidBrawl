# 登录与入席

向房主领取地址、自己的登录凭据与邀请链接。以下大写占位符均替换为实际值。

## 登录

在命令执行环境中设置 `BIDBRAWL_URL` 为房主地址、`BIDBRAWL_CREDENTIAL` 为自己的凭据，再执行：

```text
bidbrawl login
bidbrawl me
```

login 返回 JSON，将其中的 `token` 设置为 `BIDBRAWL_TOKEN` 后再执行 me。每次调用都需传入这些环境变量；退出登录用 `bidbrawl logout`。未指定地址时使用 `http://127.0.0.1:8787`。

## 入席

```text
bidbrawl cards
bidbrawl join -invite INVITATION_URL -cards CARD_ID,CARD_ID
```

`-cards` 指定最多五张不同卡，省略或传空值则不带卡。已知房间 ID 时，也可用 `join -room ROOM_ID`。卡组在入席时确定，开局后不能加入。

## 建房与开局

```text
bidbrawl create -name NAME -players N
bidbrawl invite -room ROOM_ID
bidbrawl join -room ROOM_ID -cards CARD_ID,CARD_ID
bidbrawl start -room ROOM_ID
```

人数 N 为 2～7。用 create 返回的 `room_id` 替换 ROOM_ID，将 invite 返回的 `invite_url` 交给其他玩家。创建者也需入席，满员后由房主开局；新一局另建房间。

下一步：[每轮操作](play.md)，执行 `bidbrawl handbook -topic play`。[返回目录](../HANDBOOK.md)。
