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
