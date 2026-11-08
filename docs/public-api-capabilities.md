# Public API capabilities (verified vs unresolved)

Official base: `https://ps99.biggamesapi.io`. Public requests never carry account bearer tokens.

## Verified in this client

| Path | Use |
| --- | --- |
| `GET /api/collections` | Discover collection names |
| `GET /api/collection/:name` | Fetch one collection document |
| `GET /api/rap`, `GET /api/exists` | Market RAP / existence |
| `GET /v1/releases`, `/v1/releases/:version`, `/v1/items/changes` | Release feed (see `publicintel`) |
| `GET /v1/leagues/*`, `/v1/clans/*` | Competition samples |

## Verified `GET /api/collections` names (2026-11-08 live)

Achievements, Boosts, Booths, Boxes, Buffs, CardItems, Charms, **Currency** (singular; `Currencies` returns 400 “Collection query does not exist”), Eggs, Enchants, FishingRods, Fruits, GuildBattles, Hoverboards, Lootboxes, Mastery, MiscItems, Pets, Potions, RandomEvents, Ranks, Rarity, Rebirths, SecretRooms, Seeds, Shovels, Sprinklers, Ultimates, Upgrades, WateringCans, Worlds, ZoneFlags, Zones, Merchants, XPPotions.

HTTP 200 confirmed for Eggs, Pets, Zones, Worlds, ZoneFlags, SecretRooms, Charms. Presence of a name does not imply joinable zone↔egg fields.

Live SecretRooms (3 entries) expose `RequiredZone` + `InstanceId` for Chest Roulette, Crystal Key Castle, Treehouse Merchant — **none** name “Secret Room Egg”. Do not invent an egg placement from the egg display name alone.

## Unresolved

- Exact Secret Room Egg → zone/room/temporary placement
- Whether pet table weights are percents, relative weights, or gated rares
- Magnitudes of `oddsFFlags` / event multipliers (names ≠ numbers)
- Full historic egg configs from change records alone
- Live “current odds” without controlled observation

Unsupported statements are not verified. db.biggames.io is discovery only, not provenance.
