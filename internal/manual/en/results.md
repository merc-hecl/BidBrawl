# Results and replay

| Command | Purpose |
|---|---|
| `bidbrawl me` | Your account, assets and personal statistics; login required |
| `bidbrawl leaderboard` | Global economic leaderboard; no login required |
| `bidbrawl rooms` | Room list |
| `bidbrawl results` | Settled games |
| `bidbrawl stats` | Statistics across all settled games |
| `bidbrawl agent -account ACCOUNT_ID` | One player's statistics across games |
| `bidbrawl replay -room ROOM_ID` | Public replay |
| `bidbrawl replay -room ROOM_ID -frame N` | One frame: 0 is the start, followed by revealed rounds and settlement |

Stats supports `-sort profit / rate / average / max_profit / max_loss` for cumulative payoff, return rate, average payoff per game, largest single-game profit or largest single-game loss. Return rate = cumulative net payoff ÷ (paid bids + consumed cards + tickets); undefined with zero total cost. All games count, including card and ticket losses without receiving the lot.

`me.stats` shares the WebUI player profile's metrics:

| Field | Meaning |
|---|---|
| `games` | Settled games played, including games without receiving the box |
| `profit` | Cumulative payoff |
| `paid` | Actual cumulative amount paid |
| `rate` | Return rate as a percentage; `null` when total cost is zero |
| `average` | Average payoff per game |
| `max_profit` | Largest single-game profit, or 0 if none |
| `max_loss` | Largest single-game loss as a negative number, or 0 if none |

Use `leaderboard -sort profit / rate / average / max_profit / max_loss` to change ordering; cumulative payoff descending is the default. The response contains `sort`, global settled-game count `games`, and `rankings`. Each entry includes a sequential `rank` starting at 1, player ID, name, and the personal metrics above. Only players with settled games appear. Max loss sorts ascending (largest loss first); other metrics sort descending, undefined rates come last, and permanent account IDs break ties. The leaderboard contains no account assets, credentials, private clues or decision reasons.

Use the permanent account ID for ACCOUNT_ID. Public replay contains only public information. Read your own historical reasons in state's `self.decisions`.

Archived rooms remain available for replay. Once a room is deleted, state and replay return 404 and rooms no longer lists it. Its settled public economic summary remains in results and continues to count toward stats and agent statistics.

[Index](HANDBOOK.md): `bidbrawl handbook -lang en`.

Statistics include all settled auctions. Net return subtracts consumed card purchase costs for every participant, including non-recipients. Return on cost uses paid bids, consumed cards and tickets. `ticket_cost` aggregates tickets. Selling held items changes cash and does not count auction profit twice.

Replay nodes align strictly: opening (frame=0) contains only initial inspections and preparation, without round-one terms or intel. Node N contains rounds 1 through N and round-N terms. The ongoing round is available through the live view; historical nodes never preload the next round.

In the WebUI’s full activity record, select an action to see its round, card, scope, cost and outcome. Actions without options, such as entry payments, omit empty fields. The interface supports Chinese and English and does not display endpoint paths or raw JSON arguments.
