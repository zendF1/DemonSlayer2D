# DemonSlayer2D server

Go game server for DemonSlayer2D. The Godot client lives in a separate repository and generates its protocol code from [`proto/`](proto/) and reads maps from [`maps/`](maps/).

The protocol version is 11 (`gateway.ProtocolVersion`). A login with any other
version fails with `ERROR_CODE_VERSION_MISMATCH`, so bump it together with the client.

Design documents: [game design](docs/game-design.md), [Phase 0 server spec](docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md), [Phase 2 skills spec](docs/superpowers/specs/2026-11-20-giai-doan-2-ky-nang-design.md), [Phase 3a bag, drops and equipment spec](docs/superpowers/specs/2026-11-20-giai-doan-3a-tui-do-roi-do-design.md), [Phase 3b enhancement spec](docs/superpowers/specs/2026-11-20-giai-doan-3b-cuong-hoa-design.md), [Phase 3c resonance, gems and refining spec](docs/superpowers/specs/2026-11-20-giai-doan-3c-tuong-sinh-kham-ngoc-tay-luyen-design.md), [Phase 4a maps, portals and zones spec](docs/superpowers/specs/2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md), [Phase 4b NPCs, shops and potions spec](docs/superpowers/specs/2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md), [Phase 4c stash, Lượng and respawn in place spec](docs/superpowers/specs/2026-11-20-giai-doan-4c-kho-luong-hoi-sinh-design.md).

## Requirements

- Go 1.25+
- Docker (for Postgres)

Code generators (`buf`, `protoc-gen-go`) are Go tool dependencies; nothing else to install.

## Run

```bash
cp .env.example .env   # optional: local settings, git-ignored
make db-up   # Postgres 17 on localhost:5433 (databases ds and ds_test)
make run     # server on :8080 — WebSocket at /ws, health at /healthz
```

Configuration is read from environment variables. `make run` and `make test-db`
also load them from `.env` if it exists (copy [`.env.example`](.env.example)); the
server itself never reads the file, so in production set real environment variables.
A value in `.env` beats one exported in your shell; pass `make run VAR=value` to
override it for one run.

| Variable | Default | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | (required; `make run` sets it) | Postgres connection string |
| `LISTEN_ADDR` | `:8080` | HTTP/WebSocket address |
| `MAPS_DIR` | `./maps` | Map files |
| `DATA_DIR` | `./data` | Game data (`monsters.json`, `skills.json`, `exp.json`, `items.json`) |
| `START_MAP` | `kumotori_1` | Map for new characters |
| `TICK_RATE` | `10` | Zone ticks per second, 1–100 |
| `MOVE_SPEED_TILES` | `4` | Tiles per second at speed factor 1, above 0 and at most 100 |
| `AOI_CELL_SIZE` | `10` | Visibility grid cell size in tiles |
| `ZONE_CAPACITY` | `50` | Players per zone |
| `SAVE_INTERVAL` | `60s` | Periodic position save |
| `REGISTER_PER_HOUR` | `5` | Registrations per IP per hour |
| `TRUSTED_PROXIES` | _(empty)_ | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` is believed, e.g. `127.0.0.1,10.0.0.0/8` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Behind nginx or Caddy, set `TRUSTED_PROXIES` to the proxy's address, otherwise every
player shares the proxy's IP and its rate limits. Caddy sends `X-Forwarded-For` by
default; nginx needs `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.
Only list proxies you run: the header is read only when the connection comes from one.

If something else already uses port 8080, pass another address:
`make run LISTEN_ADDR=:8099` (then use `-url ws://localhost:8099/ws` below).

## Try it with the bot client

```bash
go run ./cmd/botclient register -u tanjiro -p password123
go run ./cmd/botclient create   -u tanjiro -p password123 -slot 1 -name Tanjiro -class viem
go run ./cmd/botclient chars    -u tanjiro -p password123
go run ./cmd/botclient play     -u tanjiro -p password123 -id 1 -dir 1,0 -for 10s
```

Fight mode registers a new account (unless `-u`/`-p` are given), creates a
character, walks out of the safe zone and kills N monsters, logging the level and
EXP after each kill. It exits 0 after N kills and 1 after `-timeout` (default 2m):

```bash
go run ./cmd/botclient -fight 3                      # against ws://localhost:8080/ws
go run ./cmd/botclient -fight 3 -url ws://localhost:8099/ws -timeout 1m
```

With no monster in view it walks towards `-seek` (default `22.5,10.5`, the nest
of region r1 on `kumotori_1`). Each run without `-u` counts against `REGISTER_PER_HOUR`.

Load test (lift the registration limit on the server first, e.g. `REGISTER_PER_HOUR=100000`):

```bash
go run ./cmd/botclient load -n 1000 -for 6m -ramp 150ms
```

