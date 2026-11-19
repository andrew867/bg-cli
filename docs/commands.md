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
| `auth profile set` | Set `ps99UserId`, `ps99ClanTag`, or `accountId` on a local profile |
| `auth profile unset` | Clear those stored fields |
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
| `clan show\|battle` | `GET /api/clan/{tag}`, `GET /api/activeClanBattle` | none for the public reads; `clan show` reads the account only when no `--tag` and no stored `ps99ClanTag` |
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

`account snapshot` writes a dated dump plus a manifest into `--dir` (default `.`):

```text
{yyyy-MM-dd-HHmmss}-{prefix}-inv.json
{yyyy-MM-dd-HHmmss}-{prefix}-prof.json
{yyyy-MM-dd-HHmmss}-{prefix}-extprof.json
{yyyy-MM-dd-HHmmss}-{prefix}-inventory.csv
{yyyy-MM-dd-HHmmss}-{prefix}-currencies.csv
{yyyy-MM-dd-HHmmss}-{prefix}-clan.json
{yyyy-MM-dd-HHmmss}-{prefix}-clanbattle.json
{yyyy-MM-dd-HHmmss}-{prefix}-summary.json
{yyyy-MM-dd-HHmmss}-{prefix}-manifest.json
```

`clan.json` is the full public clan document, including `Battles`, and is written when a clan tag resolves. `clanbattle.json` is written only while a battle is active (`StartTime <= now < FinishTime`, and `Enabled` is not false). Both are public GETs with no bearer token and no `refresh=true`.

`summary.json` is written on every snapshot, atomically, and listed in the manifest with its byte size and SHA-256. It is the only file later snapshots read for deltas. Currency amounts inside it come from the profile `Currency` map, stored as exact integer text. A friendly `displayName` is copied when the currency object has one; otherwise the currency id is used.

Clan points and currencies share one prior `*-{prefix}-summary.json` per window (5 minutes, 30 minutes, 1 hour, the whole session, and previous). The finite baseline is the summary in the same `sessionId` and the same account whose age is closest to the requested window. `previous` is the latest compatible summary in that session, with no age limit. The session baseline is the earliest summary with that id for the same account. A finite window counts only when the absolute error is within a quarter of that window, and at least 60 seconds. An 18-minute gap is not reported as a 5-minute rate. `netPerHour` is the exact value of `delta / elapsedSeconds * 3600`. A non-integral rate also includes `netPerHourApprox`, a decimal approximation. Stored balances and deltas stay exact integers. Currency rates are observation estimates: each changed currency sets `rateBasis` to `observation_interval` and `estimated` to true, because profile currency comes from a cached player snapshot rather than a continuous counter. Clan-point rates stay on the uncached public reads and are not marked that way. By default the session id is copied from the latest prior summary for that account, so a restart continues the session. `--new-session` starts a new id. `--session-id` sets one explicitly. The two flags cannot be combined. A clan-point delta is unavailable when the clan tag or the active battle id differs from the baseline. A player-contribution delta requires the same PS99 user id. A currency missing on one side is `newly observed` or `no longer returned`, not zero. `current.currenciesKnown` records whether the currency list was actually returned. An unknown list is not treated as empty, and numeric currency deltas are omitted with a reason. Absent amounts stay unresolved.

`--user-id` selects the player for contribution and rank. Otherwise `profiles.<alias>.ps99UserId` in `config.json` is used. When neither is set, clan totals are kept and contribution and rank are omitted. `--clan-tag` overrides tag discovery. When the flag is omitted, the tag is taken from the profile or extended profile, then from `profiles.<alias>.ps99ClanTag`. Snapshot reads of the clan and active battle bypass the local public cache. A failed clan or active-battle request is a warning and does not fail the snapshot; `--require-clan` makes that failure fatal. The summary file itself is still required. There is no separate historic clan-battle route; `Battles` on the clan document is that record. `clans battle` remains the sampled `GET /v1/clans/battles/:battleId` command.

Same-second runs get a `-0001` collision suffix. The manifest lists payload files with exact byte sizes and SHA-256 digests (it does not self-reference its own digest). Optional `--note` stores experiment context.

JSON and CSV files are the canonical snapshot. The default terminal output is a human summary of that record: account, activity, clan, inventory, data-source status, warnings, and file names. It does not add API calls. `--output json` or `--json` prints a parseable JSON summary on stdout; progress stays on stderr. `--show-all-currencies` lists every currency in the human summary. The files always keep the full set. Deltas are labeled with the window they came from (`previous`, `5m`, `30m`, `1h`, or `session`). A missing baseline is a warning. Unknown values are `n/a`, not zero.

`--zip` writes `<stamp>-<prefix>.zip` beside the snapshot. `--zip-path` chooses another destination in the output directory, or an absolute path, and refuses to overwrite a file that already exists. The generated name is chosen by the same collision allocation as the snapshot files, so a previous `*-ps99.zip` shifts the next snapshot instead of being replaced. The ZIP is built from the manifest's artifact list after those files are finalized. It does not include itself, tokens, API keys, cookies, credential files, or local config. It does include the account telemetry and inventory that the snapshot wrote. Within the current ZIP implementation, member order and archive metadata are normalized, so a change to a source file's modification time or ordinary permissions does not change the bundle. That does not promise identical bytes under a future Go Deflate implementation. The terminal reports the bundle name, size, and a shortened SHA-256. The full digest is in `--json` output. A ZIP failure does not delete the snapshot files. The manifest does not record the ZIP's own hash.

`account snapshot diff --from OLD_MANIFEST --to NEW_MANIFEST` is fully offline: no auth, no network, no quota. It diffs Statistics, PurchasedEggs, PetHatchCount, Pet inventory variants, and currencies; optional `--egg` / `--pet` / `--egg-price` / `--currency` / `--future-eggs` / `--batch-size` enable observed hatch-rate, reconciliation, and cost/timing analysis. Observed rates are not official game odds.

Export examples:

```bash
bg-cli account quota --profile main --pretty
bg-cli account snapshot --profile main --dir . --prefix ps99 --note "Lucky Eggs VII"
bg-cli account snapshot --profile main --dir . --prefix ps99 --zip
bg-cli account snapshot --profile main --dir . --prefix ps99 --output json
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

## market

| Command | Description |
| --- | --- |
| `market rap` | Public RAP dataset; optional `--category`, `--id` |
| `market exists` | Public exists tallies; optional `--category`, `--id` |

## clan

Public, read-only. No `Authorization` header and no account refresh.

| Command | Description |
| --- | --- |
| `clan show [--tag TAG]` | `GET /api/clan/{tag}`. Tag comes from `--tag`, then stored `ps99ClanTag` (no token required), then the signed-in profile or extended profile. The body includes `Battles`. |
| `clan battle` | `GET /api/activeClanBattle`. `active` is true only inside the schedule. |

```bash
bg-cli clan show --profile main
bg-cli clan show --tag SYN
bg-cli clan battle
bg-cli account snapshot --profile main --dir . --prefix ps99 --user-id 1001
bg-cli account snapshot --profile main --dir . --prefix ps99 --new-session
```

No separate historic HTTP route is called. Per-clan history stays in the clan document's `Battles` object. Sampled battle detail by id is the existing `clans battle` command.
