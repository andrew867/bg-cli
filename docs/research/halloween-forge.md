# Halloween Forge — research and methodology

**Observations:** 2026-11-06 (America/St_Johns)  
**Scope:** Displayed Forge UI probabilities for Butler-only (and separately noted mixed) recipes. Not server RNG.

## Butler-only displayed win probabilities

Conditions: 100 Spirit Shards, ×130 Luck, no other pets, displayed max success 75%.

| Butlers | Displayed win % | Expected Butlers / success (`n/p`) |
| ---: | ---: | ---: |
| 1 | 2.58% | 38.76 |
| 5 | 12.9% | 38.76 |
| 10 | 25.8% | 38.76 |
| 20 | 51.7% | 38.68 |
| 24 | 62.0% | ~38.71 |
| 25 | 64.6% | ~38.70 |
| 26 | 67.2% | ~38.69 |
| 27 | 69.8% | 38.68 |
| 28 | 72.4% | ~38.67 |
| 29 | 74.9% | 38.72 |
| 30 | 75.0% | 40.00 |

Empirical approximation fitted by bg-cli (through-origin OLS below the observed cap):

`P_display(n) ≈ min(slope · n, 0.75)` with `slope ≈ 0.02584`

This is a **fitted model of displayed percentages**, not a verified server algorithm. Rounding in the UI absorbs small residuals.

## Mixed recipes

An earlier 27-Butler observation at **74.7%** included additional common pets (quantities uncertain). Butler-only 27 is **69.8%**. These must never be pooled for the Butler-only slope.

Historical Huge outcomes with mixed Neon/Porcelain inputs are outcome ledgers, not complete coefficient measurements.

## Loot rows

At 29–30 Butlers, UI showed Guard Elephant / Piggy / Giraffe / `<0.001%` rare / Spirit Shards fallback ~25% / Titanic **SOLD OUT**.

- Censored `<0.001%` is an upper bound, never zero or exact.
- Sold-out rewards are unavailable even if artwork remains.
- Component percentages may not sum to 100% after rounding; bg-cli does not invent missing mass.
- Fallback shards at low Butler counts were not always visible — treat as unknown unless displayed or experimentally confirmed.

## “TOO MANY PETS”

Selecting 50,000 Neon Skeleton Cats showed this warning. That is insufficient to claim a global 50k cap.

## Actual RNG vs displayed probability

Screenshot curves do **not** yield confidence intervals for real Forge outcomes. Attempt ledgers record executed results separately; one attempt never “validates” a displayed percentage.
