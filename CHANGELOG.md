# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- (none)

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
