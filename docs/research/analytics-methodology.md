# Analytics methodology (bg-cli)

Offline analyses distinguish **verified source fields**, **exact derived arithmetic**, **statistical estimates**, and **unknown / unavailable** values.

## Snapshots

Manifest schema `1.2` adds optional multi-view capture (`--all-views` / `--include`). Default remains inventory + profile + extendedProfile. Trades, booth, mail and itemIndex require explicit opt-in (privacy). Independently retrieved views are not an atomic DB transaction. `--refresh` applies to at most the first requested view.

`account snapshot validate` checks size + SHA-256 for every OK file record.

## Hatch distributions

`account hatch report` attributes species from `PetHatchCount` deltas, not inventory ownership. A Hollow Egg Nov 6 fixture proves:

`338163 + 77112 + 20 + 1 = 415296 = 1224483 − 809187`

Observed rates (~81.43% / 18.57% / 0.0048% / 0.00024%) are historical frequencies with Wilson intervals, not official drop tables. Multi-egg intervals without unique attribution remain ambiguous.

## Transactions

Trade / booth / mail adapters normalize movements. Booth APIs do not provide universal completed sale prices — RAP is never treated as proceeds.

## Portfolio

Matched RAP totals exclude unmatched rows from the sum and report coverage. Variant-specific quotes only; no silent Regular→Rainbow backfill.

## Public intelligence

Releases / leagues / clans are unauthenticated. Contributor endpoints are **sampled** (e.g. top 500). Missing ranks are not zeros. Release `changes: null` on older items means field diffs unavailable.

## Forge observed-mixed

`--mode observed-mixed` optimizes only recipes with recorded displayed probabilities. Variable shard budgets (60/63/100) are supported. Butler-only fitted slopes never fill missing mixed probabilities.
