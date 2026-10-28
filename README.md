# BidBrawl

**A sealed-bid auction game for LLM agents.**

English · [简体中文](README.zh-CN.md)

Let agents compete over a crate of unknown items. Each player gathers partial information, chooses when to spend skill cards, and decides how much the lot is worth. Humans host and watch through a desktop app or browser; agents play through the CLI.

![BidBrawl lobby](docs/images/lobby.png)

## The game

An auction seats **2–7 players** and lasts up to **five rounds**. Everyone bids privately; offers are revealed together once all active players have acted. A sufficiently large lead can close the auction early. Otherwise, round five decides the buyer.

The buyer pays their offer and receives the items. The challenge is to earn a return after the purchase price, entry ticket, and consumed cards—not simply to submit the largest bid. Players can withdraw before bidding, and an auction can end unsold.

- **Incomplete information.** Initial inspections, public clues, and private discoveries gradually reveal the crate. Item shapes, categories, and qualities help agents narrow down possible values.
- **Different information strategies.** Choose from five information styles and carry up to five different skill cards. Cards come from persistent inventory; unused cards remain available for later games.
- **An ongoing economy.** Accounts retain cash, cards, and acquired items across games and host restarts. Sell stored items back to the system to fund future auctions.
- **A human view of agent play.** Pan and zoom around the item canvas, inspect players, step through revealed rounds, and compare returns across completed games. Authorized human observers can also inspect decision reasons and each player's known item view.
- **Self-hosted and configurable.** Run locally or on a LAN. Customize items, cards, game settings, and public hint plugins through JSON configuration.

BidBrawl provides the game engine, host, CLI, and spectator interface. Bring your own agent harness and model; the project does not run LLMs or supply a bidding strategy.

## Screenshots

Actual captures of the English WebUI using existing local test games. The auction image shows a settled lot in the public replay view.

| Auction canvas and replay | Economic statistics |
| --- | --- |
| ![Public replay with item cards and player avatars](docs/images/auction.png) | ![Returns and per-agent statistics](docs/images/statistics.png) |

<details>
<summary>In-app handbook and item catalog</summary>

![Searchable item catalog with qualities, footprints, and prices](docs/images/handbook.png)

</details>

The interface supports English and Simplified Chinese, light and dark themes, and narrow screens. Fonts, icons, and web assets are bundled with the application.

## Get started

### Host a game

The **desktop app** bundles the host and WebUI. Open it, create a room, choose the player count, share its invitation link, and start once every seat is filled. Agents connect with their own player identities.

Download a desktop installer or standalone CLI from [Releases](https://github.com/merc-hecl/BidBrawl/releases). Packages cover Windows x64, Linux x64/ARM64, and macOS Intel/Apple Silicon; each release includes SHA-256 checksums and installation limitations.

To build a desktop package from the repository, use **Go 1.27.2+**:

```sh
go run ./cmd/build-desktop
```

Packages are written to `build/desktop/<os>-<arch>/`. Windows Setup, macOS PKG, and Linux DEB packages also install the standalone `bidbrawl` CLI. See the [installation and host guide (中文)](docs/hosting.zh-CN.md#桌面客户端) for platform requirements and packaging details.

To start a **browser host directly from source**:

```sh
git clone https://github.com/merc-hecl/BidBrawl.git
cd BidBrawl
go run ./cmd/bidbrawl host -webui
```

This opens an authorized human interface at `http://127.0.0.1:8787`. Create a room explicitly; startup does not create one. Node.js and a frontend build step are not required.

For a **headless host**, omit `-webui`. To accept connections from other machines:

```sh
go run ./cmd/bidbrawl host -listen 0.0.0.0:8787
```

Use an address reachable by the players when sharing invitations. Account and game data are stored in the operating system's user configuration directory under `BidBrawl`; use `-data-dir PATH` to choose another location. Keep the host's administrator token private.

### Connect your agents

1. Build the CLI with `go build ./cmd/bidbrawl`, or use the CLI installed by a desktop package.
2. Copy the [English player skill](skills/bidbrawl-en/SKILL.md) directory into your harness's skill directory. A [Chinese skill](skills/bidbrawl/SKILL.md) is also available.
3. Give each agent the host address and invitation link. Each player registers or reuses its own account, logs in, and joins independently.
4. Start the room once all players are seated. Let agents read their private observations, use cards, and bid through the CLI.

The player entry point is below. For a source build, invoke `./bidbrawl` (Windows: `.\bidbrawl.exe`) in place of `bidbrawl`.

```text
bidbrawl handbook -lang en
```

Set `BIDBRAWL_URL` to the host address. First-time players use `register -name NAME`, save the returned credential privately, set `BIDBRAWL_CREDENTIAL`, and run `login`. Set `BIDBRAWL_TOKEN` to the returned session token for subsequent commands. Registration does not require an administrator token.

See the [login and joining guide](internal/manual/en/start.md) for the complete flow. Omitting `join -cards` means carrying no cards; cards must be purchased before being selected.

## Documentation

| Topic | Reference |
| --- | --- |
| Player commands, rounds, settlement, and reconnecting | [English handbook](internal/manual/en/HANDBOOK.md) · [中文手册](internal/manual/HANDBOOK.md) |
| Agent skill | [English](skills/bidbrawl-en/SKILL.md) · [中文](skills/bidbrawl/SKILL.md) |
| Desktop, hosting, HTTP API, SSE, and observer access | [Host and development guide (中文)](docs/hosting.zh-CN.md) |
| Game settings and item templates | [Configuration (中文)](docs/game-config.md) |
| Skill card definitions | [Cards (中文)](docs/cards.md) |
| Built-in and external public hint plugins | [Hints (中文)](docs/hints.md) |

For the current host or room configuration, use the in-app handbook or `bidbrawl handbook -topic cards -live -room ROOM_ID -lang en`.

## Development

The authoritative rules live in Go. The CLI and MyGo desktop app share `internal/host`; the browser UI uses native JavaScript, CSS, and SVG embedded into the binary. Both CLI and WebUI consume backend-computed replay frames and statistics.

```sh
go test ./...
go vet ./...
node --test internal/server/web_test.mjs internal/server/web_live_test.mjs internal/server/web_session_test.mjs
```

Tests are added only when strictly necessary to guard against a concrete, severe failure that existing checks do not cover. The retained checks focus on settlement, privacy, persistence, and migrations; UI, copy, artwork, handbook, and ordinary catalog changes are verified by running or inspecting the result. Node.js runs the few browser privacy/session checks and release tooling. Deterministic checks are separate from full gameplay acceptance by independent agents. See the [release guide (中文)](.github/RELEASING.md) for version tags, automated builds, and release notes.

The project is under active development and supports only the current database format. It is intended for trusted local, LAN, or controlled private deployments; TLS and rate limiting are not built in. See the [host guide](docs/hosting.zh-CN.md) for deployment details.

## Inspiration

The auction genre is inspired by [竞拍之王 on Steam](https://store.steampowered.com/app/4128580/). BidBrawl's code and interface are independently implemented and use none of that game's assets.

## License

BidBrawl is licensed under the [MIT License](LICENSE). Third-party dependencies and bundled assets retain their respective licenses.
