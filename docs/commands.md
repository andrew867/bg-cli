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

`csv` writes spreadsheet rows for `account inventory`, `account currencies`, and `account quota`. Counts and `_am` / `amount` values keep exact JSON number text (no float64 rounding). Other commands reject `--output csv`.

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

Export examples:

```bash
bg-cli account quota --profile main --pretty
bg-cli account inventory --profile main --output csv --join-rap --excel > inventory.csv
bg-cli account currencies --profile main --output csv --excel > currencies.csv
bg-cli account inventory --profile main --output table
```

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