Keep `-ramp` generous on a first run: every bot registers and logs in, and each
of those hashes a password with argon2id (64 MiB, ~76 ms), so a 1,000-bot burst
started in a few seconds exhausts the 10-second request deadline. `-ramp 150ms`
spreads the sign-ups over 150 s and leaves the rest of the run measuring the
game loop. Measured on an 8-core dev machine with 1,000 bots in world across 20
zones: tick p99 ≈ 9 ms, no dropped clients.

## Dev characters

`devchar` changes an account's first character (lowest slot), for development
only. It reads `DATABASE_URL` and `MAPS_DIR` (default `./maps`):

```bash
export DATABASE_URL='postgres://ds:ds@localhost:5433/ds?sslmode=disable'
go run ./cmd/devchar -u tanjiro -level 90
go run ./cmd/devchar -u tanjiro -map ubuyashiki -x 20.5 -y 16.5          # the town's default spawn
go run ./cmd/devchar -u tanjiro -level 16 -map fujikasane_1 -x 6.5 -y 24.5  # both: Fujikasane's camp
```

`-level` sets the level (1–90) and EXP to 0, refunds every spent potential
point (free points become 10 × (level − 1)), sets the skill points to
(level − 1) minus the levels of the learned skills, or drops every skill and
sets them to level − 1 when that is negative, and fills HP and MP. It prints
the character's name, level, free potential points and skill points.

`-map` with `-x` and `-y` (in tiles, so 20.5 is the centre of column 20) moves
the character to that map and position. An unknown map or a
tile that is not walkable is refused before anything is written, `-level`
included. The spawn points of each map, such as the arrival point of every
portal, are listed in its file under `spawn_points`.

The character must be offline. The server does not record who is online, so
`devchar` cannot check; a running server's next save of the character
overwrites the change.

## Dev items

`devitem` gives an account's first character (lowest slot) an item or Xu, for
development only. It reads `DATABASE_URL` and `DATA_DIR` (default `./data`):

```bash
export DATABASE_URL='postgres://ds:ds@localhost:5433/ds?sslmode=disable'
go run ./cmd/devitem -u tanjiro -code eq50_armor -quality 2   # a Vàng Áo Xích Cương
go run ./cmd/devitem -u tanjiro -code eq10_weapon_viem -enhance 9   # a +9 sword
go run ./cmd/devitem -u tanjiro -code stone_enhance -count 100
go run ./cmd/devitem -u tanjiro -code charm_protect -count 20      # protection charms
go run ./cmd/devitem -u tanjiro -code eq20_armor -quality 2 -holes 3   # three empty holes
go run ./cmd/devitem -u tanjiro -code gem_hp_3 -count 5           # Ngọc Sinh Lực cấp 3
go run ./cmd/devitem -u tanjiro -xu 100000
go run ./cmd/devitem -u tanjiro -luong 5                           # 5 Lượng for the account
```

An equipment code adds one piece into the first empty bag slot, with quality
`-quality` (0 Trắng, 1 Xanh Dương, 2 Vàng, 3 Cam, 4 Đỏ; default 0), enhance
level `-enhance` (0 to 15, default 0; equipment only), `-holes` empty holes
(0 to 3, default 0; equipment only) and lines and element rolled at random.
Stackable codes are the stones (`stone_enhance`, `stone_refine`), the
protection charm `charm_protect`, which only the general store will sell from
phase 4, and the 15 gems `gem_attack_N`, `gem_hp_N` and `gem_defense_N` (level
N 1 to 5; a level N gem fits tier 10N or higher gear), which events and quests
will hand out from phase 6. A stackable code adds `-count` (default 1), topping
up existing stacks first (999 per stack). `-xu` adds Xu and can be used alone
or with `-code`. An unknown code or a full bag is an error, and nothing is
written. To try the client's Khảm ngọc and Tẩy luyện tabs, give a Vàng or
better piece, some gems that fit its tier, `stone_refine` and Xu: punching,
socketing, unsocketing and refining work only in a safe zone, like enhancing. It prints the character's name, the item added and its bag slots,
and the Xu it now has. Like `devchar`, it needs the character offline.

`-luong N` adds Lượng to the **account** (Lượng and the account stash belong to
the account, not a character), alone or with the other flags; it needs no
character and prints the account's new total. There is no top-up gateway yet,
so this is the only source of Lượng: it buys protection charms in the Lượng
shop and pays for respawning in place. The stash itself is opened at the
Thủ kho (storage keeper) of each camp and town: deposit and withdraw whole
slots, expand from 30 up to 120 slots for Xu.

Because the zone of an online character holds the account's Lượng and stash
and saves them with the character, **every character of the account must be
offline** when you use `-luong` (or any `devitem` flag); the server does not
record who is online, so `devitem` cannot check, and the next save of an online
character overwrites the change. The tool prints a warning to that effect.

## Game data

Maps live in [`maps/`](maps/) (collision, spawn points, safe zones, rest points,
monster regions, portals and `min_zones`). They are generated by
[`tools/mapgen.py`](tools/mapgen.py), which declares every map as rectangles and
checks the layout rules; edit the script, never the JSON:

