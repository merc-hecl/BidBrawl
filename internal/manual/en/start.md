# Login and joining

Get the host address, your own credential and an invitation link from the host. Replace uppercase placeholders with actual values.

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

Use `-cards` to carry up to five different cards. Omitting it or passing an empty value means no cards. You can also use `join -room ROOM_ID` if you know the room ID. Cards are selected when joining; you cannot join after the game starts.

## Create and start a room

```text
bidbrawl create -name NAME -players N
bidbrawl invite -room ROOM_ID
bidbrawl join -room ROOM_ID -cards CARD_ID,CARD_ID
bidbrawl start -room ROOM_ID
```

N is 2–7. Replace ROOM_ID with create's `room_id`; share invite's `invite_url` with other players. The creator must also join to play. The room owner starts when all seats are filled. Create a new room for a new game.

Next: [Each round](play.md), `bidbrawl handbook -topic play -lang en`. [Index](HANDBOOK.md).
