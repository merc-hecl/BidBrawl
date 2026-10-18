# Cards and items

## Cards

Use `-cards CARD_ID,CARD_ID` when joining to carry up to five different cards. Omitting it or passing an empty value means no cards. Use at most one card before bidding each round. Each card is single-use per game; cards cannot be used after bidding.

`self.deck` lists carried cards, `self.used` lists consumed cards and `self.card_used_this_round` shows whether you have used a card this round. Run `bidbrawl cards -room ROOM_ID` for card names and descriptions. Results remain in `self.hints`.

Every single-item skill reveals its target's shape and position, plus only the quality or price learned by the skill. Cumulative item knowledge is in `self.item_reveals`; raw results remain in `self.hints`. Largest Item Price uncovers its target and displays the price without adding quality. Count effects do not reveal item positions. Discoveries persist in later rounds. Authorized human observers see every item from the start by default. Selecting a player switches to that player's known items; clearing the selection restores the full box. Replay frames still exclude future rounds of player skill records.

## Clues and coordinates

A public clue appears in rounds 1 and 3, in `public_hints`. Revealed bids are in `history`. Your card results and current sealed bid are private.

At start, each player independently samples `min(3, item count)` items uniformly without replacement. All players receive the same number; each item has the same inclusion probability for every player regardless of quality. Samples may overlap between players. `self.initial_inspection` contains only sampled IDs, shape coordinates and qualities, never prices. Samples persist across reconnects and restarts.

Items are rectangles. Count, positions, qualities and prices are hidden until revealed. Items have no gaps or overlaps; empty space is only at the bottom. Coordinates use cells, starting at `(0,0)` at the top left, with x increasing rightward and y downward. The bottom-right boundary is excluded: `(2,3) → (6,7)` occupies 4×4 cells.

## View the catalog

`appearance_probability` is the baseline probability per draw (0–1), using the current configuration and warehouse preference. Item details show it as a percentage. Remaining space changes the conditional probabilities of later placements.

Search public item templates with combinable filters:

```text
bidbrawl catalog -room ROOM_ID -size 2x2 -quality rare
bidbrawl catalog -room ROOM_ID -name "vase"
bidbrawl catalog -category easter_egg
```

`-size` is width × height in cells, with exact orientation and no automatic rotation. `-quality` accepts a quality ID or name from the catalog. `-name` matches full or partial Chinese names or English translations, ignoring case. All supplied conditions must match. The response contains a `catalog` list and `count`, including names, categories, qualities, footprints and prices; no matches produces an empty list. Use `-room` for the room's frozen configuration, or omit it for host defaults. Without filters, the response includes the full catalog, qualities, categories, specialties and warehouse types. Candidate templates do not confirm actual box contents.

```text
bidbrawl handbook -topic cards -lang en
bidbrawl handbook -topic cards -lang en -live -room ROOM_ID
```

The first appends built-in default dimensions and cards to this page; the second appends the room's actual configuration. Use `-live` without `-room` for host configuration. You can also query `config -room ROOM_ID` and `cards -room ROOM_ID` separately. Built-in card text is translated; host-customized text is displayed as supplied.

[Index](HANDBOOK.md): `bidbrawl handbook -lang en`.

Buy cards with `buy -card ID -quantity N` before bidding. Ownership is required to carry them; use consumes one copy, unused copies remain. Random reveals target unknown attributes; exhausted pools yield no new items. `catalog` shows text-only item templates (category, quality, footprint and price), stored in JSON. Each specific item has one fixed quality, distinguished by its material, function, craftsmanship or provenance, rather than generic upgrade prefixes. Quality denotes the game's price tier. Footprints describe storage space, including folded, rolled or boxed sets. Common silhouettes have several candidates; category and quality narrow them further, while a few distinctive combinations can identify an item. Public clues and private specialty intel complement paid cards.
