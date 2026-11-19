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

Clan points and currencies share one prior `*-{prefix}-summary.json` per window (5 minutes, 30 minutes, 1 hour, the whole session, and previous). The finite baseline is the summary in the same `sessionId` and the same account whose age is closest to the requested window. `previous` is the latest compatible summary in that session, with no age limit. The session baseline is the earliest summary with that id for the same account. A finite window counts only when the absolute error is within a quarter of that window, and at least 60 seconds. An 18-minute gap is not reported as a 5-minute rate. `netPerHour` is the exact value of `delta / elapsedSeconds * 3600`. `netPerMinute` is `delta / elapsedSeconds * 60`. A non-integral rate also includes `netPerHourApprox` or `netPerMinuteApprox`, a decimal approximation. Stored balances and deltas stay exact integers. Currency rates and the eggs-opened rate are observation estimates: each changed currency, and eggs when both snapshots have the statistic, sets `rateBasis` to `observation_interval` and `estimated` to true, because those figures come from cached player snapshots rather than a continuous counter. Those rates are omitted when an exact decrease, or a missing observation, lies inside the window. The raw delta is still stored. Clan place is not part of that rule. Clan-point and clan-place rates stay on the uncached public reads and are not marked that way. Clan place is stored beside clan points on each window. It uses the same baseline, and it is unavailable when the place is missing, zero, or negative, or when the battle id or clan tag differs. A missing place is not treated as zero. By default the session id is copied from the latest prior summary for that account, so a restart continues the session. `--new-session` starts a new id. `--session-id` sets one explicitly. The two flags cannot be combined. A clan-point delta is unavailable when the clan tag or the active battle id differs from the baseline. A player-contribution delta requires the same PS99 user id. A currency missing on one side is `newly observed` or `no longer returned`, not zero. `current.currenciesKnown` records whether the currency list was actually returned. An unknown list is not treated as empty, and numeric currency deltas are omitted with a reason. Absent amounts stay unresolved.

`--user-id` selects the player for contribution and rank. Otherwise `profiles.<alias>.ps99UserId` in `config.json` is used. When neither is set, clan totals are kept and contribution and rank are omitted. `--clan-tag` overrides tag discovery. When the flag is omitted, the tag is taken from the profile or extended profile, then from `profiles.<alias>.ps99ClanTag`. Snapshot reads of the clan and active battle bypass the local public cache. A failed clan or active-battle request is a warning and does not fail the snapshot; `--require-clan` makes that failure fatal. The summary file itself is still required. There is no separate historic clan-battle route; `Battles` on the clan document is that record. `clans battle` remains the sampled `GET /v1/clans/battles/:battleId` command.

Same-second runs get a `-0001` collision suffix. The manifest lists payload files with exact byte sizes and SHA-256 digests (it does not self-reference its own digest). Optional `--note` stores experiment context.

JSON and CSV files are the canonical snapshot. The default terminal output is a human summary of that record: account, activity, clan, inventory, the next eligible refresh time, data-source status, warnings, and file names. The refresh line uses `nextRefreshEligibleAt` from the refresh object already returned with the account views, converted to the local timezone. If that field is absent, the line is `n/a` and says why. It does not add API calls. `--output json` or `--json` prints a parseable JSON summary on stdout; progress stays on stderr. `--show-all-currencies` lists every currency in the human summary. The files always keep the full set. The header `Since previous` is the wall-clock `windows.previous.elapsedSeconds` when that window is valid, and `n/a` otherwise. It is not the session duration and it is not derived from file names. `Playtime` is the upstream profile counter. Its delta is not forced to equal that wall-clock interval, and the difference is not labeled downtime. When the comparison window is `previous`, activity and currency lines omit a repeated "since previous". Other windows still name themselves.

Eggs and each shown currency put the change, per-hour figure, and per-minute figure on one line when rate continuity is intact. Hatch rate repeats those two rates. An unchanged shown currency whose baseline balance is known and whose history has no break shows a real zero, including `~0/h`. A currency or egg count missing on one side stays `n/a`. Terminal rates are rounded for display, to about two or three significant digits, from the exact rational rate. A nonzero rate is never shown as `~0`. Stored `netPerHour`, `netPerMinute`, and the `Approx` fields are unchanged by that rounding.

A production rate is an observed net balance-growth rate, not a gross earning rate. Snapshot history cannot see a spend that is hidden inside a net increase. If an exact currency balance is lower than the immediately previous exact balance, that observation is a balance decrease: the raw delta is kept, and no production rate is emitted for that step (`rateStatus` `reset`, `rateReason` `observed balance decrease`). The human line can say `spend/reset detected`. That wording means the balance fell; it does not mean the decrease equals the amount spent, and it does not mean a transaction was observed. Any later window whose baseline is before that snapshot and whose current time is at or after it has no production rate (`rateReason` `balance decrease occurred within interval`), even when the raw net delta is positive. The reset is per currency. A gap, a non-exact amount, or a currency that disappears and reappears withholds the rate with `rate continuity unavailable across missing observation` and does not invent a spend. `rateResetAt` is the continuity boundary when one can be named. Eggs Opened uses the same rule, with `counter decreased` instead of a balance decrease, because a falling hatch counter is a reset rather than spending. Clan place is a rank, not a cumulative counter, so a place change is not a reset.

