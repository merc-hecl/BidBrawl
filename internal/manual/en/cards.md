# Cards and items

## Cards

Use `-cards CARD_ID,CARD_ID` when joining to carry up to five different cards. Omitting it or passing an empty value means no cards. Use at most one card before bidding each round. Each card is single-use per game; cards cannot be used after bidding.

`self.deck` lists carried cards, `self.used` lists consumed cards and `self.card_used_this_round` shows whether you have used a card this round. Run `bidbrawl cards -room ROOM_ID` for card names and descriptions. Results remain in `self.hints`.

Single-item skills reveal shape and position, plus the specified quality or category. Exact price and identity are available only through paid focused appraisal. Cumulative item knowledge is in `self.item_reveals`; raw results remain in `self.hints`. Count effects do not reveal item positions. Discoveries persist in later rounds. Authorized human observers see every item from the start by default. Selecting a player switches to that player's known items; clearing the selection restores the full box. Replay frames still exclude future rounds of player skill records.

Cards may specify an earliest usable round in `min_round`; early use is rejected without consuming a card. Focused appraisal and quadrant census unlock in round 2, quality census in round 3. Counts and mean cell areas are distinct; empty means are `null`. Price totals, exact mean prices and sample mean prices are unavailable. Public clues and specialties reveal no prices, names or template identities. Ordinary intel excludes easter eggs.

Some card definitions declare `target`; supply `-target` when using them: an ordinary category ID for category, an ordinary quality ID for quality, an integer cell area for area, or northwest/northeast/southwest/southeast for region. Choose after reading current clues. Fixed cards reject extra targets. Invalid targets do not consume a card; a valid empty scope does, returning zero counts, null means or empty reveal lists. Targets and results are private to you and authorized human observers. A used card cannot be retargeted.

## Clues and coordinates

A public clue appears in rounds 1 and 3, in `public_hints`. Revealed bids are in `history`. Your card results and current sealed bid are private.

Public clues can reveal individual shapes or qualities, quality counts/total or mean areas, category counts, category-quality intersections or quadrant-quality counts. Quadrants use the top-left corner: x ≥ floor(width/2) belongs east and y ≥ floor(height/2) belongs south. Public clues provide no individual prices, mean prices, total prices or identities.

At start, each player independently samples up to 3 items uniformly without replacement. All players receive the same number, regardless of quality. Samples may overlap between players. `self.initial_inspection` contains only sampled IDs, shape coordinates and qualities, never prices. Samples persist across reconnects and restarts.

Items are rectangles. Count, positions, qualities and prices are hidden until revealed. Items have no gaps or overlaps; empty space is only at the bottom. Coordinates use cells, starting at `(0,0)` at the top left, with x increasing rightward and y downward. The bottom-right boundary is excluded: `(2,3) → (6,7)` occupies 4×4 cells.

## View the catalog

`appearance_probability` is the baseline probability per draw (0–1), using the current configuration and item box category preference. Item details show it as a percentage. Remaining space changes the conditional probabilities of later placements.

Search public item templates with combinable filters:

```text
bidbrawl catalog -room ROOM_ID -size 2x2 -quality rare
bidbrawl catalog -room ROOM_ID -name "vase"
```

`-size` is width × height in cells, with exact orientation and no automatic rotation. `-quality` accepts a quality ID or name from the catalog. `-name` matches full or partial Chinese names or English translations, ignoring case. All supplied conditions must match. The response contains a `catalog` list and `count`, including names, categories, qualities, footprints and prices; no matches produces an empty list. Use `-room` for the room's frozen configuration, or omit it for host defaults. Without filters, the response includes the full catalog, qualities, categories, specialties and item box types. Candidate templates do not confirm actual box contents.

```text
bidbrawl handbook -topic cards -lang en
bidbrawl handbook -topic cards -lang en -live -room ROOM_ID
```

The first appends built-in default dimensions and cards to this page; the second appends the room's actual configuration. Use `-live` without `-room` for host configuration. You can also query `config -room ROOM_ID` and `cards -room ROOM_ID` separately. Built-in card text is translated; host-customized text is displayed as supplied.

[Index](HANDBOOK.md): `bidbrawl handbook -lang en`.

Ordinary quality price ranges overlap: the highest-quality item need not have the highest price, and prices within one tier vary substantially. Category, quality and shape can narrow catalog candidates; a candidate does not confirm actual contents. New accounts start with 600,000 coins; existing balances are not refilled. Always use `self.max_bid` as your actual bidding limit.

Buy cards with `buy -card ID -quantity N` before bidding. Ownership is required to carry them; use consumes one copy, unused copies remain. Random reveals target unknown attributes; exhausted pools yield no new items. `catalog` shows text-only item templates (category, quality, footprint and price), stored in JSON. Each specific item has one fixed quality, distinguished by its material, function, craftsmanship or provenance, rather than generic upgrade prefixes. Quality denotes the game's price tier. Footprints describe storage space, including folded, rolled or boxed sets. Common silhouettes have several candidates; category and quality narrow them further, while a few distinctive combinations can identify an item. Public clues and private specialty intel complement paid cards.

Focused appraisal uses `target=item`: pass an actual item_id already observed through your opening inspection, specialty, cards or public single-item clues. A catalog template ID is not an item ID. It unlocks in round 2, costs 25,000 by default and appraises only that chosen item, revealing its exact price and identity. The card is consumed even if you do not win.
