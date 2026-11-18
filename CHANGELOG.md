# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Profile config stores `ps99ClanTag`. Snapshots and `clan show` use it when the account documents have no clan tag. `auth profile set` and `auth profile unset` edit `ps99UserId`, `ps99ClanTag`, and `accountId`. `auth login` keeps those fields.

## [0.2.0] - 2026-11-18

### Changed

- Auth docs state that reauthorizing a profile assumes the same game account. The snapshot console prints the previous window before the session window.

### Added

- Seven-view `account snapshot` via `--all-views` / `--include` (default three-view behavior unchanged); `account snapshot validate` integrity checks; manifest schema `1.2` with view request/content status
- Offline analytics: `account index`, `transactions`, `hatch`, `progression`, `portfolio`, `reconcile`, `audit`
- Public intelligence: `releases`, `leagues`, `clans` (sampled ranking depth disclosed)
- Read-only `clan show` and `clan battle` (`GET /api/clan/{tag}`, `GET /api/activeClanBattle`); snapshot `clan.json` (full document, including `Battles`) and `clanbattle.json` only while a battle is active
- Snapshot `summary.json`: shared baselines for clan points and currencies over 5m, 30m, 1h, and the session, with exact amounts and `netPerHour` from actual elapsed seconds
- Snapshot deltas stay inside one account, clan tag, battle, and player id. `--new-session` and `--session-id` reset the session and cannot be combined. `currenciesKnown` is stored explicitly. Optional clan telemetry does not fail the snapshot unless `--require-clan` is set. Non-integral rates also include `netPerHourApprox`. A `previous` window compares with the latest compatible summary. Currency rates are marked `observation_interval` estimates
- `auth login` keeps `accountId` and `ps99UserId` when the same profile is authorized again
- Forge `--mode observed-mixed` with variable shard budgets and observed Pareto frontier (Butler-only model unchanged)
- Docs: `docs/research/analytics-methodology.md`; Windows offline smoke `scripts/windows-smoke-offline.ps1`

## [0.1.4] - 2026-11-07

### Added

- `account loadout` analyze/optimize/recommend/calibrate: versioned effect registry, per-currency channels, world-specific pet slot handling, owned-only recommendations, and measured snapshot calibration without inventing DPS or unverified multipliers
- `account power inspect`: layered pet-power models per world/event with public-config provenance, optional annotations for Huge levels/charms, and competing Halloween 31% models (A–D)
- `account experiment compare`: offline controlled snapshot experiments with confounder checks per currency ID
- Research notes: `docs/research/pet-power-and-events.md`
- `account forge observe|analyze|optimize|ledger`: offline Halloween Forge probability fitting and recipe optimization from UI observations (capped-linear Butler-only model, mixed-recipe uncertainty, local attempt ledger)
- Research notes: `docs/research/halloween-forge.md`; operator flow: `docs/operator-guide-analysis.md`
- Calibration / snapshot diff: detect known currency ceilings (FantasyCoins `300000000000000000000`); suppress false zero income rates with `CAPPED: income rate cannot be identified from balance differences`, optional spend-offset note, `--currency-cap` override

## [0.1.3] - 2026-11-06

### Added

- `account quota` summarizes refresh used/limit and rate-limit / Retry-After headers without dumping profile bodies
- `account inventory --join-rap` joins public market RAP into CSV (`marketRap` / `marketRapMatched`)
- Currency CSV adds an Excel-friendly `amount` column (mirrors exact `_am`); `--excel` writes a UTF-8 BOM
- Inventory CSV includes pet variant columns (`variant`, `golden`, `rainbow`, `shiny`, `pt`, `sh`, `stackKey`) derived from `rawData`
- Inventory responses add `petRowCount` / `petQuantitySum`; `quantitySum` is documented as an all-rows count sum (may include currency stacks)
- `account snapshot` writes dated inventory/profile/extended-profile JSON + inventory/currencies CSV plus a compare-friendly manifest (SHA-256, collision suffixes, `--note`)
- `account snapshot diff` compares two manifests completely offline (statistics, purchases, hatches, inventory variants, currencies, hatch-rate / reconciliation / cost analysis)

## [0.1.2] - 2026-11-06

### Added

- `--output csv` for `account inventory` and `account currencies` with exact integer text (no float64 rounding)
- Richer `--output table` layouts for inventory, currencies, and leaderboard list
- `bg-cli doctor` for local config, credential store, and scope checks
- `bg-cli completion` for bash, zsh, fish, and PowerShell
- Operator acceptance checklist and opt-in public live smoke workflow

## [0.1.1] - 2026-11-06

### Changed

- Release packaging now uses GoReleaser for Linux/macOS/Windows archives, source tarball, and checksums

## [0.1.0] - 2026-11-06

### Added

- Initial public release of `bg-cli`
- OAuth Authorization Code login with PKCE S256, loopback callback, and OS keyring token storage
- Account commands: inventory, profile, currencies, extended-profile, item-index, trades, booth, mail
- Public commands: player views/show, leaderboard list/show/player, collection list/show, market rap/exists
- Stable JSON output envelope, table mode, and raw upstream bodies for data commands
- Public response cache honoring `Cache-Control` max-age
- Cross-platform CI and tagged release workflow with checksums
