# Settlement rules

2–7 players, up to five rounds. Offers are sealed until everyone submits.

| Round | Unique positive highest offer settles at |
|---|---|
| 1 | ≥ 2 × runner-up |
| 2 | ≥ 1.6 × runner-up |
| 3 | ≥ 1.3 × runner-up |
| 4 | ≥ 1.1 × runner-up |
| 5 | Highest offer, uniformly random among positive ties |

Offers must be positive integers. Before bidding, use `bidbrawl withdraw -room ROOM_ID -round ROUND -reason "reason"` to exit permanently; later rounds are skipped and new specialty intel stops. Everyone exiting ends unsold. One remaining positive offer can settle early. Other players may raise or lower later offers.

A ticket is charged once per player on successful start: 5,000 by default, configurable per room; read `state.rules.ticket_fee`. It is nonrefundable, with no extra withdrawal fine. Insufficient cash prevents the entire start and charges nobody.

The winner pays cash and keeps the items in their warehouse. Items are not automatically converted into cash. Cards are consumed only on use, even if you do not acquire the lot.

Settlement fields: `gross_profit` = acquired value minus paid (0 when unsold); `card_costs` = consumed cards at their purchase costs per player; `total_card_cost` = total consumed costs; `ticket_costs` and `total_ticket_cost` = tickets per player and in total; `easter_egg_value` = egg value in the lot; `profit` = total net return after cards and tickets; `payoffs` = each player's net return; `cash_changes` = settlement cash movements only, excluding earlier purchases and later buyback. Unsold auctions have `method=unsold` and an empty `winner_id`.

[Index](HANDBOOK.md): `bidbrawl handbook -lang en`.

Eggs form a separate category, with probability one thousandth of legendary items and a fixed buyback price of 17,500,000. Their special outline appears only when the egg category is known; legendary quality or a 1×1 shape alone does not identify an egg.
