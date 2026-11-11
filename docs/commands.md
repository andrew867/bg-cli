# Command reference

Global form:

```text
bg-cli [global flags] <group> <command> [arguments] [command flags]
```

## Global flags

| Flag | Default | Description |
| --- | --- | --- |
| `--profile` | configured default | Local authorization profile for account commands |
| `--output` | `json` | `json`, `table`, `raw`, or `csv` |
| `--pretty` | false | Indent JSON |
| `--timeout` | `15s` | HTTP timeout between 1s and 60s |
| `--quiet` | false | Suppress non-error stderr notices |
| `--no-cache` | false | Bypass the public response cache |
| `--excel` | false | CSV only: write a UTF-8 BOM so Excel detects encoding |
| `--version` / `version` | n/a | Print version |
| `--help` | n/a | Print help |

Stdout carries data. Diagnostics go to stderr. Secrets are never printed.

`raw` prints exact upstream response bytes for data endpoints and is unavailable for `auth` commands.

`csv` writes spreadsheet rows for `account inventory`, `account currencies`, and `account quota`. Counts and `_am` / `amount` values keep exact JSON number text (no float64 rounding). Inventory CSV includes pet variant columns (`variant`, `golden`, `rainbow`, `shiny`, `pt`, `sh`, `stackKey`). Other commands reject `--output csv`.

## Meta

| Command | Description |
| --- | --- |
| `version` | Print version |
| `doctor` | Check config dir, credential store, and granted scopes vs defaults |
| `completion bash\|zsh\|fish\|powershell` | Generate shell completion scripts |

## auth

| Command | Description |
| --- | --- |
| `auth login` | OAuth login (all account scopes by default; `--scopes` to opt out); stores token in the OS credential store |
| `auth status` | Local metadata; `--check` optionally probes the API |
| `auth profiles` | List local profiles |
| `auth logout` | Delete local token and metadata |

## account

Authenticated `GET` calls to `/v1/account/*`. Each command requires its matching granted scope.

| Command | Endpoint | Default scope |
| --- | --- | --- |
| `account inventory` | `/v1/account/inventory` | inventory:read |
| `account profile` | `/v1/account/profile` | profile:read |
| `account currencies` | `/v1/account/profile` | profile:read |
| `account quota` | `/v1/account/profile` | profile:read |
| `account snapshot` | inventory + profile + extendedProfile | inventory + profile + extendedProfile |
| `account loadout analyze\|optimize\|recommend` | offline `--manifest`/`--snapshot` or `/v1/account/inventory` | inventory:read when live |
| `account loadout calibrate` | two offline manifests | none (offline) |
| `account loadout objectives` | n/a | none |
| `account power inspect` | offline snapshot + optional public collections/annotations | inventory:read when live |
| `account experiment compare` | two offline manifests | none (offline) |
| `account forge observe\|analyze\|optimize\|ledger` | local observation/ledger JSON (+ optional snapshot) | none (offline) |
| `account snapshot --all-views` / `--include` | selected authenticated views | matching scopes |
| `account snapshot validate` | local manifest | none (offline) |
| `account index\|transactions\|hatch\|progression\|portfolio\|reconcile\|audit` | offline manifests / RAP / catalogs | none (offline) |
| `releases\|leagues\|clans` | public API | none |
| `account extended-profile` | `/v1/account/extendedProfile` | extendedProfile:read |
| `account item-index` | `/v1/account/itemIndex` | itemIndex:read |
| `account trades` | `/v1/account/trades` | trades:read |
| `account booth` | `/v1/account/booth` | booth:read |
| `account mail` | `/v1/account/mail` | mail:read |

Common flags:

| Flag | Description |
| --- | --- |
| `--refresh` | Append `refresh=true`; may consume quota |
| `--yes` | Confirm `--refresh` when stdin is not a TTY |

Inventory filters: `--class`, `--id`, `--equipped-only` (exact, case-insensitive).  
Inventory RAP join: `--join-rap` (public `/api/rap` by class+id; adds CSV columns `marketRap`, `marketRapMatched`).  
Currencies filter: `--id`.

`account quota` returns refresh used/limit/`resetsAt` plus rate-limit remaining and `Retry-After` without printing the profile body.

`account snapshot` writes a dated five-file dump (plus manifest) into `--dir` (default `.`):

```text
{yyyy-MM-dd-HHmmss}-{prefix}-inv.json
{yyyy-MM-dd-HHmmss}-{prefix}-prof.json
{yyyy-MM-dd-HHmmss}-{prefix}-extprof.json
{yyyy-MM-dd-HHmmss}-{prefix}-inventory.csv
{yyyy-MM-dd-HHmmss}-{prefix}-currencies.csv
{yyyy-MM-dd-HHmmss}-{prefix}-manifest.json
```

Same-second runs get a `-0001` collision suffix. The manifest lists payload files with exact byte sizes and SHA-256 digests (it does not self-reference its own digest). Optional `--note` stores experiment context. Stdout gets a small summary only.

`account snapshot diff --from OLD_MANIFEST --to NEW_MANIFEST` is fully offline: no auth, no network, no quota. It diffs Statistics, PurchasedEggs, PetHatchCount, Pet inventory variants, and currencies; optional `--egg` / `--pet` / `--egg-price` / `--currency` / `--future-eggs` / `--batch-size` enable observed hatch-rate, reconciliation, and cost/timing analysis. Observed rates are not official game odds.

Export examples:

