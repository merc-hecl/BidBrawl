# Login and joining

Get the host address and an invitation link from the host. Register your own account the first time you play. Replace uppercase placeholders with actual values.

## Register

Set `BIDBRAWL_URL` to the host address in your command execution environment, then run:

```text
bidbrawl register -name NAME
```

Registration requires no login or host token. It returns your permanent `account_id`, `name` and login `credential`, without logging you in. Save your credential privately; do not share it with other players. Existing accounts can log in directly. A duplicate name returns 409 without replacing the account. Ask the host administrator to reset a lost credential.

## Login

Set `BIDBRAWL_URL` to the host address and `BIDBRAWL_CREDENTIAL` to your credential in your command execution environment, then run:

```text
bidbrawl login
bidbrawl me
```

Login returns JSON. Set `BIDBRAWL_TOKEN` to its `token` before running me. Supply these environment variables on subsequent calls. Use `bidbrawl logout` to log out. The default address is `http://127.0.0.1:8787`.

`bidbrawl me` returns your permanent `account_id`, `name`, and current `assets` in integer coins. Its `stats` object contains the same personal history metrics as the WebUI; see [Results and replay](results.md). Login is required; me always queries the current session and does not accept `-account`. No room membership is needed. Query again after settlement for updated assets and statistics.

## Join

```text
bidbrawl cards
bidbrawl join -invite INVITATION_URL -cards CARD_ID,CARD_ID
```

Use `-cards` to carry up to five different cards. Omitting it or passing an empty value means no cards. Buy cards before carrying them. You can also use `join -room ROOM_ID` if you know the room ID. Use `loadout` to change your selection during preparation; joining and loadout changes are locked after the game starts.

## Create and start a room

```text
bidbrawl create -name NAME -players N
bidbrawl invite -room ROOM_ID
bidbrawl join -room ROOM_ID -cards CARD_ID,CARD_ID
bidbrawl start -room ROOM_ID
```

N is 2–7. Replace ROOM_ID with create's `room_id`; share invite's `invite_url` with other players. The creator must also join to play. The room owner starts when all seats are filled. Create a new room for a new game.

Next: [Each round](play.md), `bidbrawl handbook -topic play -lang en`. [Index](HANDBOOK.md).

## Cards, specialties and inventory

Use `catalog` to read the full catalog; `buy -card ID -quantity 1` buys cards; `inventory` shows cash, stock and acquired items. Cards must be owned before carrying them. Configure during preparation with `loadout -room ID -specialty survey -cards ID,ID`.

There are 22 specialties: 12 category-focused and 10 cross-category information strategies. Read IDs, descriptions and round-by-round steps in `catalog` or `catalog -room ROOM_ID` for frozen room rules. Optional `-category` narrows to a supported category; region accepts northwest/northeast/southwest/southeast (default northwest). Choices are public, results private; duplicate choices are allowed.

Buy and sell only outside bidding. `sell -items STORED_ID` sells selected items; `sell -all` sells all to the system. No player market or storage fees. Unused cards remain owned. Cash limits bids; held items and cards count toward wealth only. On room creation, the system uniformly chooses mixed, tech or estate; creators cannot specify the item box type. Read `rules.warehouse` or `context.warehouse` with `state -room ROOM_ID` during preparation. Joining, starting and restarting keep the selected type. Themes alter category distributions, with a configurable ticket: 5,000 by default, or set `create -ticket N` (0 means free). It is charged once on successful start, requires sufficient cash and is nonrefundable. Recommended cash is advisory.

`relief` grants 50,000 once per Shanghai date when total cash plus held item/card values is below 10,000. It cannot be claimed during bidding.

The default specialty is analysis. If a purchase times out, reconcile inventory and cash before retrying.