```bash
python3 tools/mapgen.py           # summary: size, walkable tiles, monsters, portals
python3 tools/mapgen.py --check   # exit 1 and name the maps that differ from the script
python3 tools/mapgen.py --write   # rewrite maps/*.json
```

| Map | Name | Type | Levels | Zones kept open |
| --- | --- | --- | --- | --- |
| `kumotori_1` | Kumotori – Đường núi tuyết | training | 1–5 | 2 |
| `ubuyashiki` | Tổng hành dinh Ubuyashiki | town (all safe, no monsters) | — | 1 |
| `sagiri_1` | Sagiri – Chân núi, đường đến ngôi đền | training | 6–10 | 2 |
| `sagiri_2` | Sagiri – Rừng sương mù | training | 11–15 | 2 |
| `fujikasane_1` | Fujikasane – Rừng tuyển chọn | training | 16–20 | 2 |

New characters start in `kumotori_1` (`START_MAP`). Portals chain the maps
Kumotori ↔ Ubuyashiki ↔ Sagiri 1 ↔ Sagiri 2 ↔ Fujikasane: a portal is a
1 × 4 tile strip by the west or east wall named `to_<map>`, and it leads to the
spawn point `from_<map>` of the next map. The client sends `UsePortalReq` when
the character steps in; the server accepts it within 1 tile of the strip and
moves the character with its HP, MP, items, Xu and cooldowns, into the zone with
the same number when it is open and has room, otherwise the first zone with
room. `ChangeZoneReq` moves to another running zone of the same map in place
(not while dead, 10 s apart), and `ZoneListReq` lists the running zones with
their player counts. Each map keeps `min_zones` zones open; zones above that
open when every zone is full and close after 5 minutes empty. Death respawns at
the nearest respawn point of the same map.

The files in [`data/`](data/) are generated too:

| File | Script | Contents |
| --- | --- | --- |
| [`data/monsters.json`](data/monsters.json) | [`monster_stats.py`](docs/balance/monster_stats.py) | Monster kinds and stats for levels 1–90, including base EXP (`HP × r(L)`) |
| [`data/skills.json`](data/skills.json) | [`skill_stats.py`](docs/balance/skill_stats.py) | The 25 class skills: shape, range, coefficients, MP, cooldowns, passives |
| [`data/exp.json`](data/exp.json) | [`exp_table.py`](docs/balance/exp_table.py) | EXP to the next level for levels 1–89 (also writes `docs/balance/exp-table.csv`) |
| [`data/items.json`](data/items.json) | [`item_stats.py`](docs/balance/item_stats.py) | Bag size, equipment slots, qualities, line rules, the 70 equipment pieces, stackables, drop tables per monster rank, the enhancement table (rates, stones, Xu), the regen milestones, the resonance table, the socket and refine costs and the 15 gems |

```bash
python3 docs/balance/monster_stats.py --write-json
python3 docs/balance/skill_stats.py --write-json
python3 docs/balance/exp_table.py --write-json --write-csv
python3 docs/balance/item_stats.py --write-json
```

The level-up table depends on monster HP, so rerun `exp_table.py` whenever you
rerun `monster_stats.py`. Both `item_stats.py` and `monster_stats.py` (which
builds monsters from level 10 against the best +0 gear) read the gear stats from
`equipment_balance.py`, so after changing those, rerun `item_stats.py`,
`monster_stats.py` and `exp_table.py`. A Go test checks that each committed JSON
matches its script's output. The Godot client keeps copies of `skills.json` and
`items.json`, refreshed by its `tools/sync_server.sh`.

## NPCs, shops and potions

NPCs are listed per map in `maps/*.json` (generated by `tools/mapgen.py`): the
town (`ubuyashiki`) has a general store, a blacksmith and a carriage; each
training map's camp has an apothecary and a carriage. An NPC action is accepted
within 3.5 tiles (`NPC_TOO_FAR` otherwise).

- **Shops** (`general`, `apothecary`) are declared in `data/items.json` (from
  `docs/balance/item_stats.py`). They sell Xu-priced stackables only: potions,
  the return scroll, and protection charms at the general store. Any shop NPC
  buys items back (`SellReq`).
- **Potions** (`UseItemReq`): 10 s cooldown per group (HP, MP), carried across
  zone changes, not saved.
- **Return scroll** (`ReturnTownReq`) and **carriage** (`TravelReq`) move the
  character through `World.Transfer`; the Xu or scroll is taken after the check
  passes.
- **Forging** (enhance, punch, socket, unsocket, refine) needs the blacksmith
  within range. `devitem` still hands out stones, charms and Xu for testing.

## Develop

```bash
make proto    # regenerate internal/pb after editing proto/
make vet
make test     # unit tests; database tests are skipped
make test-db  # all tests including Postgres and end-to-end (needs make db-up)
```