```bash
bg-cli account quota --profile main --pretty
bg-cli account snapshot --profile main --dir . --prefix ps99 --note "Lucky Eggs VII"
bg-cli account snapshot --profile main --dir D:\snaps --join-rap --excel
bg-cli account snapshot diff --from old-manifest.json --to new-manifest.json --egg "Masquerade Egg" --pet "Butler Penguin" --egg-price 50000 --currency HalloweenGamesCoins --future-eggs 10000 --pretty
bg-cli account inventory --profile main --output csv --join-rap --excel > inventory.csv
bg-cli account currencies --profile main --output csv --excel > currencies.csv
```

Excel note: `--excel` writes a UTF-8 BOM so Excel detects encoding, but Excel may still round integers beyond its numeric precision. Prefer CSV text columns or Power Query for exact large integers.

### account loadout

Read-only loadout analysis. Prefers `--manifest` (offline). Never treats displayed pet power as post-boost damage. Currency objectives are per currency ID (Coins, Diamonds, HalloweenGamesCoins, …) and never merged into one score. Unknown enchant/world multipliers stay unresolved; candidates include uncertainty and a suggested controlled snapshot test.

```bash
bg-cli account loadout objectives --pretty
bg-cli account loadout analyze --manifest ./snap-manifest.json --world halloween-games --objective max-pet-damage --pretty
bg-cli account loadout optimize --manifest ./snap-manifest.json --world halloween-games --objective halloween-coins-per-min --pretty
bg-cli account loadout calibrate --from OLD-manifest.json --to NEW-manifest.json --world halloween-games --currency HalloweenGamesCoins --pretty
```

Optional `--registry path.json` loads a versioned effect registry (schema 1.0). Built-in defaults keep unverified magnitudes as `unknown`.

### account power / experiment

```bash
bg-cli account power inspect --snapshot ./manifest.json --world halloween-games \
  --annotations ./annotations.json --pets-collection ./Pets.json --charms-collection ./Charms.json --pretty

bg-cli account loadout recommend --snapshot ./manifest.json --objective currency-rate \
  --currency HalloweenGamesCoins --world halloween-games --event-team-limit 15 --pretty

bg-cli account experiment compare --from BEFORE-manifest.json --to AFTER-manifest.json \
  --currency HalloweenGamesCoins --world halloween-games --design A/B --pretty
```

See `docs/research/pet-power-and-events.md` for verified vs presentation-only fields. Halloween `indexDesc` extras and the 31% Huge rule are **not** treated as proven server formulas.

### account forge

Offline Forge tooling. Observations preserve exact UI percentage strings (including `<0.001%` censored bounds). Butler-only curves may be fit as capped-linear models of **displayed** probabilities. Mixed recipes stay separate. Optimize never forges pets.

```bash
bg-cli account forge observe --file ./forge-observations.json --pretty
bg-cli account forge analyze --observations ./forge-observations.json --pretty
bg-cli account forge optimize --observations ./forge-observations.json --snapshot ./manifest.json --objective min-butlers --pretty
bg-cli account forge ledger --file ./forge-attempts.json --pretty
```

See `docs/research/halloween-forge.md` and `docs/operator-guide-analysis.md`.

Account response bodies are never cached to disk.

## player

| Command | Description |
| --- | --- |
| `player views <slug>` | Show account metadata and public-view availability |
| `player show <slug> --include <views>` | Fetch selected opted-in views |

Authentication does not unlock another player's private views.

## leaderboard

| Command | Description |
| --- | --- |
| `leaderboard list` | List boards; optional `--state`, `--id` |
| `leaderboard show <board-id>` | Page rows; `--source live\|final`, `--page`, `--page-size` |
| `leaderboard player <board-id> <user-id>` | One retained row |

A missing row maps to `not_ranked_or_not_retained` (exit 3). That outcome is not proof of a zero score.

## collection

| Command | Description |
| --- | --- |
| `collection list` | List collection names |
| `collection show <name>` | Fetch one collection document |

## account tiers

Offline join of inventory `id` + `rawData.tn` (and equipped `level` when > 0) to official public `Tiers[].DisplayName`. Never invents equipped loadouts from ownership. Does not spend account refresh quota.

```bash
bg-cli account tiers resolve --inventory ./inv.json --live-public --pretty
bg-cli account tiers resolve --manifest ./2026-11-08-145843-ps99-manifest.json \
  --enchants ./Enchants.json --potions ./Potions.json --charms ./Charms.json --pretty
```

## public (catalog observatory)

Offline-first public catalog. Sync never attaches account bearer tokens and does not spend inventory refresh quota.

| Command | Description |
| --- | --- |
| `public discover` | List `/api/collections` names (or `--fixture`) |
| `public sync` | Fetch curated or `--collections` / `--all` into `--dir` |
| `public collections` | List collections in the local catalog |
| `public status` | Coverage manifest |
| `public inspect COLLECTION [NAME]` | Inspect local snapshot / egg |
| `public diff --from A --to B` | Semantic collection JSON diff |
| `public export` | Export coverage + collection list |
| `public changes` | Alias to `releases changes` (official item-change feed) |

## egg / graph

| Command | Description |
| --- | --- |
| `egg show NAME` | Parse egg document from local Eggs JSON |
| `egg odds NAME --model …` | Hypothetical odds (`direct-percent`, `normalized-weight`, `independent-rare-gate`) |
| `egg flags NAME` | Named odds/feature flags without inventing magnitudes |
| `egg cost NAME` | Nominal batch affordability from balance strings |
| `graph unresolved` | Heuristic / unresolved entity joins |

See `docs/public-data-cli.md`, `docs/egg-probability-models.md`, `docs/zone-egg-mapping.md`.

## market

| Command | Description |
| --- | --- |
| `market rap` | Public RAP dataset; optional `--category`, `--id` |
| `market exists` | Public exists tallies; optional `--category`, `--id` |