Clan place in JSON stays `current - baseline`. Lower place numbers are better, so the human line shows the opposite as places gained or lost, and the places-per-hour figure uses that same presentation sign. Clan points and contribution keep their raw sign and show `netPerHour` for the selected window. `Share of clan gain` is presentation-only: user contribution delta divided by clan point delta, when the battle, clan, and baseline match, the clan delta is positive, and the contribution delta is present and not negative. It is the share of the point gain during the interval, not a share of the clan's total points. Member count is shown from the clan document already fetched. `MemberCapacity` is shown beside it only when that field is present. Contributor count is the contribution list length. Battle remaining is the active battle finish time minus the snapshot time, or `ended` when that time has passed. It is not the API refresh time.

A ZIP named `<stamp>-<prefix>.zip` is written beside the snapshot by default. `--no-zip` skips it. `--zip-path` chooses another destination in the output directory, or an absolute path, and refuses to overwrite a file that already exists. It cannot be combined with `--no-zip`. The generated name is chosen by the same collision allocation as the snapshot files, so a previous `*-ps99.zip` shifts the next snapshot instead of being replaced. The ZIP is built from the manifest's artifact list after those files are finalized. It does not include itself, tokens, API keys, cookies, credential files, or local config. It does include the account telemetry and inventory that the snapshot wrote. Within the current ZIP implementation, member order and archive metadata are normalized, so a change to a source file's modification time or ordinary permissions does not change the bundle. That does not promise identical bytes under a future Go Deflate implementation. The terminal reports the bundle name, size, and a shortened SHA-256. The full digest is in `--json` output. A ZIP failure does not delete the snapshot files. The manifest does not record the ZIP's own hash.

Each snapshot also writes `<stamp>-<prefix>-inventory-diff.json`. That file is the exact inventory comparison against the immediately previous compatible snapshot: the same account, profile, and session already chosen for `windows.previous`. It is not the lexically previous `inv.json`. The previous inventory is opened through that summary's manifest, inside the snapshot directory, and symlinks are refused. If that payload is missing, malformed, or from another account, the new snapshot still succeeds. The diff status is `unavailable`, the reason is stored, and no zero delta is invented. Current pet totals are still recorded on `summary.json` when the new inventory parsed. Those totals are exact integer text. Older summaries that lack them still load, and one new snapshot can still diff the prior inventory file.

An inventory identity is the item class, the collection, and the canonical raw data. `_am` and `count` are quantities, so they are removed before the identity is formed. `stackKey` embeds that quantity and is not an identity. Two observations of the same stack, including JSON keys in a different order, are one identity: 100 then 125 is a net change of +25, not a removal plus an addition. Rows that normalize to the same identity have their quantities summed first. `createdAt` and any other raw field that is not quantity or market metadata stays in the identity, so two pets with the same id and different `createdAt` values remain distinct. The species list rolls those identities up by pet id. Market fields (`rap`, `exists`, `icon`, `displayName`, `rarity`, and the same kind of metadata) do not create a quantity change.

Pet variants reuse the existing rule: `pt` 1 is Golden, `pt` 2 is Rainbow, and `sh` true is Shiny. Golden and Shiny can describe the same pet, and Rainbow and Shiny can too. Those counts are not exclusive. A Golden Shiny pet counts once in the pet quantity, once in Golden, and once in Shiny. Golden + Rainbow + Shiny is not the pet total. Huge is counted from an explicit `huge` boolean already on the row, by quantity. A name that starts with "Huge" is not that classification. When the boolean is absent, Huge is `n/a`, not zero.

The human Inventory block shows those totals and the net change since the previous snapshot. It then lists the largest pet species changes and the largest non-pet item changes. Currency rows stay in the JSON file, because the account section already shows currency, and they are left out of the Inventory changes list. The default list is five species and five other items, then `... N more changed entries`. `--verbose` shows the rest and the pet-versus-eggs diagnostic. The wording is net inventory change: added to inventory, removed from inventory, or a net change. The CLI does not infer a hatch, sale, trade, fuse, transfer, or deletion. A pet quantity decrease is a real negative delta. It is not a spend or a counter reset, and it has no production rate. Eggs Opened remains the hatch counter. When both the pet delta and Eggs Opened exist for that same previous interval, the file stores both and their difference. A mismatch is a diagnostic, not an error.

`account snapshot diff --from OLD_MANIFEST --to NEW_MANIFEST` is fully offline: no auth, no network, no quota. It uses the same inventory comparison, including aggregate pet deltas, variant deltas, species changes, every changed identity, additions, removals, and warnings. It also diffs Statistics, PurchasedEggs, PetHatchCount, and currencies. Optional `--egg` / `--pet` / `--egg-price` / `--currency` / `--future-eggs` / `--batch-size` enable observed hatch-rate, reconciliation, and cost/timing analysis. Observed rates are not official game odds.

Export examples:

```bash
bg-cli account quota --profile main --pretty
bg-cli account snapshot --profile main --dir . --prefix ps99 --note "Lucky Eggs VII"
bg-cli account snapshot --profile main --dir . --prefix ps99
bg-cli account snapshot --profile main --dir . --prefix ps99 --no-zip
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
