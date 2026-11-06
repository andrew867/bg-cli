# Pet power, events, and loadout — research findings

**Retrieved:** 2026-11-06 (America/St_Johns)  
**Primary sources:**

- [BIG Games PS99 public API docs](https://github.com/BIG-Games-LLC/ps99-public-api-docs)
- `GET https://ps99.biggamesapi.io/api/collections`
- `GET /api/collection/{Pets,Charms,Enchants,Potions,Boosts,Upgrades,Worlds,Zones,Currency}`

Collection rows expose `configName`, `configData`, `dateCreated`, `dateModified`, `hashShort`.

## Publicly exposed pet-power fields

| Field | Location | Interpretation |
| --- | --- | --- |
| `cachedPower` | `Pets.configData` (number array) | Published base power index. Treated as **presentation/catalog power**, not proven post-boost DPS. Array semantics beyond index `0` are incompletely documented. |
| `huge` | boolean | Marks Huge pets. |
| `fromWorldNumber` / `fromZoneNumber` | optional | Origin world/zone metadata. |
| `rarity` / `rarityOverride` | object | Display rarity. |
| `indexDesc` | string | **Presentation** catalog blurb. |

Inspected pets (Butler Penguin, Neon Skeleton Cat, Porcelain Doll Cat, Huge Ghost/Hippokin/Froggy/Guard Elephant/Guard Rhino/Dalmatian) all carry `cachedPower` and usually `indexDesc`.

## Fields available only as descriptions

Halloween event pets include text such as:

> Deals 885,735 extra damage inside the Halloween Games.

Examples (parsed from `indexDesc` only):

| Pet | Presentation “extra damage” |
| --- | ---: |
| Butler Penguin | 885,735 |
| Porcelain Doll Cat | 295,245 |
| Neon Skeleton Cat | 98,415 |

**These are not structured numeric fields.** This tool may record them as `presentationClaim` with confidence `assumed` / source `indexDesc`. It must **not** assert that the game engine parses `indexDesc` to compute damage.

Charm and enchant `Tiers[].Desc` strings similarly describe effects; `Tiers[].Power` is present but **not uniformly a damage percent** (e.g. Royalty `Power: 1` while Desc claims 40% damage + diamonds + speed; Lightning `Power: 2.5` while Desc describes a proc).

## Fields absent from the public API

Not found in public collections / account inventory as of this research:

- Halloween Games **31% of best event pet** rule (observed only in-game tooltips, 2026-11-06)
- Per-instance Huge XP level, attached charm lists, or post-boost damage on authenticated inventory
- Halloween Games / Diamond Mountain as entries in `/api/collection/Worlds` (only World 1–4 with `WorldCurrency` Coins/TechCoins/VoidCoins/FantasyCoins)
- Event team size limits
- Authoritative Huge level→damage formula

Official inventory docs treat enchant `level = 0` as not found / unresolved — **not** tier I.

### Equipped loadout field names (official)

From [v1/account.md](https://github.com/BIG-Games-LLC/ps99-public-api-docs/blob/master/v1/account.md):

- **Pets:** `{ list, equippedCount, maxEquipped }`
- **Enchants:** `{ list, paidCount, maxEnchants, maxPaidEnchants }` — **no** `equippedCount` / `maxEquipped`

bg-cli derives enchant `equippedCount` from the sum of list-entry quantities and reads `maxEnchants` for the slot cap.

Observed in the wild: multiple equipped Butler Penguins can share one `uid`. Treat `uid` as **stack identity**, not a unique instance key; preserve each `list`/`slot` entry and its quantity.

## Normal Huge damage mechanics

Verified from public config:

- Huges are flagged with `huge: true` and publish `cachedPower` (often `[1]` for exclusives — catalog placeholder risk).

Unverified:

- Exact normal-world combination of base power, variant, level curve, charms, enchants, potions.
- Whether Equip Best (Update 100, 2026-10-31) uses the same function as inventory sort order.

## Halloween-specific rules

**In-game observation (2026-11-06):** Huge Ghost, Huge Hippokin, Huge Froggy tooltips state they deal **31% of your best event pet** in this event, and that they are normally stronger outside the event. Treat as different contexts.

Competing models implemented for comparison (none selected as truth):

- **A:** `0.31 × bestEventStatPet` (ignore level/charms)
- **B:** `0.31 × bestEventStatPet × levelFactor × variantFactor × charmDamageFactors`
- **C:** subset of modifiers apply (configurable mask)
- **D:** additional class/variant coefficients (placeholders; unknown)

Raising the strongest event stat pet can raise absolute Huge damage while the displayed **31%** stays constant.

## XP and charm behavior

Screenshot regression targets (displayed XP damage bonus):

| Level | Displayed bonus |
| ---: | ---: |
| 59 | +42.1% |
| 76 | +56.9% |
| 78 | +59.3% |

Golden Huge Dalmatian L58 ≈ +41.5% under the same community display curve (user observation).

Charms (public `Charms` collection) are classified by effect family from Desc + id:

| Charm | Family (from Desc) | Notes |
| --- | --- | --- |
| Strength | direct damage % | Power aligns with Desc % |
| Royalty | mixed (damage + diamonds + speed) | Power field is not the damage % |
| Bonus | reward proc chance | **not** raw damage |
| Lightning | triggered attack | requires proc model; not a constant multiplier |
| TNT | triggered spawn | utility/triggered |
| Agility | movement speed | not damage |
| Overload | +2 charm slots, unique | slot rules |
| Coins / Diamonds | currency-specific rewards | never auto-apply to other currency IDs |

## Currency separation

`/api/collection/Currency` exposes distinct IDs including `Coins`, `TechCoins`, `VoidCoins`, `FantasyCoins`, `Diamonds`, `HalloweenGamesCoins`, `HalloweenGamesTreats`, `TrailCoins`, `HatchWarCoins`, `LuckyCoins`, `SpaceCoins`, …  

### Currency ceilings (calibration)

`FantasyCoins` has been repeatedly observed at exactly `300000000000000000000` across distinct snapshots (e.g. 2026-11-06 12:48 and 13:59 America/St_Johns) while play continued. Excess production appears discarded at this ceiling. Calibration and snapshot diff therefore treat a flat balance at that value as **CAPPED**: `ΔFantasyCoins/Δt = 0` must not be reported as zero income. Optional `--currency-cap` overrides the ceiling for other currencies when experimentally confirmed. 

World 1–4 bind `WorldCurrency` to Coins/Tech/Void/Fantasy only. Event currencies are separate economies. A Coins enchant Desc references **Coins**, not HalloweenGamesCoins.

## Known unknowns

1. Server formula linking `cachedPower`, variants, levels, charms, enchants.
2. Whether presentation Halloween extras are additive absolute damage, multiplicative, or flavor text only.
3. 31% rule internals (Models A–D).
4. Event pet equip caps (observed teams ≠ account `maxEquipped=66`).
5. Equip Best vs UI sort identity.
6. Official Huge XP curve.

## Validation experiments needed

1. Controlled A/B with annotations for Huge levels/charms; compare Equip Best order vs model rankings.
2. Snapshot intervals measuring `HalloweenGamesCoins` vs `HalloweenGamesTreats` under damage-only vs Coins-enchant loadouts.
3. Swap best event pet and record whether Huge absolute damage changes at constant 31% display.
4. Charm ablation (Royalty on/off) in normal world vs Halloween Games.
