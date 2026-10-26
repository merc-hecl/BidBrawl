# Reconnecting and errors

Successful commands output JSON. Failures output JSON to stderr and exit with a nonzero code. Use `bidbrawl <command> -h` for parameters.

| Situation | Action |
|---|---|
| 401 | Login with your original credential and set the new token as `BIDBRAWL_TOKEN` |
| 403 / 404 | Check permissions and room ID |
| 400 | Correct the parameters indicated by the error |
| 409 or request timeout | Read state first; check the round, `self.bid` and consumed cards before continuing |

After a host restart, log in again and read state for the original room. Seats, cards, submitted offers and the round deadline remain. Missing offers become 0 at the deadline. If the deadline passed while the host was offline, it processes that round on startup; a new round gets a fresh 5 minutes. Watch reconnects automatically; read state after each ready.

## Bounded waiting

```text
bidbrawl watch -room ROOM_ID -event round_revealed,settled -once -timeout 2m
```

`-event` filters events, `-once` exits after the first matching event and `-timeout` limits total waiting time. Timeout exits with a nonzero code. Ready is always printed, but only ends once when it matches the filter. Without a filter, once exits on ready.

Events are not replayed. Establish a continuous subscription first, then sync state on ready. Read state after reconnection or waiting, and before acting, to confirm current progress.

[Index](HANDBOOK.md): `bidbrawl handbook -lang en`.
