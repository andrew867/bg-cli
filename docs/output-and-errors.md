# Output and errors

## JSON envelope

Default stdout is a single JSON object. The schema lives at `testdata/schemas/cli-output.schema.json`.

Success objects include:

- `schemaVersion` — currently `1.0`
- `ok` — `true`
- `command` — stable command name
- `source` — provider metadata, endpoint, HTTP status, `retrievedAt`, cache flag (or `null` for local auth commands)
- `freshness` — snapshot semantics; `meaning` is always `cached player snapshot, not real time`
- `data` — command payload

Error objects set `ok` to `false` and include an `error` object with `code`, `message`, and `retryable`. Optional fields: `upstreamStatus`, `retryAfterSeconds`, `details`.

Rules:

- `retrievedAt` is observed after the response arrives (UTC RFC 3339)
- `snapshotAt` is copied only from an explicit upstream `fetchedAt`
- Refresh fields are copied without deriving timestamps
- Unknown upstream fields are tolerated

## Output modes

| Mode | Behavior |
| --- | --- |
| `json` | One schema-shaped object (default) |
| `table` | Richer terminal tables for inventory, currencies, and leaderboard list; otherwise a compact key dump |
| `raw` | Exact upstream bytes for data endpoints; rejected for `auth` |
| `csv` | Spreadsheet rows for `account inventory` / `account currencies` / `account quota` with exact numeric text; rejected elsewhere and for `auth` |

CSV inventory columns (stable order): `class`, `id`, `count`, `displayName`, `variant`, `golden`, `rainbow`, `shiny`, `pt`, `sh`, `stackKey`, `rap`, `rapApproximate`, `exists`, `rarity`, `rarityNumber`, `collection`. With `--join-rap`: also `marketRap`, `marketRapMatched`.

Variant derivation (pets): `rawData.pt=1` → Golden, `pt=2` → Rainbow, `rawData.sh=true` → Shiny; absent fields → Regular. Non-pet rows leave `variant` blank.

Inventory JSON also reports `petRowCount` / `petQuantitySum` (class `Pet` only). `quantitySum` still sums every filtered row and may include non-pet stacks such as currencies — it is not a total object count.

CSV currencies columns: `key`, `id`, `_am`, `amount` (`amount` mirrors exact `_am` for Excel).

`--excel` prefixes CSV with a UTF-8 BOM.

## Quota summary

`account quota` surfaces refresh quota (`used`, `limit`, `resetsAt`, `nextRefreshEligibleAt`, `quotaExhausted`) and HTTP rate-limit headers (`remaining`, `Retry-After`) without dumping account payloads.

## Snapshot export

`account snapshot` writes dated JSON/CSV files into `--dir` and a `*-manifest.json` that includes API `snapshotAt` (from inventory `fetchedAt` when present), per-file SHA-256 digests, `petQuantitySum`, exact currency amounts, and profile Statistics. The manifest does not claim a self-referential size/digest. Every snapshot also writes `summary.json` atomically and lists that file with its size and SHA-256. Later snapshots read prior `*-{prefix}-summary.json` files for clan-point, clan-place, egg, and currency deltas. They do not reopen raw inventory, profile, or clan payloads for those rates. Amounts stay exact integer text. `netPerHour` and `netPerMinute` are exact, and the `Approx` fields are a decimal view of the same rate. Human rate text may round that rational value; it does not replace the stored fields, and a nonzero rate is not displayed as zero. Currency and egg rates set `rateBasis` to `observation_interval` and `estimated` to true only while rate continuity holds. An exact balance decrease keeps `baseline`, `current`, and `delta`, and sets `rateStatus` to `reset` with `rateReason` `observed balance decrease`. A window that contains that decrease keeps the raw net delta and sets `rateStatus` to `unavailable` with `rateReason` `balance decrease occurred within interval`. A missing observation uses `rate continuity unavailable across missing observation`. `rateResetAt` names the continuity boundary when the history can. These fields are omitted on an ordinary earning interval, so older summaries stay readable. A net increase is not evidence of a hidden spend. Eggs Opened uses the same continuity rule with `counter decreased`. Clan place is omitted from the delta when it was not a positive place on both sides of the same battle. Its stored delta remains `current - baseline`; the human summary shows the improvement in the other direction. `Share of clan gain` is not stored. A finite window is omitted when no prior summary for the same account is close enough. `previous` still compares with the latest compatible summary. A clan delta is not taken across two battle ids or two clan tags. Snapshot clan and active-battle reads bypass the public response cache. A failure of those optional reads is recorded on the summary and does not fail the command unless `--require-clan` is set. The default stdout is a human summary, not the payload files. `--output json` or `--json` prints the summary envelope instead. A ZIP of the manifest artifacts is written by default for sharing and excludes auth material and local config. `--no-zip` skips it. The ZIP still contains account telemetry and inventory. Entry timestamps and modes are normalized so source modification times do not change the bundle bytes within this implementation. `--zip-path` refuses to overwrite an existing destination. The human summary prints the next eligible refresh time in the local timezone, or `n/a` with a reason when that time was not returned.

## Offline snapshot diff

`account snapshot diff` loads two manifests and their payload JSON offline. JSON output includes raw compared values, deltas, assumptions, Wilson 95% bounds (when hatch-rate analysis is requested), reconciliation status (`RECONCILED` / `PARTIAL` / `MISMATCH` / `INSUFFICIENT_DATA`), and data-quality warnings. Negative deltas are flagged but not treated as automatic errors.

Timing rates prefer inventory `snapshotAt` values and are labeled as a proxy when profile counters may have been saved at different times. Missing timestamps yield null rates rather than invented values.

## Loadout analysis

`account loadout analyze|optimize` JSON includes current equipped pets/enchants, owned pools, per-channel estimates with `scoreFullyResolved`, unresolved factor IDs, confidence, and candidate substitutions with reasons. Equipped enchant `level: 0` is reported as unresolved tier (not tier I). Numeric marginal gains are omitted unless a fully verified multiplier chain exists.

`account loadout calibrate` reports observed currency deltas, optional per-minute rates, changed variables, data freshness, confidence, and possible confounders. It does not auto-write registry magnitudes.

When both snapshots show a currency at a known ceiling (currently `FantasyCoins = 300000000000000000000`) with a zero delta, `observedPerMinute` is omitted and the report includes `currencyCap` with finding `CAPPED: income rate cannot be identified from balance differences`. Concurrent egg/purchase activity while still at the ceiling may add a note that production appears sufficient to offset spending, still without inventing a rate.

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | Success |
| 2 | Invalid arguments or local validation failure |
| 3 | Public resource unavailable or retained row missing |
| 4 | Network, DNS, TLS, or timeout failure |
| 5 | Upstream error, invalid JSON, or unusable shape |
| 6 | HTTP 429 rate limit |
| 7 | Secure credential store failure |
| 8 | Not logged in, token expired/revoked, or required scope missing |
| 9 | OAuth state, callback, or token-exchange failure |

Quota exhaustion with HTTP 200 remains success. Responses surface `quotaExhausted: true` when present.

HTTP 429 reports `Retry-After` when provided. The client does not sleep automatically.

## Freshness and quota

Authenticated account data is a Roblox save snapshot cached by BIG Games.

- Standard accounts: up to 48 fresh snapshots per UTC day
- VIP accounts: up to 96 per UTC day

`--refresh` bypasses the short freshness window but still consumes quota and cannot exceed the daily limit. Forced refresh requires explicit confirmation in non-interactive use.
