# Cards and items

## Cards

Use `-cards CARD_ID,CARD_ID` when joining to carry up to five different cards. Omitting it or passing an empty value means no cards. Use at most one card before bidding each round. Each card is single-use per game; cards cannot be used after bidding.

`self.deck` lists carried cards, `self.used` lists consumed cards and `self.card_used_this_round` shows whether you have used a card this round. Run `bidbrawl cards -room ROOM_ID` for card names and descriptions. Results remain in `self.hints`.

## Clues and coordinates

A public clue appears in rounds 1 and 3, in `public_hints`. Revealed bids are in `history`. Your card results and current sealed bid are private.

Items are rectangles. Count, positions, qualities and prices are hidden until revealed. Items have no gaps or overlaps; empty space is only at the bottom. Coordinates use cells, starting at `(0,0)` at the top left, with x increasing rightward and y downward. The bottom-right boundary is excluded: `(2,3) → (6,7)` occupies 4×4 cells. Value bands `[lower, upper)` include the lower bound and exclude the upper bound.

## View the catalog

```text
bidbrawl handbook -topic cards -lang en
bidbrawl handbook -topic cards -lang en -live -room ROOM_ID
```

The first appends built-in default dimensions and cards to this page; the second appends the room's actual configuration. Use `-live` without `-room` for host configuration. You can also query `config -room ROOM_ID` and `cards -room ROOM_ID` separately. Built-in card text is translated; host-customized text is displayed as supplied.

[Index](HANDBOOK.md): `bidbrawl handbook -lang en`.
