# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Seven-view `account snapshot` via `--all-views` / `--include` (default three-view behavior unchanged); `account snapshot validate` integrity checks; manifest schema `1.2` with view request/content status
- Offline analytics: `account index`, `transactions`, `hatch`, `progression`, `portfolio`, `reconcile`, `audit`
- Public intelligence: `releases`, `leagues`, `clans` (sampled ranking depth disclosed)
- Forge `--mode observed-mixed` with variable shard budgets and observed Pareto frontier (Butler-only model unchanged)
- Docs: `docs/research/analytics-methodology.md`; Windows offline smoke `scripts/windows-smoke-offline.ps1`
- `account craft` Combine-o-Matic (NOT Forge): recipes, BOM plan, optimize; Gargantuan BOM 480/3420/48/12/4/2 regression
- `account hatch compare` for versioned experiment checkpoints; Masquerade Thorn Egg Nov 7 fixture; EggHatchCount≠pull-size warnings
- Item-flow annotations + Candy Coins runway economics with explicit residuals
- Public data observatory: `public discover|sync|inspect|status|diff|export`, `egg show|odds|flags|cost`, `graph unresolved`; Secret Room odds models + 51724 hatch fixture; docs under `docs/public-*.md` / `docs/egg-probability-models.md`
- `account tiers resolve`: join inventory `id`+`tn` (and equipped `level`>0) to public Enchants/Potions/Charms `Tiers[].DisplayName`

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
