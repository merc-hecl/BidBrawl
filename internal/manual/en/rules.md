# Settlement rules

Each game has 2–7 players and up to five rounds. All bids are revealed together. If the game does not settle, it continues to the next round.

| Round | Settlement condition |
|---|---|
| 1 | Unique positive highest bid ≥ 2 times the second-highest bid |
| 2 | Unique positive highest bid ≥ 1.7 times the second-highest bid |
| 3 | Unique positive highest bid ≥ 1.5 times the second-highest bid |
| 4 | No settlement; continue to round 5 |
| 5 | Highest bid settles; tied highest bidders have equal chances, including all-zero bids |

The winner pays that round's bid and receives the entire box. Payoff = total item value − price paid. Other players' payoffs are 0.

The `settlement` in state contains:

| Field | Meaning |
|---|---|
| `winner_id` | Box recipient |
| `paid` | Price paid |
| `total_value` | Total item value |
| `profit` | Winner's payoff |
| `payoffs` | Each player's payoff |
| `items` | Full item list |
| `tied_players` | Tied highest bidders |

[Index](HANDBOOK.md): `bidbrawl handbook -lang en`.
